package downloader

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	runtimedebug "runtime/debug"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iyear/gowidevine"
	"github.com/unki2aut/go-mpd"
)

const maxWorkers = 10

func buildUrl(base, representationId, file string, partNum *int64) string {
	if partNum != nil {
		file = strings.ReplaceAll(file, "$Number$", fmt.Sprintf("%05d", *partNum))
		file = strings.ReplaceAll(file, "$Number%05d$", fmt.Sprintf("%05d", *partNum))
	}
	return base + strings.ReplaceAll(file, "$RepresentationID$", representationId)
}

func downloadPart(url string) ([]byte, error) {
	maxRetries := 5
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			sleepCtx(time.Duration(attempt) * 2 * time.Second)
		}

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Origin", "https://static.crunchyroll.com")
		req.Header.Set("Referer", "https://static.crunchyroll.com/")
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:147.0) Gecko/20100101 Firefox/147.0")
		resp, err := httpClient.Do(req)
		if err != nil {
			if attempt < maxRetries-1 {
				continue
			}
			return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, err)
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			if attempt < maxRetries-1 {
				continue
			}
			return nil, fmt.Errorf("failed after %d retries, status: %d", maxRetries, resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			if attempt < maxRetries-1 {
				continue
			}
			return nil, fmt.Errorf("failed reading body after %d retries: %w", maxRetries, err)
		}
		return body, nil
	}
	return nil, fmt.Errorf("failed after %d retries", maxRetries)
}

func getFilename(set *mpd.AdaptationSet, subExt string) string {
	if set == nil {
		if subExt == "" {
			subExt = "ass"
		}
		f, _ := os.CreateTemp("", "crdl-subs-*."+subExt)
		return f.Name()
	}
	for _, representation := range set.Representations {
		if representation.Height != nil {
			f, _ := os.CreateTemp("", "crdl-video-*.mp4")
			return f.Name()
		} else if representation.Bandwidth != nil {
			f, _ := os.CreateTemp("", "crdl-audio-*.mp3")
			return f.Name()
		}
	}
	return ""
}

// maxBufferedSegments bounds how many segments may be resident in memory at
// once, counting both in-flight downloads and payloads still waiting to be
// written. Without it the workers race arbitrarily far ahead of the sequential
// writer whenever an early segment is slow, which is what exhausted memory on
// movie-length titles.
const maxBufferedSegments = maxWorkers * 2

// streamSegments fetches every url concurrently but writes the payloads to w
// strictly in index order, releasing each one as soon as it has been written.
// Peak memory is therefore bounded by maxBufferedSegments rather than growing
// with the length of the media.
//
// onProgress, if non-nil, is called with the running count of fetched segments.
func streamSegments(w io.Writer, urls []string, fetch func(string) ([]byte, error), onProgress func(fetched int64)) error {
	total := len(urls)
	if total == 0 {
		return nil
	}

	var (
		mu      sync.Mutex
		cond    = sync.NewCond(&mu)
		payload = make([][]byte, total)
		ready   = make([]bool, total)
		failure error
	)

	abort := make(chan struct{})
	var abortOnce sync.Once
	fail := func(err error) {
		mu.Lock()
		if failure == nil {
			failure = err
		}
		mu.Unlock()
		cond.Broadcast()
		abortOnce.Do(func() { close(abort) })
	}

	// A worker claims a slot before claiming an index, so the indices held at
	// any moment are always the lowest outstanding ones. That ordering is what
	// guarantees the writer's next index is always held by a live worker and
	// can never be starved by later segments hogging every slot.
	slots := make(chan struct{}, maxBufferedSegments)
	var next atomic.Int64
	var fetched atomic.Int64

	var wg sync.WaitGroup
	for n := 0; n < maxWorkers; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case slots <- struct{}{}:
				case <-abort:
					return
				}

				i := int(next.Add(1) - 1)
				if i >= total {
					<-slots
					return
				}

				data, err := fetch(urls[i])
				if err != nil {
					fail(err)
					return
				}

				mu.Lock()
				payload[i] = data
				ready[i] = true
				mu.Unlock()
				cond.Broadcast()

				if onProgress != nil {
					onProgress(fetched.Add(1))
				}
			}
		}()
	}

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for i := 0; i < total; i++ {
			mu.Lock()
			for !ready[i] && failure == nil {
				cond.Wait()
			}
			if failure != nil {
				mu.Unlock()
				return
			}
			data := payload[i]
			payload[i] = nil
			mu.Unlock()

			_, err := w.Write(data)
			<-slots
			if err != nil {
				fail(fmt.Errorf("writing segment %d: %w", i, err))
				return
			}
		}
	}()

	wg.Wait()
	<-writerDone

	mu.Lock()
	defer mu.Unlock()
	return failure
}

func downloadParts(title string, baseUrl, representationId *string, set *mpd.AdaptationSet, keys []*widevine.Key) (string, error) {
	initUrl := buildUrl(*baseUrl, *representationId, *set.SegmentTemplate.Initialization, nil)
	initData, err := downloadPart(initUrl)
	if err != nil {
		return "", err
	}

	timeline := expandTimeline(set.SegmentTemplate.SegmentTimeline.S, 1)
	total := len(timeline)
	urls := make([]string, total)
	for i, item := range timeline {
		urls[i] = buildUrl(*baseUrl, *representationId, *set.SegmentTemplate.Media, &item)
	}

	filename := getFilename(set, "")
	encPath := filename + ".enc"
	encFile, err := os.Create(encPath)
	if err != nil {
		return "", err
	}
	defer os.Remove(encPath)
	defer encFile.Close()

	if _, err = encFile.Write(initData); err != nil {
		return "", fmt.Errorf("writing init segment: %w", err)
	}

	bar := newProgressBar(title, int64(total), "segments")
	var totalBytes atomic.Int64
	fetch := func(url string) ([]byte, error) {
		data, err := downloadPart(url)
		if err == nil {
			totalBytes.Add(int64(len(data)))
		}
		return data, err
	}
	err = streamSegments(encFile, urls, fetch, func(fetched int64) {
		bar.update(fetched, totalBytes.Load())
	})
	bar.finish()
	if err != nil {
		return "", err
	}

	if _, err = encFile.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("rewinding %s: %w", encPath, err)
	}

	file, err := os.Create(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if err = decryptMP4(initData, encFile, keys, file); err != nil {
		return "", fmt.Errorf("decryptMP4: %w", err)
	}

	return filename, nil
}

// downloadAudioTrack downloads a version's audio representation into a temporary
// file. sets is non-nil only for on-demand manifests.
func downloadAudioTrack(manifest *mpd.MPD, sets []onDemandAdaptationSet, locale, quality string, keys []*widevine.Key) (string, error) {
	title := "Downloading " + trackTitle(locale) + " audio"
	if isOnDemand(manifest) {
		return downloadOnDemandAdaptation(title, sets, false, quality, keys)
	}
	audioSet := manifest.Period[0].AdaptationSets[1]
	audioBaseUrl, audioRepresentationId := getBaseUrl(audioSet, false, quality)
	if audioBaseUrl == nil {
		return "", fmt.Errorf("failed to get the audio base URL for %s, maybe the audio quality you entered is wrong?", locale)
	}
	return downloadParts(title, audioBaseUrl, audioRepresentationId, audioSet, keys)
}

// downloadVideoTrack downloads the video representation into a temporary file.
// The video track is identical across dubs, so it is downloaded only once.
func downloadVideoTrack(manifest *mpd.MPD, sets []onDemandAdaptationSet, quality string, keys []*widevine.Key) (string, error) {
	if isOnDemand(manifest) {
		return downloadOnDemandAdaptation("Downloading video", sets, true, quality, keys)
	}
	videoSet := manifest.Period[0].AdaptationSets[0]
	baseUrl, representationId := getBaseUrl(videoSet, true, quality)
	if baseUrl == nil {
		return "", fmt.Errorf("failed to get the video base URL, maybe the video quality you entered is wrong?")
	}
	return downloadParts("Downloading video", baseUrl, representationId, videoSet, keys)
}

func downloadSubs(url, format string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Origin", "https://static.crunchyroll.com")
	req.Header.Set("Referer", "https://static.crunchyroll.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:147.0) Gecko/20100101 Firefox/147.0")
	body, err := readGET(req)
	if err != nil {
		return "", fmt.Errorf("fetching subtitles: %w", err)
	}

	filename := getFilename(nil, format)
	file, err := os.Create(filename)
	if err != nil {
		return "", err
	}
	if _, err = file.Write(body); err != nil {
		file.Close()
		return "", err
	}
	file.Close()

	return filename, nil
}

// buildGuidByLocale maps each available audio locale to its playback GUID. The
// episode's "versions" list is authoritative; the episode's own content ID is
// only a fallback for single-audio content where the API lists no versions.
// Crunchyroll sets "audio_locale" to the preferred language (which may be a dub)
// while the episode ID still points at the original, so mapping audio_locale to
// the content ID directly would resolve the dub to the wrong (original) stream.
func buildGuidByLocale(info EpisodeInfo, baseContentId string) map[string]string {
	guidByLocale := map[string]string{}
	for _, v := range info.EpisodeMetadata.Versions {
		guidByLocale[v.AudioLocale] = v.GUID
	}
	if len(guidByLocale) == 0 && info.EpisodeMetadata.AudioLocale != "" {
		guidByLocale[info.EpisodeMetadata.AudioLocale] = baseContentId
	}
	return guidByLocale
}

// mergeSubtitleAndCaptions combines the subtitles and captions offered by every
// audio version of an episode. Subtitles (translation scripts) are usually
// identical across versions, but captions (closed captions) transcribe a
// specific dub and only appear on that version's playback. Taking the first
// downloadable entry per locale preserves the union without duplication.
func mergeSubtitleAndCaptions(episodes []Episode) (subtitles, captions map[string]*Subtitle) {
	subtitles = map[string]*Subtitle{}
	captions = map[string]*Subtitle{}
	for _, ep := range episodes {
		for locale, sub := range ep.Subtitles {
			if sub != nil && (subtitles[locale] == nil || subtitles[locale].URL == "") {
				subtitles[locale] = sub
			}
		}
		for locale, cc := range ep.Captions {
			if cc != nil && (captions[locale] == nil || captions[locale].URL == "") {
				captions[locale] = cc
			}
		}
	}
	return subtitles, captions
}

// filterAvailableLangs drops subtitle/caption locales that the episode does not
// offer, warning about each one instead of aborting the whole download. Subtitles
// are optional, so a missing locale (e.g. the default "en-US" on a movie) should
// not prevent the video and audio from being saved.
func filterAvailableLangs(langs []string, available map[string]*Subtitle, kind string, episode int) []string {
	var filtered []string
	for _, locale := range langs {
		if available[locale] == nil {
			logf("! %s locale %s is not available for episode %v, skipping it.\n", kind, locale, episode)
			continue
		}
		filtered = append(filtered, locale)
	}
	return filtered
}

func requestsAll(langs []string) bool {
	for _, locale := range langs {
		if strings.EqualFold(locale, "all") {
			return true
		}
	}
	return false
}

// expandLangs keeps explicitly requested locales first, then adds available
// locales in stable order. With ALL alone, the usual default locale leads when
// offered. Lists without ALL are returned unchanged, including duplicates and
// missing locales, so their existing handling remains intact.
func expandLangs(requested, available []string, preferred string) []string {
	if !requestsAll(requested) {
		return requested
	}

	selected := make([]string, 0, len(requested)+len(available))
	seen := make(map[string]bool, len(requested)+len(available))
	for _, locale := range requested {
		if strings.EqualFold(locale, "all") || seen[locale] {
			continue
		}
		selected = append(selected, locale)
		seen[locale] = true
	}
	if len(selected) == 0 {
		for _, locale := range available {
			if locale == preferred {
				selected = append(selected, locale)
				seen[locale] = true
				break
			}
		}
	}
	sort.Strings(available)
	for _, locale := range available {
		if !seen[locale] {
			selected = append(selected, locale)
			seen[locale] = true
		}
	}
	return selected
}

// additionalVersionGUIDs finds unselected dubs whose playback may offer other
// subtitles or captions. Sorting by locale keeps the first merged track for a
// repeated language stable across runs.
func additionalVersionGUIDs(guidByLocale map[string]string, selected []string) []string {
	seen := make(map[string]bool, len(selected))
	for _, guid := range selected {
		seen[guid] = true
	}
	locales := make([]string, 0, len(guidByLocale))
	for locale := range guidByLocale {
		if locale != "" {
			locales = append(locales, locale)
		}
	}
	sort.Strings(locales)
	var guids []string
	for _, locale := range locales {
		guid := guidByLocale[locale]
		if guid != "" && !seen[guid] {
			guids = append(guids, guid)
			seen[guid] = true
		}
	}
	return guids
}

// availableSubtitleLangs excludes entries with no downloadable file from ALL.
// Explicit requests still use filterAvailableLangs and retain their old rules.
func availableSubtitleLangs(tracks map[string]*Subtitle) []string {
	available := make([]string, 0, len(tracks))
	for locale, track := range tracks {
		if locale != "" && track != nil && track.URL != "" {
			available = append(available, locale)
		}
	}
	return available
}

func downloadEpisode(baseContentId string, info EpisodeInfo, audioLangs, subsLangs, ccLangs []string, videoQuality, audioQuality *string, baseDirectory string) (err error) {
	cleanSeriesTitle := sanitizeFilename(info.EpisodeMetadata.SeriesTitle)
	cleanEpisodeTitle := sanitizeFilename(info.Title)

	if _, err := os.Stat(filepath.Join(baseDirectory, cleanSeriesTitle)); err != nil {
		_ = os.MkdirAll(filepath.Join(baseDirectory, cleanSeriesTitle), 0777)
	}

	outputFile := filepath.Join(baseDirectory, cleanSeriesTitle, fmt.Sprintf("%s S%02dE%02d - %s [%s].mkv",
		cleanSeriesTitle,
		info.EpisodeMetadata.SeasonNumber,
		info.EpisodeMetadata.EpisodeNumber,
		cleanEpisodeTitle,
		*videoQuality,
	))

	if _, statErr := os.Stat(outputFile); statErr == nil {
		logf("Episode %v is already downloaded, skipping...\n", info.EpisodeMetadata.EpisodeNumber)
		ev := episodeEvent(baseContentId, info, StatusSkipped)
		ev.Output = outputFile
		emitEpisode(ev)
		return
	}

	// Resolve each requested audio locale to its version GUID. Each dub is a
	// separate playback stream with its own manifest, token and Widevine keys.
	guidByLocale := buildGuidByLocale(info, baseContentId)

	availableAudio := make([]string, 0, len(guidByLocale))
	for locale, guid := range guidByLocale {
		if locale != "" && guid != "" {
			availableAudio = append(availableAudio, locale)
		}
	}
	audioLangs = expandLangs(audioLangs, availableAudio, "ja-JP")

	type audioVersion struct {
		locale    string
		contentId string
	}
	var versions []audioVersion
	for _, locale := range audioLangs {
		guid, ok := guidByLocale[locale]
		if !ok {
			logf("! Audio locale %s is not available for episode %v, skipping it.\n", locale, info.EpisodeMetadata.EpisodeNumber)
			continue
		}
		versions = append(versions, audioVersion{locale: locale, contentId: guid})
	}
	if len(versions) == 0 {
		return fmt.Errorf("none of the requested audio locales are available for episode %v", info.EpisodeMetadata.EpisodeNumber)
	}

	logf("Downloading: %s (S%02vE%02v) from %s\n", info.Title, info.EpisodeMetadata.SeasonNumber, info.EpisodeMetadata.EpisodeNumber, info.EpisodeMetadata.SeriesTitle)

	// Space this download out from the previous one, if a delay is configured.
	// Applied here rather than around the whole function so that episodes
	// already on disk (returned above) don't burn the wait for nothing.
	backoff.wait()
	if err := runContext().Err(); err != nil {
		return err
	}
	emitEpisode(episodeEvent(baseContentId, info, StatusStarted))

	defer func() {
		backoff.done()
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("%v", r)
			}
			if debug {
				logf("Recovered from error: %v\n%s\n", r, runtimedebug.Stack())
			} else {
				logf("Recovered from error: %v\n", r)
			}
		}
	}()

	// Download subtitle files while their playback session is still open.
	// Cache by URL so identical scripts shared by dubs are fetched only once.
	var subtitleMu sync.Mutex
	subtitleFiles := map[string]string{}
	defer func() {
		for _, file := range subtitleFiles {
			_ = os.Remove(file)
		}
	}()
	cacheSubtitles := func(ep Episode) error {
		subtitleMu.Lock()
		defer subtitleMu.Unlock()
		for _, group := range []struct {
			requested []string
			tracks    map[string]*Subtitle
		}{{subsLangs, ep.Subtitles}, {ccLangs, ep.Captions}} {
			langs := expandLangs(group.requested, availableSubtitleLangs(group.tracks), "")
			for _, locale := range langs {
				sub := group.tracks[locale]
				if sub == nil || sub.URL == "" || subtitleFiles[sub.URL] != "" {
					continue
				}
				file, err := downloadSubs(sub.URL, sub.Format)
				if err != nil {
					return fmt.Errorf("downloading subtitles for %s: %w", locale, err)
				}
				subtitleFiles[sub.URL] = file
			}
		}
		return nil
	}

	episodes := make([]Episode, len(versions))
	audioTracks := make([]mediaTrack, len(versions))
	var videoFile string
	selectedGUIDs := make([]string, len(versions))
	for i, version := range versions {
		selectedGUIDs[i] = version.contentId
	}
	if err := runPlaybackJobs(len(versions), simultaneous, func(i int) error {
		version := versions[i]
		return withPlaybackSession(version.contentId, func(ep Episode) error {
			episodes[i] = ep
			if err := cacheSubtitles(ep); err != nil {
				return err
			}
			manifest, body, err := parseManifest(ep.ManifestURL)
			if err != nil {
				return fmt.Errorf("parsing manifest for %s: %w", version.locale, err)
			}
			pssh := getPssh(manifest)
			if pssh == nil {
				return fmt.Errorf("PSSH not found for %s", version.locale)
			}
			keys, err := getLicense(*pssh, version.contentId, ep.Token)
			if err != nil {
				return fmt.Errorf("getLicense for %s: %w", version.locale, err)
			}
			var sets []onDemandAdaptationSet
			if isOnDemand(manifest) {
				sets, err = parseOnDemand(body)
				if err != nil {
					return err
				}
			}
			// The first dub's audio and video share one playback session.
			// Always join both downloads before releasing that session.
			var videoErr error
			var videoWG sync.WaitGroup
			if i == 0 {
				videoWG.Add(1)
				go func() {
					defer videoWG.Done()
					videoFile, videoErr = downloadVideoTrack(manifest, sets, *videoQuality, keys)
				}()
			}
			audioFile, audioErr := downloadAudioTrack(manifest, sets, version.locale, *audioQuality, keys)
			videoWG.Wait()
			if audioErr != nil {
				audioErr = fmt.Errorf("downloading %s audio: %w", version.locale, audioErr)
			}
			if videoErr != nil {
				videoErr = fmt.Errorf("downloading video: %w", videoErr)
			}
			if err := errors.Join(audioErr, videoErr); err != nil {
				return err
			}
			audioTracks[i] = mediaTrack{file: audioFile, locale: version.locale}
			return nil
		})
	}); err != nil {
		panic(err)
	}
	if requestsAll(subsLangs) || requestsAll(ccLangs) {
		guids := additionalVersionGUIDs(guidByLocale, selectedGUIDs)
		extraEpisodes := make([]Episode, len(guids))
		if err := runPlaybackJobs(len(guids), simultaneous, func(i int) error {
			fetchErr := withPlaybackSession(guids[i], func(ep Episode) error {
				extraEpisodes[i] = ep
				return cacheSubtitles(ep)
			})
			if errors.Is(fetchErr, errPlaybackOpen) && !errors.Is(fetchErr, ErrRateLimited) && !errors.Is(fetchErr, ErrActiveStreams) {
				logf("! Playback %s is unavailable; skipping its subtitles and captions: %v\n", guids[i], fetchErr)
				return nil
			}
			return fetchErr
		}); err != nil {
			panic(err)
		}
		episodes = append(episodes, extraEpisodes...)
	}

	// Merge subtitles and captions across versions. Subtitles (translation
	// scripts) are usually identical across versions, while captions are the
	// per-dub transcriptions that only exist on their own version.
	subtitles, captions := mergeSubtitleAndCaptions(episodes)

	subsLangs = expandLangs(subsLangs, availableSubtitleLangs(subtitles), "en-US")
	ccLangs = expandLangs(ccLangs, availableSubtitleLangs(captions), "")

	logf("Audio locales: %s | Subtitle locales: %s | CC locales: %s\n",
		strings.Join(audioLangs, ", "), strings.Join(subsLangs, ", "), strings.Join(ccLangs, ", "))

	subsLangs = filterAvailableLangs(subsLangs, subtitles, "Subtitle", info.EpisodeMetadata.EpisodeNumber)
	ccLangs = filterAvailableLangs(ccLangs, captions, "Closed caption", info.EpisodeMetadata.EpisodeNumber)

	// Build the list of subtitle and caption downloads.
	type subJob struct {
		url    string
		format string
		locale string
		isCC   bool
	}
	var subJobs []subJob
	for _, locale := range subsLangs {
		sub := subtitles[locale]
		subJobs = append(subJobs, subJob{url: sub.URL, format: sub.Format, locale: locale})
	}
	for _, locale := range ccLangs {
		cc := captions[locale]
		subJobs = append(subJobs, subJob{url: cc.URL, format: cc.Format, locale: locale, isCC: true})
	}

	// Select cached tracks in the requested order after merging version metadata.
	subTracks := make([]mediaTrack, len(subJobs))
	for j, job := range subJobs {
		subTracks[j] = mediaTrack{file: subtitleFiles[job.url], locale: job.locale, format: job.format, isCC: job.isCC}
	}
	if len(subTracks) > 0 {
		logf("Downloaded subtitles!")
	}

	emitEpisode(episodeEvent(baseContentId, info, StatusMerging))
	mergeEverything(videoFile, audioTracks, subTracks, outputFile, info)
	ev := episodeEvent(baseContentId, info, StatusDone)
	ev.Output = outputFile
	emitEpisode(ev)
	return nil
}

// downloadEpisodeWithRetry runs downloadEpisode, retrying the same episode
// with a growing delay whenever Crunchyroll rate-limits the account, instead
// of moving on to the next episode and immediately tripping the same rate
// limit again.
func downloadEpisodeWithRetry(baseContentId string, info EpisodeInfo, audioLangs, subsLangs, ccLangs []string, videoQuality, audioQuality *string, baseDirectory string) error {
	for {
		err := downloadEpisode(baseContentId, info, audioLangs, subsLangs, ccLangs, videoQuality, audioQuality, baseDirectory)
		if err == nil {
			backoff.resetRateLimit()
			return nil
		}
		if ctxErr := runContext().Err(); ctxErr != nil {
			err = ctxErr
		}
		if !errors.Is(err, ErrRateLimited) {
			logf("! %v", err)
			ev := episodeEvent(baseContentId, info, StatusFailed)
			ev.Error = err.Error()
			emitEpisode(ev)
			return err
		}

		wait := backoff.rateLimitWait()
		retryAt := time.Now().Add(wait)
		logf("Retrying this episode in %s [%s]...", wait.Round(time.Second), retryAt.Local().Format(time.Kitchen))
		ev := episodeEvent(baseContentId, info, StatusRetrying)
		ev.Error = err.Error()
		ev.RetryAt = &retryAt
		emitEpisode(ev)
		sleepCtx(wait)
	}
}

// downloadSeason downloads every episode, moving past failed ones; it stops
// early only when the download is cancelled.
func downloadSeason(videoQuality, audioQuality *string, audioLangs, subsLangs, ccLangs []string, episodes []SeasonEpisode, baseDirectory string) error {
	if len(episodes) == 0 {
		return nil
	}
	logf("Downloading season %v of %s (%v episodes)", episodes[0].SeasonNumber, episodes[0].SeriesTitle, len(episodes))

	for _, episode := range episodes {
		info := EpisodeInfo{
			EpisodeMetadata: EpisodeMetadata{
				SeriesTitle:        episode.SeriesTitle,
				SeasonNumber:       episode.SeasonNumber,
				EpisodeNumber:      episode.EpisodeNumber,
				AudioLocale:        episode.AudioLocale,
				Versions:           episode.Versions,
				AvailabilityStarts: episode.AvailabilityStarts,
			},
			Title: episode.Title,
		}

		_ = downloadEpisodeWithRetry(episode.ID, info, audioLangs, subsLangs, ccLangs, videoQuality, audioQuality, baseDirectory)
		if err := runContext().Err(); err != nil {
			return err
		}
	}
	return nil
}
