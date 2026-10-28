package downloader

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Use real media timestamps: checking the argument list alone would miss
// FFmpeg independently resetting each input's presentation timeline.
func TestMergePreservesPresentationTimeline(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not installed")
	}
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
			t.Fatalf("ffmpeg: %v\n%s", err, out)
		}
	}
	firstPTS := func(file, stream string) float64 {
		t.Helper()
		out, err := exec.Command(ffprobe, "-v", "error", "-select_streams", stream,
			"-read_intervals", "%+#1", "-show_entries", "packet=pts_time", "-of", "csv=p=0", file).CombinedOutput()
		if err != nil {
			t.Fatalf("ffprobe: %v\n%s", err, out)
		}
		pts, err := strconv.ParseFloat(strings.TrimSpace(strings.SplitN(string(out), ",", 2)[0]), 64)
		if err != nil {
			t.Fatalf("parse first timestamp %q: %v", out, err)
		}
		return pts
	}
	dir := t.TempDir()
	video := filepath.Join(dir, "video.mp4")
	audio := filepath.Join(dir, "audio.m4a")
	primedAudio := filepath.Join(dir, "primed-audio.m4a")
	subtitles := filepath.Join(dir, "subtitles.srt")
	output := filepath.Join(dir, "merged.mkv")
	run("-v", "error", "-f", "lavfi", "-i", "color=size=32x32:rate=25:duration=2",
		"-c:v", "mpeg4", video)
	run("-v", "error", "-f", "lavfi", "-i", "sine=duration=1:sample_rate=48000",
		"-c:a", "aac", "-output_ts_offset", "0.5", audio)
	run("-v", "error", "-f", "lavfi", "-i", "sine=duration=1:sample_rate=48000",
		"-c:a", "aac", primedAudio)
	if err := os.WriteFile(subtitles, []byte("1\n00:00:00,750 --> 00:00:01,500\nTest cue\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wantAudio := firstPTS(audio, "a:0")
	wantPrimedAudio := firstPTS(primedAudio, "a:0")
	wantVideo := firstPTS(video, "v:0")
	oldFFmpeg := ffmpegPath
	ffmpegPath = ffmpeg
	t.Cleanup(func() { ffmpegPath = oldFFmpeg })
	mergeEverything(video, []mediaTrack{{file: audio, locale: "ja-JP"}, {file: primedAudio, locale: "en-US"}},
		[]mediaTrack{{file: subtitles, locale: "en-US", format: "srt"}}, output, EpisodeInfo{})
	for _, track := range []struct {
		stream string
		want   float64
	}{{"v:0", wantVideo}, {"a:0", wantAudio}, {"a:1", wantPrimedAudio}, {"s:0", 0.75}} {
		if got := firstPTS(output, track.stream); math.Abs(got-track.want) > 0.002 {
			t.Errorf("%s first PTS = %.6f, want %.6f", track.stream, got, track.want)
		}
	}
}
