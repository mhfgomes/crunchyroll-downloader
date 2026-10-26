package downloader

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchKeepsSeriesOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "frieren" {
			t.Errorf("q = %q", r.URL.Query().Get("q"))
		}
		_, _ = w.Write([]byte(`{"data":[{"type":"series","items":[
			{"id":"S1","type":"series","title":"Frieren","images":{"poster_tall":[[{"source":"small","width":60},{"source":"big","width":480}]]},"series_metadata":{"season_count":2,"episode_count":38}},
			{"id":"M1","type":"movie_listing","title":"A movie"}]}]}`))
	}))
	defer srv.Close()
	old := searchURL
	searchURL = srv.URL
	defer func() { searchURL = old }()
	oldToken := token
	token = "test"
	defer func() { token = oldToken }()

	got, err := Search(" frieren ")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "S1" || got[0].Poster != "big" || got[0].EpisodeCount != 38 {
		t.Fatalf("Search = %+v", got)
	}
}
