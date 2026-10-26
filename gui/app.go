package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"sync"
	"time"

	"crunchyroll-downloader/downloader"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Settings is everything the GUI persists between launches.
type Settings struct {
	EtpRt      string             `json:"etpRt"`
	FFmpegPath string             `json:"ffmpegPath"`
	Options    downloader.Options `json:"options"`
}

// JobStatus is the lifecycle of a queued episode.
type JobStatus string

const (
	JobQueued      JobStatus = "queued"
	JobDownloading JobStatus = "downloading"
	JobMerging     JobStatus = "merging"
	JobWaiting     JobStatus = "waiting"
	JobDone        JobStatus = "done"
	JobSkipped     JobStatus = "skipped"
	JobFailed      JobStatus = "failed"
	JobCancelled   JobStatus = "cancelled"
)

// Track is one in-flight track download of a job.
type Track struct {
	ID       int64   `json:"id"`
	Title    string  `json:"title"`
	Label    string  `json:"label"`
	Current  int64   `json:"current"`
	Total    int64   `json:"total"`
	RateMbps float64 `json:"rateMbps"`
}

// Job is one episode in the download queue.
type Job struct {
	ID      string           `json:"id"`
	Title   downloader.Title `json:"title"`
	Status  JobStatus        `json:"status"`
	Error   string           `json:"error,omitempty"`
	Output  string           `json:"output,omitempty"`
	RetryAt *time.Time       `json:"retryAt,omitempty"`
	Tracks  []Track          `json:"tracks"`
}

// Environment reports what the downloader needs before it can run.
type Environment struct {
	FFmpegPath  string `json:"ffmpegPath"`
	FFmpegError string `json:"ffmpegError,omitempty"`
	Widevine    bool   `json:"widevine"`
	LoggedIn    bool   `json:"loggedIn"`
	LoginError  string `json:"loginError,omitempty"`
}

// Language is a selectable locale.
type Language struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// LogLine is a timestamped message from the downloader.
type LogLine struct {
	Time string `json:"time"`
	Text string `json:"text"`
}

// App is bound to the frontend.
type App struct {
	ctx context.Context

	mu        sync.Mutex
	settings  Settings
	env       Environment
	jobs      []*Job
	nextID    int
	current   *Job
	cancel    context.CancelFunc
	wake      chan struct{}
	lastEmit  time.Time
	emitTimer *time.Timer
	logs      []LogLine
}

func NewApp() *App {
	return &App{wake: make(chan struct{}, 1)}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.settings = loadSettings()
	if err := downloader.Configure(a.settings.Options); err != nil {
		a.settings.Options = downloader.DefaultOptions()
		_ = downloader.Configure(a.settings.Options)
	}
	downloader.SetHooks(downloader.Hooks{
		Log:      a.onLog,
		Progress: a.onProgress,
		Episode:  a.onEpisode,
	})
	go a.worker()
}

func settingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "crunchyroll-downloader", "settings.json"), nil
}

func loadSettings() Settings {
	s := Settings{Options: downloader.DefaultOptions()}
	if home, err := os.UserHomeDir(); err == nil {
		s.Options.OutputDir = filepath.Join(home, "Videos", "Crunchyroll")
	}
	path, err := settingsPath()
	if err != nil {
		return s
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s)
	return s
}

func (a *App) saveSettingsFile() error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(a.settings, "", "  ")
	if err != nil {
		return err
	}
	// The file holds the account cookie, so keep it private to the user.
	return os.WriteFile(path, data, 0o600)
}

// GetSettings returns the saved settings.
func (a *App) GetSettings() Settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settings
}

// DefaultOptions returns the downloader defaults.
func (a *App) DefaultOptions() downloader.Options {
	return downloader.DefaultOptions()
}

// SaveSettings validates, applies and persists settings. It blocks until any
// running download finishes with the old options.
func (a *App) SaveSettings(s Settings) (Environment, error) {
	if err := downloader.Configure(s.Options); err != nil {
		return a.Environment(), err
	}
	a.mu.Lock()
	cookieChanged := s.EtpRt != a.settings.EtpRt
	a.settings = s
	err := a.saveSettingsFile()
	a.mu.Unlock()
	if err != nil {
		return a.Environment(), fmt.Errorf("saving settings: %w", err)
	}
	if cookieChanged {
		return a.CheckEnvironment(), nil
	}
	return a.refreshEnvironment(false), nil
}

// Environment returns the last environment check without repeating it.
func (a *App) Environment() Environment {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.env
}

// CheckEnvironment locates ffmpeg and the CDM and logs in.
func (a *App) CheckEnvironment() Environment {
	return a.refreshEnvironment(true)
}

func (a *App) refreshEnvironment(login bool) Environment {
	a.mu.Lock()
	s := a.settings
	env := a.env
	a.mu.Unlock()

	env.FFmpegError = ""
	if s.FFmpegPath != "" {
		if _, err := os.Stat(s.FFmpegPath); err != nil {
			env.FFmpegPath, env.FFmpegError = "", "The configured ffmpeg path does not exist."
		} else {
			env.FFmpegPath = s.FFmpegPath
			downloader.SetFFmpegPath(s.FFmpegPath)
		}
	} else if path, err := downloader.FindFFmpeg(); err != nil {
		env.FFmpegPath, env.FFmpegError = "", "FFmpeg wasn't found on PATH or next to the app."
	} else {
		env.FFmpegPath = path
	}
	env.Widevine = downloader.HasWidevineDevice()

	if login {
		env.LoggedIn, env.LoginError = false, ""
		switch err := downloader.Login(s.EtpRt); {
		case s.EtpRt == "":
			env.LoginError = "No etp_rt cookie saved yet."
		case errors.Is(err, downloader.ErrAuthRejected):
			env.LoginError = "Crunchyroll rejected the etp_rt cookie. Copy a fresh one from your browser."
		case err != nil:
			env.LoginError = err.Error()
		default:
			env.LoggedIn = true
		}
	}

	a.mu.Lock()
	a.env = env
	a.mu.Unlock()
	return env
}

// Languages lists the known locales and their display names.
func (a *App) Languages() []Language {
	var out []Language
	for _, code := range downloader.Languages() {
		out = append(out, Language{Code: code, Name: downloader.LanguageName(code)})
	}
	return out
}

// Lookup resolves a Crunchyroll URL.
func (a *App) Lookup(url string) (downloader.LookupResult, error) {
	return downloader.Lookup(url)
}

// SeasonEpisodes lists a season's episodes.
func (a *App) SeasonEpisodes(seasonID string) ([]downloader.Title, error) {
	return downloader.SeasonEpisodes(seasonID)
}

// Enqueue adds titles to the download queue, skipping ones already waiting.
func (a *App) Enqueue(titles []downloader.Title) []*Job {
	a.mu.Lock()
	pending := map[string]bool{}
	for _, j := range a.jobs {
		if isActive(j.Status) {
			pending[j.Title.ID] = true
		}
	}
	for _, t := range titles {
		if pending[t.ID] {
			continue
		}
		a.nextID++
		a.jobs = append(a.jobs, &Job{ID: strconv.Itoa(a.nextID), Title: t, Status: JobQueued, Tracks: []Track{}})
	}
	a.mu.Unlock()
	a.signal()
	a.emitQueue(true)
	return a.Queue()
}

func isActive(s JobStatus) bool {
	return s == JobQueued || s == JobDownloading || s == JobMerging || s == JobWaiting
}

// Queue returns a snapshot of the download queue.
func (a *App) Queue() []*Job {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.snapshotLocked()
}

func (a *App) snapshotLocked() []*Job {
	out := make([]*Job, len(a.jobs))
	for i, j := range a.jobs {
		c := *j
		c.Tracks = append([]Track{}, j.Tracks...)
		out[i] = &c
	}
	return out
}

// Cancel stops a running job or drops a queued one.
func (a *App) Cancel(id string) {
	a.mu.Lock()
	for _, j := range a.jobs {
		if j.ID != id {
			continue
		}
		if j == a.current && a.cancel != nil {
			a.cancel()
		} else if j.Status == JobQueued {
			j.Status = JobCancelled
		}
	}
	a.mu.Unlock()
	a.emitQueue(true)
}

// Retry re-queues a failed or cancelled job.
func (a *App) Retry(id string) {
	a.mu.Lock()
	for _, j := range a.jobs {
		if j.ID == id && (j.Status == JobFailed || j.Status == JobCancelled) {
			j.Status, j.Error, j.RetryAt, j.Tracks = JobQueued, "", nil, []Track{}
		}
	}
	a.mu.Unlock()
	a.signal()
	a.emitQueue(true)
}

// Remove drops a job that is not running.
func (a *App) Remove(id string) {
	a.mu.Lock()
	for i, j := range a.jobs {
		if j.ID == id && j != a.current {
			a.jobs = append(a.jobs[:i], a.jobs[i+1:]...)
			break
		}
	}
	a.mu.Unlock()
	a.emitQueue(true)
}

// ClearFinished removes every job that is no longer pending.
func (a *App) ClearFinished() {
	a.mu.Lock()
	kept := a.jobs[:0]
	for _, j := range a.jobs {
		if isActive(j.Status) {
			kept = append(kept, j)
		}
	}
	a.jobs = kept
	a.mu.Unlock()
	a.emitQueue(true)
}

// Logs returns the retained log lines.
func (a *App) Logs() []LogLine {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]LogLine{}, a.logs...)
}

func (a *App) signal() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

// worker downloads queued jobs one at a time.
func (a *App) worker() {
	for {
		a.mu.Lock()
		var job *Job
		for _, j := range a.jobs {
			if j.Status == JobQueued {
				job = j
				break
			}
		}
		if job == nil {
			a.mu.Unlock()
			<-a.wake
			continue
		}
		ctx, cancel := context.WithCancel(a.ctx)
		a.current, a.cancel = job, cancel
		job.Status, job.Error = JobDownloading, ""
		a.mu.Unlock()
		a.emitQueue(true)

		err := downloader.DownloadTitle(ctx, job.Title)

		a.mu.Lock()
		switch {
		case ctx.Err() != nil:
			job.Status = JobCancelled
		case err != nil:
			job.Status, job.Error = JobFailed, err.Error()
		case job.Status != JobSkipped:
			job.Status = JobDone
		}
		job.Tracks, job.RetryAt = []Track{}, nil
		a.current, a.cancel = nil, nil
		a.mu.Unlock()
		cancel()
		a.emitQueue(true)
	}
}

func (a *App) onLog(line string) {
	l := LogLine{Time: time.Now().Format("15:04:05"), Text: line}
	a.mu.Lock()
	a.logs = append(a.logs, l)
	if len(a.logs) > 2000 {
		a.logs = a.logs[len(a.logs)-2000:]
	}
	a.mu.Unlock()
	runtime.EventsEmit(a.ctx, "log", l)
}

func (a *App) onProgress(ev downloader.ProgressEvent) {
	a.mu.Lock()
	job := a.current
	if job != nil {
		idx := -1
		for i, t := range job.Tracks {
			if t.ID == ev.ID {
				idx = i
			}
		}
		switch {
		case ev.Done && idx >= 0:
			job.Tracks = append(job.Tracks[:idx], job.Tracks[idx+1:]...)
		case !ev.Done && idx >= 0:
			job.Tracks[idx] = trackFrom(ev)
		case !ev.Done:
			job.Tracks = append(job.Tracks, trackFrom(ev))
		}
	}
	a.mu.Unlock()
	a.emitQueue(false)
}

func trackFrom(ev downloader.ProgressEvent) Track {
	return Track{ID: ev.ID, Title: ev.Title, Label: ev.Label, Current: ev.Current, Total: ev.Total, RateMbps: ev.RateMbps}
}

func (a *App) onEpisode(ev downloader.EpisodeEvent) {
	a.mu.Lock()
	if job := a.current; job != nil && job.Title.ID == ev.ID {
		switch ev.Status {
		case downloader.StatusStarted:
			job.Status, job.RetryAt = JobDownloading, nil
		case downloader.StatusMerging:
			job.Status = JobMerging
		case downloader.StatusRetrying:
			job.Status, job.RetryAt, job.Error = JobWaiting, ev.RetryAt, ev.Error
		case downloader.StatusSkipped:
			job.Status, job.Output = JobSkipped, ev.Output
		case downloader.StatusDone:
			job.Output = ev.Output
		}
	}
	a.mu.Unlock()
	a.emitQueue(true)
}

// emitQueue pushes the queue to the frontend, throttling progress-only updates
// to about ten per second.
func (a *App) emitQueue(force bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !force && time.Since(a.lastEmit) < 100*time.Millisecond {
		if a.emitTimer == nil {
			a.emitTimer = time.AfterFunc(100*time.Millisecond, func() {
				a.mu.Lock()
				a.emitTimer = nil
				a.mu.Unlock()
				a.emitQueue(true)
			})
		}
		return
	}
	a.lastEmit = time.Now()
	runtime.EventsEmit(a.ctx, "queue", a.snapshotLocked())
}

// PickDirectory opens a folder picker, returning "" when cancelled.
func (a *App) PickDirectory(title, current string) (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: title, DefaultDirectory: existingDir(current)})
}

// PickFile opens a file picker, returning "" when cancelled.
func (a *App) PickFile(title, current, pattern string) (string, error) {
	opts := runtime.OpenDialogOptions{Title: title, DefaultDirectory: existingDir(filepath.Dir(current))}
	if pattern != "" {
		opts.Filters = []runtime.FileFilter{{DisplayName: pattern, Pattern: pattern}}
	}
	return runtime.OpenFileDialog(a.ctx, opts)
}

func existingDir(dir string) string {
	if dir == "" {
		return ""
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		return dir
	}
	return ""
}

// Reveal shows a file in the system file manager.
func (a *App) Reveal(path string) error {
	switch goruntime.GOOS {
	case "windows":
		return exec.Command("explorer", "/select,", path).Start()
	case "darwin":
		return exec.Command("open", "-R", path).Start()
	default:
		return exec.Command("xdg-open", filepath.Dir(path)).Start()
	}
}

// OpenURL opens a link in the default browser.
func (a *App) OpenURL(url string) {
	runtime.BrowserOpenURL(a.ctx, url)
}

// Search finds series by title.
func (a *App) Search(query string) ([]downloader.SearchResult, error) {
	return downloader.Search(query)
}

// Browse lists catalogue series sorted by "popularity" or "newly_added".
func (a *App) Browse(sortBy string, n int) ([]downloader.SearchResult, error) {
	return downloader.Browse(sortBy, n)
}
