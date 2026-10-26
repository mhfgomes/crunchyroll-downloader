package main

import (
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHTTPClientUsesHTTP1WithHTTP2Peer(t *testing.T) {
	for _, initialized := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh_default", true: "initialized_default"}[initialized], func(t *testing.T) {
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					if r.ProtoMajor != 1 {
						t.Errorf("login negotiated %s", r.Proto)
					}
					_, _ = io.WriteString(w, `{"access_token":"test-token"}`)
					return
				}
				_, _ = io.WriteString(w, r.Proto)
			}))
			srv.EnableHTTP2 = true
			srv.TLS = &tls.Config{NextProtos: []string{"h2", "http/1.1"}}
			srv.StartTLS()
			defer srv.Close()

			// Clone can inherit h2 ALPN entries from an initialized default
			// transport. Retain them here so the test catches that mismatch.
			base := &http.Transport{ForceAttemptHTTP2: true}
			base.TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
			defer base.CloseIdleConnections()
			oldDefault := http.DefaultTransport
			http.DefaultTransport = base
			t.Cleanup(func() { http.DefaultTransport = oldDefault })
			if initialized {
				resp, err := (&http.Client{Transport: base}).Get(srv.URL)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.ProtoMajor != 2 {
					t.Fatalf("warmup used %s, want HTTP/2", resp.Proto)
				}
			}
			client := newHTTPClient()
			transport := client.Transport.(*http.Transport)
			// Add test trust roots without replacing the ALPN configuration.
			transport.TLSClientConfig.RootCAs = srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
			defer transport.CloseIdleConnections()
			resp, err := client.Get(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != "HTTP/1.1" || resp.TLS.NegotiatedProtocol != "http/1.1" {
				t.Fatalf("response=%q ALPN=%q, want HTTP/1.1", body, resp.TLS.NegotiatedProtocol)
			}
			oldClient, oldURL := httpClient, tokenURL
			httpClient, tokenURL = client, srv.URL
			t.Cleanup(func() { httpClient, tokenURL = oldClient, oldURL })
			if got, err := GetAccessToken("test-cookie"); err != nil || got != "test-token" {
				t.Fatalf("TLS login token=%q err=%v", got, err)
			}
		})
	}
}

type interruptedBody struct {
	closed *int
}

func (b interruptedBody) Read(p []byte) (int, error) {
	return copy(p, "partial"), io.ErrUnexpectedEOF
}

func (b interruptedBody) Close() error { *b.closed++; return nil }

func TestReadGETRetriesInterruptedBody(t *testing.T) {
	for _, permanent := range []bool{false, true} {
		t.Run(map[bool]string{false: "recovers", true: "exhausted"}[permanent], func(t *testing.T) {
			attempts, closed := 0, 0
			mockPlayback(t, func(r *http.Request) (*http.Response, error) {
				attempts++
				resp := playbackResponse(r, 200, "complete")
				if permanent || attempts == 1 {
					resp.Body = interruptedBody{&closed}
				}
				return resp, nil
			})
			req, _ := http.NewRequest(http.MethodGet, "https://cdn.example/subtitles", nil)
			body, err := readGET(req)
			if permanent {
				if !errors.Is(err, io.ErrUnexpectedEOF) || attempts != 3 || closed != 3 {
					t.Fatalf("attempts=%d closed=%d err=%v", attempts, closed, err)
				}
			} else if err != nil || string(body) != "complete" || attempts != 2 || closed != 1 {
				t.Fatalf("body=%q attempts=%d closed=%d err=%v", body, attempts, closed, err)
			}
		})
	}
}

func TestReadGETDoesNotRetryHTTPRejection(t *testing.T) {
	var calls int
	mockPlayback(t, func(r *http.Request) (*http.Response, error) {
		calls++
		return playbackResponse(r, http.StatusForbidden, "forbidden"), nil
	})
	req, _ := http.NewRequest(http.MethodGet, "https://cdn.example/subtitles", nil)
	if _, err := readGET(req); err == nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestPlaybackCreationDoesNotRetryInterruptedBody(t *testing.T) {
	opened, closed := 0, 0
	mockPlayback(t, func(r *http.Request) (*http.Response, error) {
		opened++
		resp := playbackResponse(r, 200, "")
		resp.Body = interruptedBody{&closed}
		return resp, nil
	})
	_, err := getEpisode("dub")
	if !errors.Is(err, io.ErrUnexpectedEOF) || opened != 1 || closed != 1 {
		t.Fatalf("opened=%d closed=%d err=%v", opened, closed, err)
	}
}

// useTokenServer points GetAccessToken at handler for the duration of the
// test and restores the global token afterwards.
func useTokenServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	oldURL, oldToken := tokenURL, token
	tokenURL = srv.URL
	t.Cleanup(func() { tokenURL, token = oldURL, oldToken })
}

func TestGetAccessToken(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		body         string
		want         string
		wantRejected bool
		wantErr      bool
	}{
		{"valid cookie", http.StatusOK, `{"access_token":"abc"}`, "abc", false, false},
		{"invalid cookie", http.StatusBadRequest, `{"code":"auth.obtain_access_token.oauth2_error","error":"invalid_grant"}`, "", true, true},
		{"unauthorized", http.StatusUnauthorized, `{}`, "", true, true},
		{"server error", http.StatusInternalServerError, `oops`, "", false, true},
		{"empty token", http.StatusOK, `{"access_token":""}`, "", false, true},
		{"malformed JSON", http.StatusOK, `not json`, "", false, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			useTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})

			got, err := GetAccessToken("cookie")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if errors.Is(err, errAuthRejected) != tc.wantRejected {
				t.Errorf("errors.Is(err, errAuthRejected) = %v, want %v (err: %v)", !tc.wantRejected, tc.wantRejected, err)
			}
			if got != tc.want {
				t.Errorf("token = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDoRequestRefreshesTokenOnce(t *testing.T) {
	useTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"access_token":"fresh"}`)
	})
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer fresh" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// Echo the body to prove the retry resent it.
		_, _ = io.Copy(w, r.Body)
	}))
	defer api.Close()

	req, err := http.NewRequest(http.MethodPost, api.URL, strings.NewReader("payload"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer stale")

	resp, err := DoRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "payload" {
		t.Errorf("retried body = %q, want %q", body, "payload")
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("API called %d times, want 2", n)
	}
	if token != "fresh" {
		t.Errorf("token = %q, want %q", token, "fresh")
	}
}

func TestDoRequestStopsWhenCookieRejected(t *testing.T) {
	var tokenCalls atomic.Int32
	useTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		tokenCalls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
	})
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer api.Close()

	req, err := http.NewRequest(http.MethodGet, api.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := DoRequest(req); !errors.Is(err, errAuthRejected) {
		t.Errorf("err = %v, want errAuthRejected", err)
	}
	if n := tokenCalls.Load(); n != 1 {
		t.Errorf("token endpoint called %d times, want 1", n)
	}
}

func TestDoRequestStopsWhenStillUnauthorized(t *testing.T) {
	var tokenCalls atomic.Int32
	useTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		tokenCalls.Add(1)
		_, _ = io.WriteString(w, `{"access_token":"fresh"}`)
	})
	var apiCalls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer api.Close()

	req, err := http.NewRequest(http.MethodGet, api.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := DoRequest(req); err == nil {
		t.Error("expected an error after a second 401, got nil")
	}
	if n := tokenCalls.Load(); n != 1 {
		t.Errorf("token endpoint called %d times, want 1", n)
	}
	if n := apiCalls.Load(); n != 2 {
		t.Errorf("API called %d times, want 2", n)
	}
}
