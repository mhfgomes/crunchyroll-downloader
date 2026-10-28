package main

import (
	"time"
)

// The mixer spreads queued episodes over every saved account so no single
// account downloads often enough to be rate-limited. Each account rests for
// the "pause between episodes" after every episode, and an account that is
// rate-limited anyway is benched for a growing cooldown while the episode
// moves straight on to the next rested account.

const (
	// minCooldown is the first bench for a rate-limited account; it doubles
	// with each hit in a row, up to maxCooldown.
	minCooldown = time.Minute
	maxCooldown = 30 * time.Minute
	// streamsCooldown benches an account that has too many open streams,
	// usually because it is being watched elsewhere.
	streamsCooldown = 2 * time.Minute
	// loginCooldown benches an account whose cookie was refused.
	loginCooldown = maxCooldown
)

// accountState is what the mixer tracks for each account. It is kept in
// memory only, since rate limits don't outlive a restart by much.
type accountState struct {
	readyAt  time.Time // the account is resting until then
	lastUsed time.Time
	hits     int // rate limits in a row
	reason   string
}

// AccountStatus is an account's mixer state as the frontend sees it.
type AccountStatus struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Busy    bool       `json:"busy"`
	InMixer bool       `json:"inMixer"`
	ReadyAt *time.Time `json:"readyAt,omitempty"`
	Reason  string     `json:"reason,omitempty"`
}

// mixed reports whether the mixer may hand episodes to acc.
func (acc Account) mixed() bool { return acc.EtpRt != "" && !acc.NoMixer }

// mixerAccounts lists the accounts the mixer chooses from.
func (s Settings) mixerAccounts() []Account {
	var out []Account
	for _, acc := range s.Accounts {
		if acc.mixed() {
			out = append(out, acc)
		}
	}
	return out
}

// mixerOn reports whether s spreads downloads over its accounts, which takes
// at least two of them in the mixer.
func (s Settings) mixerOn() bool {
	return s.Mixer && len(s.mixerAccounts()) > 1
}

// pickAccount chooses the account to download with from candidates: the
// rested one that has waited longest since its last episode. When none is
// rested it returns nil and the earliest time one will be.
func pickAccount(candidates []Account, state map[string]*accountState, now time.Time) (*Account, time.Time) {
	var best *Account
	var bestUsed, next time.Time
	for i := range candidates {
		acc := &candidates[i]
		if acc.EtpRt == "" {
			continue
		}
		st := state[acc.ID]
		if st == nil {
			st = &accountState{}
		}
		if st.readyAt.After(now) {
			if next.IsZero() || st.readyAt.Before(next) {
				next = st.readyAt
			}
			continue
		}
		if best == nil || st.lastUsed.Before(bestUsed) {
			best, bestUsed = acc, st.lastUsed
		}
	}
	if best != nil {
		return best, time.Time{}
	}
	return nil, next
}

// cooldown is how long to bench an account after its hits-th rate limit in a
// row: the configured timeout when there is one, otherwise starting from the
// larger of minCooldown and the pause and doubling up to maxCooldown.
func cooldown(hits int, pause, timeout time.Duration) time.Duration {
	if timeout > 0 {
		return timeout
	}
	wait := max(minCooldown, pause)
	for i := 1; i < hits && wait < maxCooldown; i++ {
		wait *= 2
	}
	return min(wait, maxCooldown)
}

// idOrEmpty is the account's ID, or "" for no account.
func (acc *Account) idOrEmpty() string {
	if acc == nil {
		return ""
	}
	return acc.ID
}

// stateLocked returns the mixer state for an account, creating it.
func (a *App) stateLocked(id string) *accountState {
	st := a.accState[id]
	if st == nil {
		st = &accountState{}
		a.accState[id] = st
	}
	return st
}

// nextMixedLocked picks the first queued job that has a rested account. When
// every candidate account is resting it returns a nil job and when the next
// one is free.
func (a *App) nextMixedLocked(now time.Time) (*Job, *Account, time.Time) {
	var wait time.Time
	for _, j := range a.jobs {
		if j.Status != JobQueued {
			continue
		}
		candidates := a.settings.mixerAccounts()
		if acc := a.settings.account(j.AccountID); acc != nil {
			candidates = []Account{*acc}
		}
		acc, next := pickAccount(candidates, a.accState, now)
		if acc != nil {
			return j, acc, time.Time{}
		}
		if !next.IsZero() && (wait.IsZero() || next.Before(wait)) {
			wait = next
		}
	}
	return nil, nil, wait
}

// accountStatusLocked lists each account's mixer state.
func (a *App) accountStatusLocked(now time.Time) []AccountStatus {
	out := []AccountStatus{}
	for _, acc := range a.settings.Accounts {
		s := AccountStatus{ID: acc.ID, Name: acc.Name, Busy: a.currentAccount == acc.ID, InMixer: acc.mixed()}
		if st := a.accState[acc.ID]; st != nil && st.readyAt.After(now) {
			t := st.readyAt
			s.ReadyAt, s.Reason = &t, st.reason
		}
		out = append(out, s)
	}
	return out
}
