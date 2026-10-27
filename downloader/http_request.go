package downloader

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Some playback/CDN peers reset HTTP/2 streams with PROTOCOL_ERROR, including
// while a response body is being read. Use HTTP/1.1 consistently for API,
// subtitle and media requests, retaining parallel downloads and TLS checking.
var httpClient = newHTTPClient()

func isRetryableReadError(err error) bool {
	var networkErr net.Error
	return errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) || errors.As(err, &networkErr)
}

func newHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Protocols = new(http.Protocols)
	transport.Protocols.SetHTTP1(true)
	// Clone initializes the default transport's HTTP/2 support and can inherit
	// "h2" in TLS ALPN even though Protocols now allows only HTTP/1. Without
	// clearing it, the server selects h2 and our HTTP/1 reader sees binary frames.
	transport.ForceAttemptHTTP2 = false
	transport.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper)
	if transport.TLSClientConfig == nil {
		transport.TLSClientConfig = new(tls.Config)
	}
	transport.TLSClientConfig.NextProtos = []string{"http/1.1"}
	transport.MaxIdleConnsPerHost = maxWorkers * 2
	return &http.Client{Transport: contextTransport{transport}}
}

// readGET retries interrupted read-only downloads, including failures after
// headers arrived. Do not use this for playback creation: despite its GET
// method, replaying that request can allocate another stream slot.
func readGET(req *http.Request) ([]byte, error) {
	if req.Method != http.MethodGet {
		return nil, fmt.Errorf("readGET requires GET")
	}
	const attempts = 3
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(time.Duration(attempt) * 250 * time.Millisecond)
			select {
			case <-req.Context().Done():
				timer.Stop()
				return nil, req.Context().Err()
			case <-timer.C:
			}
		}
		resp, err := httpClient.Do(req)
		if err == nil {
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				return nil, fmt.Errorf("%s %s: HTTP %d", req.Method, req.URL.Host, resp.StatusCode)
			}
			var body []byte
			body, err = io.ReadAll(resp.Body)
			resp.Body.Close()
			if err == nil {
				return body, nil
			}
		}
		if !isRetryableReadError(err) || attempt == attempts-1 || req.Context().Err() != nil {
			return nil, fmt.Errorf("%s %s: reading response failed: %w", req.Method, req.URL.Host, err)
		}
		logf("Connection interrupted while reading %s; retrying (%d/%d)...\n", req.URL.Host, attempt+1, attempts-1)
	}
	panic("unreachable")
}

// DoRequest sends req and, if Crunchyroll answers 401, refreshes the access
// token and retries once. A second 401 is returned as an error rather than
// refreshing again, since it means the new token is being refused too.
func DoRequest(req *http.Request) (*http.Response, error) {
	resp, err := httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		return resp, err
	}

	logf("Access token expired. Refetching one...\n")
	resp.Body.Close()
	newToken, err := GetAccessToken(etpRt)
	if err != nil {
		return nil, err
	}
	token = newToken
	req.Header.Set("Authorization", "Bearer "+token)
	// The first attempt already consumed req.Body, so reset it from
	// GetBody before retrying or the retry sends an empty/truncated
	// payload (which broke license requests as "invalid license type").
	if req.GetBody != nil {
		if body, err := req.GetBody(); err == nil {
			req.Body = body
		}
	}

	resp, err = httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		return nil, fmt.Errorf("%s %s: still unauthorized after refreshing the access token", req.Method, req.URL)
	}
	return resp, nil
}
