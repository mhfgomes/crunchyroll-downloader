package downloader

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Options configures every download. Language lists are comma-separated
// locales, optionally including ALL (see the CLI flag help for the rules).
type Options struct {
	AudioLangs    string        `json:"audioLangs"`
	SubsLangs     string        `json:"subsLangs"`
	CCLangs       string        `json:"ccLangs"`
	VideoQuality  string        `json:"videoQuality"`
	AudioQuality  string        `json:"audioQuality"`
	OutputDir     string        `json:"outputDir"`
	Simultaneous  int           `json:"simultaneous"`
	DownloadDelay time.Duration `json:"downloadDelay"`
	DebugManifest bool          `json:"debugManifest"`
}

// DefaultOptions returns the same defaults as the CLI flags.
func DefaultOptions() Options {
	return Options{
		AudioLangs:   "ja-JP",
		SubsLangs:    "en-US",
		VideoQuality: "1080p",
		AudioQuality: "192k",
		OutputDir:    ".",
		Simultaneous: 1,
	}
}

// ProgressEvent describes one track download. A bar is announced with Done
// false, updated as it advances, and reported once more with Done true.
type ProgressEvent struct {
	ID       int64   `json:"id"`
	Title    string  `json:"title"`
	Label    string  `json:"label"`
	Current  int64   `json:"current"`
	Total    int64   `json:"total"`
	RateMbps float64 `json:"rateMbps"`
	Done     bool    `json:"done"`
}

// EpisodeStatus is the lifecycle stage reported in an EpisodeEvent.
type EpisodeStatus string

const (
	StatusStarted  EpisodeStatus = "started"
	StatusMerging  EpisodeStatus = "merging"
	StatusSkipped  EpisodeStatus = "skipped"
	StatusDone     EpisodeStatus = "done"
	StatusFailed   EpisodeStatus = "failed"
	StatusRetrying EpisodeStatus = "retrying"
)

// EpisodeEvent reports an episode moving through the download pipeline.
type EpisodeEvent struct {
	ID          string        `json:"id"`
	SeriesTitle string        `json:"seriesTitle"`
	Title       string        `json:"title"`
	Season      int           `json:"season"`
	Number      int           `json:"number"`
	Status      EpisodeStatus `json:"status"`
	Output      string        `json:"output,omitempty"`
	Error       string        `json:"error,omitempty"`
	RetryAt     *time.Time    `json:"retryAt,omitempty"`
}

// Hooks receive output that the CLI otherwise prints to the terminal. Any nil
// hook keeps the terminal behaviour; a non-nil Progress replaces the ANSI bars.
type Hooks struct {
	Log      func(line string)
	Progress func(ProgressEvent)
	Episode  func(EpisodeEvent)
}

var (
	options = DefaultOptions()
	debug   bool
	// simultaneous bounds concurrent playback sessions; see Options.
	simultaneous = 1
	etpRt        string
	token        = ""
	// backoff spaces out consecutive episode downloads by the configured
	// delay, and is a no-op (including when nil) when no delay is set.
	backoff *downloadBackoff

	hooksMu sync.RWMutex
	hooks   Hooks

	// runMu serialises downloads, which share the access token, backoff and
	// cancellation context.
	runMu  sync.Mutex
	ctxMu  sync.RWMutex
	runCtx = context.Background()
)

// Configure validates and applies opts to later downloads.
func Configure(opts Options) error {
	if opts.Simultaneous < 1 {
		return errors.New("simultaneous must be at least 1")
	}
	if opts.OutputDir == "" {
		opts.OutputDir = "."
	}
	if opts.VideoQuality == "" {
		opts.VideoQuality = "1080p"
	}
	if opts.AudioQuality == "" {
		opts.AudioQuality = "192k"
	}
	runMu.Lock()
	defer runMu.Unlock()
	options = opts
	debug = opts.DebugManifest
	simultaneous = opts.Simultaneous
	backoff = newDownloadBackoff(opts.DownloadDelay)
	return nil
}

// SetHooks replaces the output hooks.
func SetHooks(h Hooks) {
	hooksMu.Lock()
	hooks = h
	hooksMu.Unlock()
}

func currentHooks() Hooks {
	hooksMu.RLock()
	defer hooksMu.RUnlock()
	return hooks
}

// logf reports a message, as one or more lines, to the Log hook or stdout.
func logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if h := currentHooks().Log; h != nil {
		for _, line := range strings.Split(strings.TrimSpace(msg), "\n") {
			h(line)
		}
		return
	}
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}
	fmt.Print(msg)
}

func emitEpisode(ev EpisodeEvent) {
	if h := currentHooks().Episode; h != nil {
		h(ev)
	}
}

func episodeEvent(id string, info EpisodeInfo, status EpisodeStatus) EpisodeEvent {
	return EpisodeEvent{
		ID:          id,
		SeriesTitle: info.EpisodeMetadata.SeriesTitle,
		Title:       info.Title,
		Season:      info.EpisodeMetadata.SeasonNumber,
		Number:      info.EpisodeMetadata.EpisodeNumber,
		Status:      status,
	}
}

// runContext is the context of the download in progress. Requests that carry
// no context of their own inherit it, so cancelling a download aborts them.
func runContext() context.Context {
	ctxMu.RLock()
	defer ctxMu.RUnlock()
	return runCtx
}

// begin makes ctx the run context until the returned function is called.
func begin(ctx context.Context) func() {
	runMu.Lock()
	ctxMu.Lock()
	runCtx = ctx
	ctxMu.Unlock()
	return func() {
		ctxMu.Lock()
		runCtx = context.Background()
		ctxMu.Unlock()
		runMu.Unlock()
	}
}

// sleepCtx sleeps for d, returning early when the download is cancelled.
func sleepCtx(d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-runContext().Done():
	}
}

// contextTransport attaches the run context to requests made without one.
type contextTransport struct{ base http.RoundTripper }

func (t contextTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Context() == context.Background() {
		req = req.WithContext(runContext())
	}
	return t.base.RoundTrip(req)
}

// recoverErr converts a panic from the request helpers into an error.
func recoverErr(err *error) {
	if r := recover(); r != nil {
		if e, ok := r.(error); ok {
			*err = e
		} else {
			*err = fmt.Errorf("%v", r)
		}
	}
}

// ErrAuthRejected marks a login refused because the etp_rt cookie is wrong or
// expired.
var ErrAuthRejected = errAuthRejected

// Login exchanges the etp_rt cookie for an access token used by every request.
func Login(cookie string) error {
	if cookie == "" {
		return errors.New("the etp_rt cookie is empty")
	}
	t, err := GetAccessToken(cookie)
	if err != nil {
		return err
	}
	etpRt, token = cookie, t
	return nil
}

// FindFFmpeg locates ffmpeg and uses it for merging.
func FindFFmpeg() (string, error) {
	path, err := findFFmpeg()
	if err == nil {
		ffmpegPath = path
	}
	return path, err
}

// SetFFmpegPath overrides the ffmpeg binary used for merging.
func SetFFmpegPath(path string) { ffmpegPath = path }

// LanguageName returns the display name of a locale.
func LanguageName(locale string) string { return trackTitle(locale) }

// Languages lists every known locale, sorted.
func Languages() []string {
	out := make([]string, 0, len(languageNames))
	for locale := range languageNames {
		out = append(out, locale)
	}
	sort.Strings(out)
	return out
}

// parseLangs splits a comma-separated locale list, trimming spaces and dropping
// empties.
func parseLangs(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// preferredRequestLocale provides a real locale for the season API even when
// ALL is requested; track selection still happens against each episode later.
func preferredRequestLocale(langs []string, fallback string) string {
	for _, locale := range langs {
		if !strings.EqualFold(locale, "all") {
			return locale
		}
	}
	return fallback
}

// parseUrl extracts the content type ("watch" or "series") and content ID from a
// Crunchyroll URL, tolerating an optional locale prefix such as "/fr/". The
// content type and ID may appear at any position after the host.
func parseUrl(url string) (contentType, contentId string) {
	parts := strings.Split(strings.TrimRight(url, "/"), "/")
	for i, p := range parts {
		if (p == "watch" || p == "series") && i+1 < len(parts) {
			return p, parts[i+1]
		}
	}
	return "", ""
}

type langSelection struct {
	audio, subs, cc           []string
	primaryAudio, primarySubs string
}

func selectLangs() langSelection {
	s := langSelection{
		audio: parseLangs(options.AudioLangs),
		subs:  parseLangs(options.SubsLangs),
		cc:    parseLangs(options.CCLangs),
	}
	if len(s.audio) == 0 {
		s.audio = []string{"ja-JP"}
	}
	// The season/series API endpoints take a single preferred locale; use the
	// first explicit request or the usual default for ALL alone. Track
	// availability is resolved from each episode's dub versions later.
	s.primaryAudio = preferredRequestLocale(s.audio, "ja-JP")
	s.primarySubs = preferredRequestLocale(s.subs, "en-US")
	return s
}

// ProcessURL downloads an episode, one season of a series, or (when season is
// 0) every season. Episode failures are reported and skipped.
func ProcessURL(ctx context.Context, url string, season int) (err error) {
	contentType, contentId := parseUrl(url)
	if contentType == "" || contentId == "" {
		return fmt.Errorf("invalid URL (must be /watch/ or /series/): %s", url)
	}
	defer begin(ctx)()
	defer recoverErr(&err)

	langs := selectLangs()
	if contentType == "watch" {
		info := getEpisodeInfo(contentId)
		return downloadEpisodeWithRetry(contentId, info, langs.audio, langs.subs, langs.cc, &options.VideoQuality, &options.AudioQuality, options.OutputDir)
	}

	seasons := getSeasons(contentId, langs.primaryAudio, langs.primarySubs)
	if season != 0 {
		var seasonId string
		for _, s := range seasons {
			if s.SeasonNumber == season {
				seasonId = s.ID
				break
			}
		}
		if seasonId == "" {
			return fmt.Errorf("this anime has no season %v", season)
		}
		episodes := getSeasonEpisodes(seasonId, langs.primaryAudio, langs.primarySubs)
		return downloadSeason(&options.VideoQuality, &options.AudioQuality, langs.audio, langs.subs, langs.cc, episodes, options.OutputDir)
	}

	logf("No season number specified, downloading all seasons...\n")
	for _, s := range seasons {
		episodes := getSeasonEpisodes(s.ID, langs.primaryAudio, langs.primarySubs)
		if err := downloadSeason(&options.VideoQuality, &options.AudioQuality, langs.audio, langs.subs, langs.cc, episodes, options.OutputDir); err != nil {
			return err
		}
	}
	return nil
}

// Title is a downloadable episode or movie.
type Title struct {
	ID                 string        `json:"id"`
	SeriesTitle        string        `json:"seriesTitle"`
	Title              string        `json:"title"`
	Season             int           `json:"season"`
	Number             int           `json:"number"`
	Thumbnail          string        `json:"thumbnail"`
	AvailabilityStarts string        `json:"availabilityStarts"`
	AudioLocale        string        `json:"audioLocale"`
	Versions           []*DubVersion `json:"versions"`
}

// SeasonSummary is one season of a series; Episodes is filled by
// SeasonEpisodes.
type SeasonSummary struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Number       int      `json:"number"`
	EpisodeCount int      `json:"episodeCount"`
	AudioLocales []string `json:"audioLocales"`
}

// LookupResult describes what a URL points at: a single episode, or a series
// and its seasons.
type LookupResult struct {
	Kind        string          `json:"kind"` // "episode" or "series"
	SeriesTitle string          `json:"seriesTitle"`
	Episode     *Title          `json:"episode,omitempty"`
	Seasons     []SeasonSummary `json:"seasons,omitempty"`
}

// Lookup resolves a Crunchyroll URL without downloading anything.
func Lookup(url string) (res LookupResult, err error) {
	defer recoverErr(&err)
	contentType, contentId := parseUrl(url)
	switch contentType {
	case "watch":
		info := getEpisodeInfo(contentId)
		t := titleFromInfo(contentId, info)
		return LookupResult{Kind: "episode", SeriesTitle: t.SeriesTitle, Episode: &t}, nil
	case "series":
		langs := selectLangs()
		seasons := getSeasons(contentId, langs.primaryAudio, langs.primarySubs)
		res := LookupResult{Kind: "series"}
		for _, s := range seasons {
			res.Seasons = append(res.Seasons, SeasonSummary{ID: s.ID, Title: s.Title, Number: s.SeasonNumber, EpisodeCount: s.NumberOfEpisodes, AudioLocales: s.AudioLocales})
			if res.SeriesTitle == "" {
				res.SeriesTitle = s.SeriesTitle
			}
		}
		return res, nil
	}
	return LookupResult{}, fmt.Errorf("invalid URL (must be /watch/ or /series/): %s", url)
}

// SeasonEpisodes lists the episodes of a season.
func SeasonEpisodes(seasonID string) (titles []Title, err error) {
	defer recoverErr(&err)
	langs := selectLangs()
	for _, e := range getSeasonEpisodes(seasonID, langs.primaryAudio, langs.primarySubs) {
		titles = append(titles, Title{
			ID:                 e.ID,
			SeriesTitle:        e.SeriesTitle,
			Title:              e.Title,
			Season:             e.SeasonNumber,
			Number:             e.EpisodeNumber,
			Thumbnail:          e.Images.thumbnail(),
			AvailabilityStarts: e.AvailabilityStarts,
			AudioLocale:        e.AudioLocale,
			Versions:           e.Versions,
		})
	}
	return titles, nil
}

func titleFromInfo(id string, info EpisodeInfo) Title {
	m := info.EpisodeMetadata
	return Title{
		ID:                 id,
		SeriesTitle:        m.SeriesTitle,
		Title:              info.Title,
		Season:             m.SeasonNumber,
		Number:             m.EpisodeNumber,
		Thumbnail:          info.Images.thumbnail(),
		AvailabilityStarts: m.AvailabilityStarts,
		AudioLocale:        m.AudioLocale,
		Versions:           m.Versions,
	}
}

// DownloadTitle downloads one episode with the configured options, retrying
// while Crunchyroll rate-limits the account.
func DownloadTitle(ctx context.Context, t Title) (err error) {
	defer begin(ctx)()
	defer recoverErr(&err)
	langs := selectLangs()
	info := EpisodeInfo{
		EpisodeMetadata: EpisodeMetadata{
			SeriesTitle:        t.SeriesTitle,
			SeasonNumber:       t.Season,
			EpisodeNumber:      t.Number,
			AudioLocale:        t.AudioLocale,
			Versions:           t.Versions,
			AvailabilityStarts: t.AvailabilityStarts,
		},
		Title: t.Title,
	}
	return downloadEpisodeWithRetry(t.ID, info, langs.audio, langs.subs, langs.cc, &options.VideoQuality, &options.AudioQuality, options.OutputDir)
}
