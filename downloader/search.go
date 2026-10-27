package downloader

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// searchURL is a variable so tests can point it at a local server.
var searchURL = "https://www.crunchyroll.com/content/v2/discover/search"

// SearchResult is a series matching a search query.
type SearchResult struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Poster       string `json:"poster"`
	Backdrop     string `json:"backdrop"`
	SeasonCount  int    `json:"seasonCount"`
	EpisodeCount int    `json:"episodeCount"`
}

type catalogItem struct {
	ID             string `json:"id"`
	Type           string `json:"type"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	Images         Images `json:"images"`
	SeriesMetadata struct {
		SeasonCount  int `json:"season_count"`
		EpisodeCount int `json:"episode_count"`
	} `json:"series_metadata"`
}

func (item catalogItem) result() SearchResult {
	return SearchResult{
		ID:           item.ID,
		Title:        item.Title,
		Description:  item.Description,
		Poster:       pickImage(item.Images.PosterTall, 240),
		Backdrop:     pickImage(item.Images.PosterWide, 1600),
		SeasonCount:  item.SeriesMetadata.SeasonCount,
		EpisodeCount: item.SeriesMetadata.EpisodeCount,
	}
}

type searchResponse struct {
	Data []struct {
		Type  string        `json:"type"`
		Items []catalogItem `json:"items"`
	} `json:"data"`
}

// Search finds series by title.
func Search(query string) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if err := ensureToken(); err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("q", query)
	params.Set("n", "24")
	params.Set("type", "series")
	params.Set("preferred_audio_language", preferredRequestLocale(parseLangs(options.AudioLangs), "ja-JP"))
	params.Set("locale", preferredRequestLocale(parseLangs(options.SubsLangs), "en-US"))

	req, err := http.NewRequest(http.MethodGet, searchURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:147.0) Gecko/20100101 Firefox/147.0")
	resp, err := DoRequest(req)
	if err != nil {
		return nil, fmt.Errorf("searching: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("searching: HTTP %d", resp.StatusCode)
	}

	var body searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("searching: %w", err)
	}
	results := []SearchResult{}
	for _, group := range body.Data {
		for _, item := range group.Items {
			if item.Type != "" && item.Type != "series" {
				continue
			}
			results = append(results, item.result())
		}
	}
	return results, nil
}

// browseURL is a variable so tests can point it at a local server.
var browseURL = "https://www.crunchyroll.com/content/v2/discover/browse"

// Browse lists series from the catalogue, sorted by "popularity" or
// "newly_added".
func Browse(sortBy string, n int) ([]SearchResult, error) {
	if err := ensureToken(); err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("n", fmt.Sprint(n))
	params.Set("type", "series")
	params.Set("sort_by", sortBy)
	params.Set("locale", preferredRequestLocale(parseLangs(options.SubsLangs), "en-US"))

	req, err := http.NewRequest(http.MethodGet, browseURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:147.0) Gecko/20100101 Firefox/147.0")
	resp, err := DoRequest(req)
	if err != nil {
		return nil, fmt.Errorf("browsing: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("browsing: HTTP %d", resp.StatusCode)
	}

	var body struct {
		Data []catalogItem `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("browsing: %w", err)
	}
	results := make([]SearchResult, 0, len(body.Data))
	for _, item := range body.Data {
		results = append(results, item.result())
	}
	return results, nil
}
