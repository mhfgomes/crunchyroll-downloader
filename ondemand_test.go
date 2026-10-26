package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type eofReader struct{}

func (eofReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestStreamRangeResumesWithoutDuplicateBytes(t *testing.T) {
	for _, ignoreRange := range []bool{false, true} {
		t.Run(fmt.Sprint(ignoreRange), func(t *testing.T) {
			const payload = "0123456789abcdef"
			calls := 0
			mockPlayback(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					if r.Header.Get("Range") != "bytes=2-8388609" {
						t.Errorf("initial Range=%q", r.Header.Get("Range"))
					}
					resp := playbackResponse(r, 206, "")
					resp.Header.Set("Content-Range", "bytes 2-15/16")
					resp.Body = io.NopCloser(io.MultiReader(strings.NewReader(payload[2:6]), eofReader{}))
					return resp, nil
				}
				if r.Header.Get("Range") != "bytes=6-15" {
					t.Errorf("resume Range=%q", r.Header.Get("Range"))
				}
				if ignoreRange {
					resp := playbackResponse(r, 200, payload)
					resp.ContentLength = int64(len(payload))
					return resp, nil
				}
				resp := playbackResponse(r, 206, payload[6:])
				resp.Header.Set("Content-Range", "bytes 6-15/16")
				return resp, nil
			})
			var output bytes.Buffer
			if err := streamRange(&output, "https://cdn.example/media", 2, nil); err != nil {
				t.Fatal(err)
			}
			if output.String() != payload[2:] || calls != 2 {
				t.Fatalf("output=%q calls=%d", output.String(), calls)
			}
		})
	}
}

func TestStreamRangeUsesBoundedChunks(t *testing.T) {
	payload := bytes.Repeat([]byte("m"), 8*1024*1024+17)
	calls := 0
	mockPlayback(t, func(r *http.Request) (*http.Response, error) {
		calls++
		var start, end int
		if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end); err != nil {
			t.Fatal(err)
		}
		if end-start+1 > 8*1024*1024 {
			t.Fatalf("unbounded range %q", r.Header.Get("Range"))
		}
		end = min(end, len(payload)-1)
		resp := playbackResponse(r, 206, "")
		resp.Body = io.NopCloser(bytes.NewReader(payload[start : end+1]))
		resp.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload)))
		return resp, nil
	})
	var result bytes.Buffer
	var last int64
	err := streamRange(&result, "https://cdn.example/media", 3, func(current, total int64) {
		if current < last || total != int64(len(payload)) {
			t.Errorf("invalid progress %d/%d after %d", current, total, last)
		}
		last = current
	})
	if err != nil || calls != 2 || !bytes.Equal(result.Bytes(), payload[3:]) {
		t.Fatalf("calls=%d bytes=%d err=%v", calls, result.Len(), err)
	}
}

func TestStreamRangeRejectsWrongOffset(t *testing.T) {
	mockPlayback(t, func(r *http.Request) (*http.Response, error) {
		resp := playbackResponse(r, 206, "wrong bytes")
		resp.Header.Set("Content-Range", "bytes 0-9/10")
		return resp, nil
	})
	var result bytes.Buffer
	if err := streamRange(&result, "https://cdn.example/media", 5, nil); err == nil || result.Len() != 0 {
		t.Fatalf("invalid range accepted, bytes=%d err=%v", result.Len(), err)
	}
}

func TestParseByteRange(t *testing.T) {
	tests := []struct {
		in        string
		wantStart int64
		wantEnd   int64
		wantErr   bool
	}{
		{"0-1837", 0, 1837, false},
		{"1838-6129", 1838, 6129, false},
		{"1708-6011", 1708, 6011, false},
		{"12345-", 0, 0, true},
		{"", 0, 0, true},
		{"abc-def", 0, 0, true},
	}

	for _, tc := range tests {
		start, end, err := parseByteRange(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseByteRange(%q) = %d,%d, nil; want error", tc.in, start, end)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseByteRange(%q) returned error: %v", tc.in, err)
			continue
		}
		if start != tc.wantStart || end != tc.wantEnd {
			t.Errorf("parseByteRange(%q) = %d,%d; want %d,%d", tc.in, start, end, tc.wantStart, tc.wantEnd)
		}
	}
}

func TestParseOnDemand(t *testing.T) {
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" profiles="urn:mpeg:dash:profile:isoff-on-demand:2011" type="static">
  <Period>
    <AdaptationSet contentType="video" mimeType="video/mp4" segmentAlignment="true">
      <Representation id="84df326d" bandwidth="4386044" width="1280" height="720" codecs="avc1.64001F">
        <BaseURL>https://example.com/video.mp4</BaseURL>
        <SegmentBase indexRange="1838-6129">
          <Initialization range="0-1837"/>
        </SegmentBase>
      </Representation>
      <Representation id="b32ce5ea" bandwidth="16481538" width="1920" height="1080" codecs="avc1.640028">
        <BaseURL>https://example.com/video-1080.mp4</BaseURL>
        <SegmentBase indexRange="1839-6130">
          <Initialization range="0-1838"/>
        </SegmentBase>
      </Representation>
    </AdaptationSet>
    <AdaptationSet contentType="audio" mimeType="audio/mp4" segmentAlignment="true">
      <Representation id="433db54e" bandwidth="199094" codecs="mp4a.40.2" audioSamplingRate="48000">
        <BaseURL>https://example.com/audio.mp4</BaseURL>
        <SegmentBase indexRange="1708-6011">
          <Initialization range="0-1707"/>
        </SegmentBase>
      </Representation>
    </AdaptationSet>
  </Period>
</MPD>`)

	sets, err := parseOnDemand(body)
	if err != nil {
		t.Fatalf("parseOnDemand returned error: %v", err)
	}
	if len(sets) != 2 {
		t.Fatalf("got %d adaptation sets, want 2", len(sets))
	}

	if !sets[0].IsVideo || sets[1].IsVideo {
		t.Fatalf("adaptation set video flags = %v,%v; want true,false", sets[0].IsVideo, sets[1].IsVideo)
	}

	if len(sets[0].Representations) != 2 {
		t.Fatalf("video has %d representations, want 2", len(sets[0].Representations))
	}
	rep := sets[0].Representations[1]
	if rep.Height != 1080 || rep.InitRange != "0-1838" || rep.IndexRange != "1839-6130" {
		t.Fatalf("video 1080p representation = %+v", rep)
	}

	audio := sets[1].Representations[0]
	if audio.Bandwidth != 199094 || audio.InitRange != "0-1707" || audio.IndexRange != "1708-6011" {
		t.Fatalf("audio representation = %+v", audio)
	}
}

func TestResponseRange(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		length    int64
		wantTotal int64
		wantStart int64
		wantOK    bool
	}{
		{"206 with content-range", "bytes 123456-999999/1000000", 0, 1000000, 123456, true},
		{"206 zero start", "bytes 0-999/1000", 0, 1000, 0, true},
		{"no content-range, known length", "", 5000, 5000, 0, true},
		{"no length info", "", -1, 0, 0, false},
		{"malformed content-range", "bytes foo-bar/baz", 0, 0, 0, false},
		{"missing bytes prefix", "0-999/1000", 0, 0, 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}, ContentLength: tc.length}
			if tc.header != "" {
				resp.Header.Set("Content-Range", tc.header)
			}
			total, start, ok := responseRange(resp)
			if ok != tc.wantOK {
				t.Fatalf("responseRange() ok = %v, want %v", ok, tc.wantOK)
			}
			if total != tc.wantTotal || start != tc.wantStart {
				t.Fatalf("responseRange() = %d,%d; want %d,%d", total, start, tc.wantTotal, tc.wantStart)
			}
		})
	}
}

func TestSelectOnDemandRepresentation(t *testing.T) {
	video := onDemandAdaptationSet{
		IsVideo: true,
		Representations: []onDemandRepresentation{
			{ID: "720p", Height: 720},
			{ID: "1080p", Height: 1080},
		},
	}
	if rep, ok := selectOnDemandRepresentation(video, "1080p", true); !ok || rep.ID != "1080p" {
		t.Fatalf("selectOnDemandRepresentation(video, 1080p) = %+v, %v", rep, ok)
	}

	audio := onDemandAdaptationSet{
		Representations: []onDemandRepresentation{
			{ID: "128k", Bandwidth: 135875},
			{ID: "192k", Bandwidth: 200315},
		},
	}
	if rep, ok := selectOnDemandRepresentation(audio, "192k", false); !ok || rep.ID != "192k" {
		t.Fatalf("selectOnDemandRepresentation(audio, 192k) = %+v, %v", rep, ok)
	}
}
