package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"crunchyroll-downloader/downloader"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Account is a saved Crunchyroll login.
type Account struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	EtpRt string `json:"etpRt"`
	// NoMixer leaves the account out of the mixer; jobs pinned to it still
	// use it.
	NoMixer bool `json:"noMixer,omitempty"`
}

// Settings is everything the GUI persists between launches.
type Settings struct {
	// EtpRt is the single cookie saved by earlier versions; normalize moves it
	// into Accounts.
	EtpRt         string    `json:"etpRt,omitempty"`
	Accounts      []Account `json:"accounts"`
	ActiveAccount string    `json:"activeAccount"`
	// Mixer spreads episodes queued without a fixed account over every
	// account; see mixer.go.
	Mixer bool `json:"mixer"`
	// RateLimitTimeout is how long an account sits out after Crunchyroll
	// rate-limits it. Zero means automatic: a minute, doubling on hits in a
	// row up to 30 minutes.
	RateLimitTimeout time.Duration      `json:"rateLimitTimeout"`
	FFmpegPath       string             `json:"ffmpegPath"`
	Options          downloader.Options `json:"options"`
}

// account returns the account with the given ID, or nil.
func (s Settings) account(id string) *Account {
	for i := range s.Accounts {
		if s.Accounts[i].ID == id {
			return &s.Accounts[i]
		}
	}
	return nil
}

// normalize migrates the legacy cookie and keeps ActiveAccount pointing at a
// saved account whenever there is one.
func (s *Settings) normalize() {
	if s.EtpRt != "" {
		if len(s.Accounts) == 0 {
			s.Accounts = []Account{{ID: "1", Name: "Main", EtpRt: s.EtpRt}}
		}
		s.EtpRt = ""
	}
	if s.Accounts == nil {
		s.Accounts = []Account{}
	}
	if s.account(s.ActiveAccount) == nil {
		s.ActiveAccount = ""
		if len(s.Accounts) > 0 {
			s.ActiveAccount = s.Accounts[0].ID
		}
	}
}

// activeCookie is the etp_rt cookie of the active account.
func (s Settings) activeCookie() string {
	if acc := s.account(s.ActiveAccount); acc != nil {
		return acc.EtpRt
	}
	return ""
}

// JobStatus is the lifecycle of a queued episode.
type JobStatus string

const (
	JobQueued      JobStatus = "queued"
	JobDownloading JobStatus = "downloading"
	JobMerging     JobStatus = "merging"
	JobWaiting     JobStatus = "waiting"
	JobPaused      JobStatus = "paused"
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
	ID    string           `json:"id"`
	Title downloader.Title `json:"title"`
	// AccountID is the account the job downloads with. Empty, or an account
	// that was since removed, means the active one.
	AccountID string     `json:"accountId,omitempty"`
	Status    JobStatus  `json:"status"`
	Error     string     `json:"error,omitempty"`
	Output    string     `json:"output,omitempty"`
	RetryAt   *time.Time `json:"retryAt,omitempty"`
	Tracks    []Track    `json:"tracks"`
}

// QueueState is the queue as the frontend sees it.
type QueueState struct {
	Paused bool   `json:"paused"`
	Jobs   []*Job `json:"jobs"`
	// Mixer reports whether downloads are spread over accounts, and
	// Accounts how each one is doing.
	Mixer    bool            `json:"mixer"`
	Accounts []AccountStatus `json:"accounts"`
	// WaitUntil is set while every account the queue could use is resting.
	WaitUntil *time.Time `json:"waitUntil,omitempty"`
}

// stopReason records why the running job's context was cancelled.
type stopReason int

const (
	stopCancel stopReason = iota
	stopPause
	stopSkip
	stopQueue // the whole queue was paused; the job goes back to the front
)

// Environment reports what the downloader needs before it can run.
type Environment struct {
	FFmpegPath  string `json:"ffmpegPath"`
	FFmpegError string `json:"ffmpegError,omitempty"`
	Widevine    bool   `json:"widevine"`
	LoggedIn    bool   `json:"loggedIn"`
	Account     string `json:"account,omitempty"`
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
	ctx   context.Context
	store *store

	mu        sync.Mutex
	paused    bool
	stop      stopReason
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
	// loginMu guards session, the cookie the downloader is logged in with.
	loginMu sync.Mutex
	session string

	// accState is the mixer's view of each account, and currentAccount the
	// account of the running job.
	accState       map[string]*accountState
	currentAccount string
	waitUntil      *time.Time
}

func NewApp() *App {
	return &App{wake: make(chan struct{}, 1), accState: map[string]*accountState{}}
}

// configure applies s to the downloader. With the mixer on, the pause
// between episodes is kept per account here instead of globally.
func configure(s Settings) error {
	opts := s.Options
	if s.mixerOn() {
		opts.DownloadDelay = 0
	}
	return downloader.Configure(opts)
}

// ServiceStartup runs when the Wails application starts.
func (a *App) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	a.ctx = ctx
	st, err := openStore()
	if err != nil {
		return fmt.Errorf("opening the database: %w", err)
	}
	a.store = st
	a.settings = st.loadSettings(defaultSettings())
	a.settings.normalize()
	if jobs, paused, err := st.loadQueue(); err == nil {
		a.jobs, a.paused = jobs, paused
		for _, j := range jobs {
			if n, _ := strconv.Atoi(j.ID); n > a.nextID {
				a.nextID = n
			}
		}
	}
	if state, err := st.loadAccountState(); err == nil {
		a.accState = state
	}
	if err := configure(a.settings); err != nil {
		a.settings.Options = downloader.DefaultOptions()
		_ = configure(a.settings)
	}
	downloader.SetHooks(downloader.Hooks{
		Log:      a.onLog,
		Progress: a.onProgress,
		Episode:  a.onEpisode,
	})
	go a.worker()
	return nil
}

// ServiceShutdown saves the queue and closes the database.
func (a *App) ServiceShutdown() error {
	if a.store == nil {
		return nil
	}
	a.mu.Lock()
	err := errors.Join(a.store.saveQueue(a.jobs, a.paused), a.store.saveAccountState(a.accState))
	a.mu.Unlock()
	return errors.Join(err, a.store.Close())
}

func defaultSettings() Settings {
	s := Settings{Options: downloader.DefaultOptions()}
	if home, err := os.UserHomeDir(); err == nil {
		s.Options.OutputDir = filepath.Join(home, "Videos", "Crunchyroll")
	}
	return s
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
	if err := a.checkUnlocked(); err != nil {
		return a.Environment(), err
	}
	s.normalize()
	if err := configure(s); err != nil {
		return a.Environment(), err
	}
	ids := map[string]bool{}
	for _, acc := range s.Accounts {
		if acc.ID == "" || ids[acc.ID] {
			return a.Environment(), errors.New("every account needs a unique ID")
		}
		ids[acc.ID] = true
	}
	a.mu.Lock()
	cookieChanged := s.activeCookie() != a.settings.activeCookie()
	a.settings = s
	err := a.store.saveSettings(s)
	a.mu.Unlock()
	if err != nil {
		return a.Environment(), fmt.Errorf("saving settings: %w", err)
	}
	// The mixer or the accounts may have changed which jobs can start.
	a.signal()
	a.emitQueue(true)
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

// SwitchAccount makes id the active account, which new jobs are queued with,
// and signs in with it.
func (a *App) SwitchAccount(id string) (Environment, error) {
	if err := a.checkUnlocked(); err != nil {
		return a.Environment(), err
	}
	a.mu.Lock()
	s := a.settings
	a.mu.Unlock()
	if s.account(id) == nil {
		return a.Environment(), errors.New("that account no longer exists")
	}
	s.Accounts = slices.Clone(s.Accounts)
	s.ActiveAccount = id
	return a.SaveSettings(s)
}

// login signs the downloader in with cookie unless it already is. The worker
// calls it after setting a.current.
func (a *App) login(cookie string) error {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	if cookie == a.session {
		return nil
	}
	if err := downloader.Login(cookie); err != nil {
		return err
	}
	a.session = cookie
	return nil
}

// checkLogin verifies cookie with Crunchyroll. When nothing is downloading it
// also signs in with it, so browsing uses that account; otherwise the running
// download's session, which may belong to another account, is left alone.
func (a *App) checkLogin(cookie string) error {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	a.mu.Lock()
	busy := a.current != nil
	a.mu.Unlock()
	if busy {
		_, err := downloader.GetAccessToken(cookie)
		return err
	}
	a.session = ""
	if err := downloader.Login(cookie); err != nil {
		return err
	}
	a.session = cookie
	return nil
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

	env.Account = ""
	if acc := s.account(s.ActiveAccount); acc != nil {
		env.Account = acc.Name
	}
	if login {
		env.LoggedIn, env.LoginError = false, ""
		cookie := s.activeCookie()
		var err error
		if cookie != "" {
			err = a.checkLogin(cookie)
		}
		switch {
		case len(s.Accounts) == 0:
			env.LoginError = "No account added yet."
		case cookie == "":
			env.LoginError = "The active account has no etp_rt cookie."
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
func (a *App) Enqueue(titles []downloader.Title) QueueState {
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
		a.jobs = append(a.jobs, &Job{
			ID: strconv.Itoa(a.nextID), Title: t, AccountID: a.queueAccountLocked(),
			Status: JobQueued, Tracks: []Track{},
		})
	}
	a.mu.Unlock()
	a.signal()
	a.emitQueue(true)
	return a.Queue()
}

// queueAccountLocked is the account new jobs are tied to: none with the
// mixer on, so it can choose, otherwise the active one.
func (a *App) queueAccountLocked() string {
	if a.settings.mixerOn() {
		return ""
	}
	return a.settings.ActiveAccount
}

func isActive(s JobStatus) bool {
	return s == JobQueued || s == JobDownloading || s == JobMerging || s == JobWaiting || s == JobPaused
}

// Queue returns a snapshot of the download queue.
func (a *App) Queue() QueueState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.snapshotLocked()
}

func (a *App) snapshotLocked() QueueState {
	out := make([]*Job, len(a.jobs))
	for i, j := range a.jobs {
		c := *j
		c.Tracks = append([]Track{}, j.Tracks...)
		out[i] = &c
	}
	return QueueState{
		Paused: a.paused, Jobs: out,
		Mixer: a.settings.mixerOn(), Accounts: a.accountStatusLocked(time.Now()), WaitUntil: a.waitUntil,
	}
}

// errSettingsLocked is returned when settings change while the queue runs.
var errSettingsLocked = errors.New("pause the queue before changing settings")

// runningLocked reports whether the queue is working: something is
// downloading, or it isn't paused and has episodes waiting to start. Settings
// are locked meanwhile so a download never sees them change halfway.
func (a *App) runningLocked() bool {
	if a.current != nil {
		return true
	}
	if a.paused {
		return false
	}
	for _, j := range a.jobs {
		if j.Status == JobQueued || j.Status == JobWaiting {
			return true
		}
	}
	return false
}

func (a *App) checkUnlocked() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.runningLocked() {
		return errSettingsLocked
	}
	return nil
}

// update runs fn under the lock, then wakes the worker and publishes the queue.
func (a *App) update(fn func()) {
	a.mu.Lock()
	fn()
	a.mu.Unlock()
	a.signal()
	a.emitQueue(true)
}

func (a *App) findLocked(id string) (int, *Job) {
	for i, j := range a.jobs {
		if j.ID == id {
			return i, j
		}
	}
	return -1, nil
}

// stopCurrentLocked cancels the running job, recording why.
func (a *App) stopCurrentLocked(why stopReason) {
	if a.cancel != nil {
		a.stop = why
		a.cancel()
	}
}

// PauseAll stops the queue. The running episode is interrupted and will start
// over when the queue resumes.
func (a *App) PauseAll() {
	a.update(func() {
		a.paused = true
		a.stopCurrentLocked(stopQueue)
	})
}

// ResumeAll restarts the queue.
func (a *App) ResumeAll() {
	a.update(func() { a.paused = false })
}

// Pause holds one job so the queue moves past it.
func (a *App) Pause(id string) {
	a.update(func() {
		_, j := a.findLocked(id)
		switch {
		case j == nil:
		case j == a.current:
			a.stopCurrentLocked(stopPause)
		case j.Status == JobQueued:
			j.Status = JobPaused
		}
	})
}

// Resume puts a paused job back in the queue.
func (a *App) Resume(id string) {
	a.update(func() {
		if _, j := a.findLocked(id); j != nil && j.Status == JobPaused {
			j.Status = JobQueued
		}
	})
}

// Skip passes over a job without downloading it.
func (a *App) Skip(id string) {
	a.update(func() {
		_, j := a.findLocked(id)
		switch {
		case j == nil:
		case j == a.current:
			a.stopCurrentLocked(stopSkip)
		case j.Status == JobQueued || j.Status == JobPaused:
			j.Status, j.Error = JobSkipped, "Skipped"
		}
	})
}

// Move shifts a job to index to in the queue.
func (a *App) Move(id string, to int) {
	a.update(func() {
		i, j := a.findLocked(id)
		if j == nil {
			return
		}
		to = max(0, min(to, len(a.jobs)-1))
		a.jobs = append(a.jobs[:i], a.jobs[i+1:]...)
		a.jobs = append(a.jobs[:to], append([]*Job{j}, a.jobs[to:]...)...)
	})
}

// SortPending reorders the jobs that haven't started yet, leaving running and
// finished jobs where they are. by is "episode" (series, season, number),
// "title", "added" or "reverse".
func (a *App) SortPending(by string) {
	a.update(func() {
		var slots []int
		var pending []*Job
		for i, j := range a.jobs {
			if (j.Status == JobQueued || j.Status == JobPaused) && j != a.current {
				slots = append(slots, i)
				pending = append(pending, j)
			}
		}
		added := func(j *Job) int { n, _ := strconv.Atoi(j.ID); return n }
		switch by {
		case "reverse":
			slices.Reverse(pending)
		case "title":
			slices.SortStableFunc(pending, func(x, y *Job) int {
				return cmp.Compare(strings.ToLower(x.Title.Title), strings.ToLower(y.Title.Title))
			})
		case "added":
			slices.SortStableFunc(pending, func(x, y *Job) int { return cmp.Compare(added(x), added(y)) })
		default:
			slices.SortStableFunc(pending, func(x, y *Job) int {
				return cmp.Or(
					cmp.Compare(strings.ToLower(x.Title.SeriesTitle), strings.ToLower(y.Title.SeriesTitle)),
					cmp.Compare(x.Title.Season, y.Title.Season),
					cmp.Compare(x.Title.Number, y.Title.Number),
				)
			})
		}
		for k, i := range slots {
			a.jobs[i] = pending[k]
		}
	})
}

// RetryFailed re-queues every failed or cancelled job.
func (a *App) RetryFailed() {
	a.update(func() {
		for _, j := range a.jobs {
			if j.Status == JobFailed || j.Status == JobCancelled {
				j.Status, j.Error, j.RetryAt, j.Tracks = JobQueued, "", nil, []Track{}
			}
		}
	})
}

// SetJobAccount changes which account a job that isn't running downloads
// with. An empty accountID leaves the choice to the mixer, or to the active
// account when the mixer is off.
func (a *App) SetJobAccount(id, accountID string) {
	a.update(func() {
		_, j := a.findLocked(id)
		if j == nil || j == a.current || (accountID != "" && a.settings.account(accountID) == nil) {
			return
		}
		j.AccountID = accountID
	})
}

// Cancel stops a running job or drops a queued one.
func (a *App) Cancel(id string) {
	a.mu.Lock()
	for _, j := range a.jobs {
		if j.ID != id {
			continue
		}
		if j == a.current {
			a.stopCurrentLocked(stopCancel)
		} else if j.Status == JobQueued || j.Status == JobPaused {
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
		now := time.Now()
		mix := a.settings.mixerOn()
		timed := a.settings.RateLimitTimeout > 0
		var job *Job
		var acc *Account
		var until time.Time
		if !a.paused {
			if mix {
				job, acc, until = a.nextMixedLocked(now)
			} else {
				for _, j := range a.jobs {
					if j.Status == JobQueued {
						job = j
						break
					}
				}
				if job != nil {
					acc = a.settings.account(job.AccountID)
					if acc == nil {
						acc = a.settings.account(a.settings.ActiveAccount)
					}
					// With a time-out set, wait out a timed-out account.
					if st := a.accState[acc.idOrEmpty()]; timed && st != nil && st.readyAt.After(now) {
						job, acc, until = nil, nil, st.readyAt
					}
				}
			}
		}
		if job == nil {
			changed := (a.waitUntil == nil) != until.IsZero() || (a.waitUntil != nil && !a.waitUntil.Equal(until))
			a.waitUntil = nil
			if !until.IsZero() {
				a.waitUntil = &until
			}
			a.mu.Unlock()
			if changed {
				a.emitQueue(false)
			}
			if until.IsZero() {
				<-a.wake
			} else {
				t := time.NewTimer(time.Until(until))
				select {
				case <-a.wake:
				case <-t.C:
				}
				t.Stop()
			}
			continue
		}
		ctx, cancel := context.WithCancel(a.ctx)
		a.current, a.cancel, a.stop, a.waitUntil = job, cancel, stopCancel, nil
		job.Status, job.Error = JobDownloading, ""
		var cookie, accName string
		if acc != nil {
			cookie, accName, a.currentAccount = acc.EtpRt, acc.Name, acc.ID
		}
		multi := len(a.settings.Accounts) > 1
		a.mu.Unlock()
		a.emitQueue(true)

		var err error
		loginFailed := false
		if cookie != "" {
			if err = a.login(cookie); err != nil {
				err, loginFailed = fmt.Errorf("signing in as %s: %w", accName, err), true
			} else if multi {
				a.onLog("Downloading as " + accName)
			}
		}
		if err == nil {
			if mix || timed {
				err = downloader.DownloadTitleOnce(ctx, job.Title)
			} else {
				err = downloader.DownloadTitle(ctx, job.Title)
			}
		}

		a.mu.Lock()
		now = time.Now()
		pause := a.settings.Options.DownloadDelay
		var st *accountState
		if (mix || timed) && acc != nil {
			st = a.stateLocked(acc.ID)
			st.lastUsed = now
		}
		benched := false
		bench := func(d time.Duration, reason string) {
			benched = true
			st.readyAt, st.reason = now.Add(d), reason
			next := "it moves to another account"
			if !mix {
				next = "it waits until " + st.readyAt.Local().Format(time.Kitchen)
			}
			job.Status, job.Error = JobQueued, fmt.Sprintf("%s on %s, so %s.", reason, accName, next)
			go a.onLog(fmt.Sprintf("%s on %s. Resting it until %s.", reason, accName, st.readyAt.Local().Format(time.Kitchen)))
		}
		switch {
		case ctx.Err() != nil:
			switch a.stop {
			case stopPause:
				job.Status = JobPaused
			case stopSkip:
				job.Status, job.Error = JobSkipped, "Skipped"
			case stopQueue:
				job.Status, job.Error = JobQueued, ""
			default:
				job.Status = JobCancelled
			}
		case st != nil && errors.Is(err, downloader.ErrRateLimited):
			st.hits++
			bench(cooldown(st.hits, pause, a.settings.RateLimitTimeout), "Rate-limited")
		case mix && errors.Is(err, downloader.ErrActiveStreams):
			bench(streamsCooldown, "Too many open streams")
		case mix && loginFailed:
			bench(loginCooldown, "Sign-in failed")
			if job.AccountID != "" {
				job.Status, job.Error = JobFailed, err.Error()
			}
		case err != nil:
			job.Status, job.Error = JobFailed, err.Error()
		case job.Status != JobSkipped:
			job.Status = JobDone
		}
		if mix && st != nil && !benched {
			// Finished one way or another; rest before this account's next
			// episode.
			st.hits, st.readyAt, st.reason = 0, now.Add(pause), "Resting"
			if pause <= 0 {
				st.reason = ""
			}
		}
		if st != nil && !mix && !benched {
			st.hits = 0
		}
		if st != nil && a.store != nil {
			if err := a.store.saveAccountState(a.accState); err != nil {
				go a.onLog("Couldn't save the account mixer state: " + err.Error())
			}
		}
		job.Tracks, job.RetryAt = []Track{}, nil
		a.current, a.cancel, a.currentAccount = nil, nil, ""
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
	application.Get().Event.Emit("log", l)
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
				defer a.mu.Unlock()
				a.emitTimer = nil
				a.lastEmit = time.Now()
				application.Get().Event.Emit("queue", a.snapshotLocked())
			})
		}
		return
	}
	a.lastEmit = time.Now()
	// Progress-only updates aren't worth a write; everything else is saved.
	if force && a.store != nil {
		if err := a.store.saveQueue(a.jobs, a.paused); err != nil {
			go a.onLog("Couldn't save the queue: " + err.Error())
		}
	}
	application.Get().Event.Emit("queue", a.snapshotLocked())
}

// PickDirectory opens a folder picker, returning "" when cancelled.
func (a *App) PickDirectory(title, current string) (string, error) {
	return application.Get().Dialog.OpenFile().
		SetTitle(title).
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		SetDirectory(existingDir(current)).
		PromptForSingleSelection()
}

// PickFile opens a file picker, returning "" when cancelled.
func (a *App) PickFile(title, current, pattern string) (string, error) {
	d := application.Get().Dialog.OpenFile().
		SetTitle(title).
		CanChooseFiles(true).
		SetDirectory(existingDir(filepath.Dir(current)))
	if pattern != "" {
		d.AddFilter(pattern, pattern)
	}
	return d.PromptForSingleSelection()
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
func (a *App) OpenURL(url string) error {
	return application.Get().Browser.OpenURL(url)
}

// Search finds series by title.
func (a *App) Search(query string) ([]downloader.SearchResult, error) {
	return downloader.Search(query)
}

// Browse lists catalogue series sorted by "popularity" or "newly_added".
func (a *App) Browse(sortBy string, n int) ([]downloader.SearchResult, error) {
	return downloader.Browse(sortBy, n)
}
