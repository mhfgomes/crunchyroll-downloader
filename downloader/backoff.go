package downloader

import (
	"math/rand/v2"
	"sync"
	"time"
)

// downloadBackoff enforces a minimum gap between the end of one episode
// download and the start of the next, to help avoid tripping Crunchyroll's
// rate limiting on accounts that download many episodes in a row. now and
// sleep are overridable so tests don't have to wait on the real clock.
type downloadBackoff struct {
	delay time.Duration
	// maxDelay, when above delay, makes each gap a random length between
	// the two, so downloads don't start on a regular beat.
	maxDelay time.Duration
	gap      time.Duration // the gap the next wait enforces
	now      func() time.Time
	sleep    func(time.Duration)

	mu            sync.Mutex
	started       bool
	last          time.Time
	rateLimitHits int
}

// defaultRateLimitWait is the starting retry delay for a rate-limited episode
// when no -download-delay is configured.
const defaultRateLimitWait = time.Minute

// maxRateLimitWait caps rateLimitWait's exponential growth so a long streak of
// consecutive rate-limit hits doesn't produce absurd delays.
const maxRateLimitWait = 30 * time.Minute

func newDownloadBackoff(delay, maxDelay time.Duration) *downloadBackoff {
	return &downloadBackoff{delay: delay, maxDelay: maxDelay, gap: delay, now: time.Now, sleep: sleepCtx}
}

// RandomBetween returns a random duration in [lo, hi], or lo when hi isn't
// above it.
func RandomBetween(lo, hi time.Duration) time.Duration {
	if hi <= lo {
		return lo
	}
	return lo + rand.N(hi-lo+1)
}

// wait blocks until delay has elapsed since the matching done call for the
// previous download. It is a no-op before the first download, when b is nil,
// or when no delay is configured.
func (b *downloadBackoff) wait() {
	if b == nil || b.delay <= 0 {
		return
	}

	b.mu.Lock()
	if !b.started {
		b.mu.Unlock()
		return
	}
	remaining := b.gap - b.now().Sub(b.last)
	b.mu.Unlock()

	if remaining > 0 {
		logf("Waiting %s before the next download (rate-limit backoff)...\n", remaining.Round(time.Second))
		b.sleep(remaining)
	}
}

// done marks a download as finished, starting the clock that the next wait
// call measures against.
func (b *downloadBackoff) done() {
	if b == nil || b.delay <= 0 {
		return
	}
	b.mu.Lock()
	b.last = b.now()
	b.started = true
	b.gap = RandomBetween(b.delay, b.maxDelay)
	b.mu.Unlock()
}

// rateLimitWait returns how long to wait before retrying an episode that was
// just rate-limited by Crunchyroll, doubling with each consecutive hit (since
// the last resetRateLimit) up to maxRateLimitWait. Safe to call on a nil
// backoff, so it works whether or not -download-delay is configured.
func (b *downloadBackoff) rateLimitWait() time.Duration {
	base := defaultRateLimitWait
	if b != nil && b.delay > base {
		base = b.delay
	}

	var hits int
	if b != nil {
		b.mu.Lock()
		hits = b.rateLimitHits
		b.rateLimitHits++
		b.mu.Unlock()
	}

	wait := base
	for i := 0; i < hits && wait < maxRateLimitWait; i++ {
		wait *= 2
	}
	if wait > maxRateLimitWait {
		wait = maxRateLimitWait
	}
	return wait
}

// resetRateLimit clears the consecutive-hit counter after a successful
// download. Safe to call on a nil backoff.
func (b *downloadBackoff) resetRateLimit() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.rateLimitHits = 0
	b.mu.Unlock()
}
