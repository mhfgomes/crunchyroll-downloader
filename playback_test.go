package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type playbackTransport func(*http.Request) (*http.Response, error)

func (f playbackTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func mockPlayback(t *testing.T, handler playbackTransport) {
	t.Helper()
	old := http.DefaultTransport
	http.DefaultTransport = handler
	t.Cleanup(func() { http.DefaultTransport = old })
}

func playbackResponse(r *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}
}

func TestPlaybackSessionLimit(t *testing.T) {
	for _, limit := range []int{1, 4} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			var mu sync.Mutex
			active := map[string]bool{}
			peak, opened, closed := 0, 0, 0
			firstWave := make(chan struct{})
			mockPlayback(t, func(r *http.Request) (*http.Response, error) {
				mu.Lock()
				defer mu.Unlock()
				parts := strings.Split(r.URL.Path, "/")
				if r.Method == http.MethodGet {
					id := parts[3]
					if len(active) >= limit {
						return playbackResponse(r, 200, `{"error":"TOO_MANY_ACTIVE_STREAMS"}`), nil
					}
					active[id] = true
					opened++
					peak = max(peak, len(active))
					if opened == limit {
						close(firstWave)
					}
					return playbackResponse(r, 200, fmt.Sprintf(`{"token":%q}`, id)), nil
				}
				id := parts[4]
				if !active[id] || parts[5] != id {
					t.Errorf("wrong playback released: %s", r.URL.Path)
				}
				delete(active, id)
				closed++
				return playbackResponse(r, 204, ""), nil
			})
			err := runPlaybackJobs(9, limit, func(i int) error {
				return withPlaybackSession(fmt.Sprintf("dub-%d", i), func(ep Episode) error {
					<-firstWave
					mu.Lock()
					defer mu.Unlock()
					if !active[ep.Token] {
						return errors.New("playback closed before tracks finished")
					}
					return nil
				})
			})
			if err != nil {
				t.Fatal(err)
			}
			if peak != limit || opened != 9 || closed != 9 || len(active) != 0 {
				t.Fatalf("peak=%d opened=%d closed=%d active=%d", peak, opened, closed, len(active))
			}
		})
	}
}

func TestPlaybackStopsAndReleasesOnFailure(t *testing.T) {
	for _, mode := range []string{"download", "panic", "release", "open"} {
		t.Run(mode, func(t *testing.T) {
			opened, closed, consumed := 0, 0, 0
			failure := errors.New("track failed")
			mockPlayback(t, func(r *http.Request) (*http.Response, error) {
				if r.Method == http.MethodGet {
					opened++
					if mode == "open" {
						return playbackResponse(r, 200, `{"error":"TOO_MANY_ACTIVE_STREAMS"}`), nil
					}
					return playbackResponse(r, 200, `{"token":"session"}`), nil
				}
				closed++
				if mode == "release" {
					return playbackResponse(r, 500, ""), nil
				}
				return playbackResponse(r, 204, ""), nil
			})
			err := runPlaybackJobs(8, 1, func(i int) error {
				return withPlaybackSession("dub", func(ep Episode) error {
					consumed++
					if mode == "panic" {
						panic(failure)
					}
					if mode == "download" {
						return failure
					}
					return nil
				})
			})
			if err == nil || opened != 1 {
				t.Fatalf("err=%v opened=%d", err, opened)
			}
			if mode == "open" {
				if closed != 0 || consumed != 0 || !errors.Is(err, ErrActiveStreams) {
					t.Fatalf("closed=%d consumed=%d err=%v", closed, consumed, err)
				}
			} else if closed != 1 {
				t.Fatalf("closed=%d", closed)
			}
			if (mode == "download" || mode == "panic") && !errors.Is(err, failure) {
				t.Fatalf("lost failure: %v", err)
			}
		})
	}
}

func TestPlaybackRejectsInvalidLimit(t *testing.T) {
	for _, limit := range []int{0, -1} {
		if err := runPlaybackJobs(1, limit, func(int) error { t.Fatal("job started"); return nil }); err == nil {
			t.Fatal("invalid limit accepted")
		}
	}
}

func TestPlaybackJobsJoinsRunningWorkOnFailure(t *testing.T) {
	failure := errors.New("download failed")
	started := make(chan struct{})
	failed := make(chan struct{})
	finish := make(chan struct{})
	joined := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- runPlaybackJobs(2, 2, func(i int) error {
			if i == 0 {
				<-started
				close(failed)
				return failure
			}
			close(started)
			<-finish
			close(joined)
			return nil
		})
	}()
	<-failed
	select {
	case err := <-done:
		close(finish)
		t.Fatalf("returned before running work finished: %v", err)
	default:
	}
	close(finish)
	if err := <-done; !errors.Is(err, failure) {
		t.Fatalf("err=%v, want download failure", err)
	}
	select {
	case <-joined:
	default:
		t.Fatal("running work was not joined")
	}
}
