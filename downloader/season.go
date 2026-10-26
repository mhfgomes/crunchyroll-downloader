package downloader

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type seasonEpisodesResponse struct {
	Data []SeasonEpisode `json:"data"`
}

type SeasonEpisode struct {
	ID                 string        `json:"id"`
	Versions           []*DubVersion `json:"versions"`
	SeasonNumber       int           `json:"season_number"`
	EpisodeNumber      int           `json:"episode_number"`
	SeriesTitle        string        `json:"series_title"`
	AudioLocale        string        `json:"audio_locale"`
	Title              string        `json:"title"`
	AvailabilityStarts string        `json:"availability_starts"`
	Images             Images        `json:"images"`
}

func getSeasonEpisodes(contentId string, audio_locale string, sub_locale string) []SeasonEpisode {
	if audio_locale == "" {
		audio_locale = "ja-JP"
	}

	if sub_locale == "" {
		sub_locale = "en-US"
	}

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://www.crunchyroll.com/content/v2/cms/seasons/%s/episodes?preferred_audio_language=%s&locale=%s", contentId, audio_locale, sub_locale), nil)
	if err != nil {
		panic(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:147.0) Gecko/20100101 Firefox/147.0")
	resp, err := DoRequest(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	var episodes seasonEpisodesResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	if err = json.Unmarshal(body, &episodes); err != nil {
		panic(err)
	}

	return episodes.Data
}

type Seasons struct {
	Data []Season `json:"data"`
}

type Season struct {
	ID               string   `json:"id"`
	SeasonNumber     int      `json:"season_number"`
	Title            string   `json:"title"`
	SeriesTitle      string   `json:"series_title"`
	NumberOfEpisodes int      `json:"number_of_episodes"`
	AudioLocales     []string `json:"audio_locales"`
}

// Images holds the artwork variants Crunchyroll returns for a content object,
// each a list of sizes.
type Images struct {
	Thumbnail  [][]Image `json:"thumbnail"`
	PosterTall [][]Image `json:"poster_tall"`
	PosterWide [][]Image `json:"poster_wide"`
}

type Image struct {
	Source string `json:"source"`
	Width  int    `json:"width"`
}

// pickImage returns the smallest variant at least minWidth wide, or the
// largest when none is.
func pickImage(sets [][]Image, minWidth int) string {
	if len(sets) == 0 || len(sets[0]) == 0 {
		return ""
	}
	best := sets[0][0]
	for _, img := range sets[0] {
		if (best.Width < minWidth && img.Width > best.Width) || (img.Width >= minWidth && img.Width < best.Width) {
			best = img
		}
	}
	return best.Source
}

func (i Images) thumbnail() string { return pickImage(i.Thumbnail, 480) }

func getSeasons(contentId string, audioLocale string, subLocale string) []Season {
	if audioLocale == "" {
		audioLocale = "ja-JP"
	}

	if subLocale == "" {
		subLocale = "en-US"
	}

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://www.crunchyroll.com/content/v2/cms/series/%s/seasons?force_locale=&preferred_audio_language=%s&locale=%s", contentId, audioLocale, subLocale), nil)
	if err != nil {
		panic(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:147.0) Gecko/20100101 Firefox/147.0")
	resp, err := DoRequest(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	var seasons Seasons
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	if err = json.Unmarshal(body, &seasons); err != nil {
		panic(err)
	}

	return seasons.Data
}
