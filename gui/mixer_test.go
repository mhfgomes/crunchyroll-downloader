package main

import (
	"testing"
	"time"
)

func TestPickAccount(t *testing.T) {
	now := time.Now()
	accounts := []Account{{ID: "a", EtpRt: "x"}, {ID: "b", EtpRt: "y"}, {ID: "c"}}
	state := map[string]*accountState{
		"a": {lastUsed: now.Add(-time.Minute)},
		"b": {lastUsed: now.Add(-time.Hour)},
	}
	if acc, _ := pickAccount(accounts, state, now); acc == nil || acc.ID != "b" {
		t.Fatalf("want the longest-rested account b, got %+v", acc)
	}

	state["b"].readyAt = now.Add(10 * time.Minute)
	if acc, _ := pickAccount(accounts, state, now); acc == nil || acc.ID != "a" {
		t.Fatalf("want a while b is benched, got %+v", acc)
	}

	state["a"].readyAt = now.Add(5 * time.Minute)
	acc, next := pickAccount(accounts, state, now)
	if acc != nil || !next.Equal(state["a"].readyAt) {
		t.Fatalf("want no account until a is ready, got %+v %v", acc, next)
	}

	// Accounts without a cookie are never picked, even untouched ones.
	if acc, next := pickAccount(accounts[2:], state, now); acc != nil || !next.IsZero() {
		t.Fatalf("cookieless account picked: %+v %v", acc, next)
	}
}

func TestCooldown(t *testing.T) {
	cases := []struct {
		hits    int
		pause   time.Duration
		timeout time.Duration
		want    time.Duration
	}{
		{1, 0, 0, time.Minute},
		{2, 0, 0, 2 * time.Minute},
		{3, 0, 0, 4 * time.Minute},
		{1, 5 * time.Minute, 0, 5 * time.Minute},
		{20, 0, 0, maxCooldown},
		{1, 0, 15 * time.Minute, 15 * time.Minute},
		{5, 5 * time.Minute, 2 * time.Hour, 2 * time.Hour},
	}
	for _, c := range cases {
		if got := cooldown(c.hits, c.pause, c.timeout); got != c.want {
			t.Errorf("cooldown(%d, %s, %s) = %s, want %s", c.hits, c.pause, c.timeout, got, c.want)
		}
	}
}

func TestMixerOn(t *testing.T) {
	two := []Account{{ID: "a", EtpRt: "x"}, {ID: "b", EtpRt: "y"}}
	if !(Settings{Mixer: true, Accounts: two}).mixerOn() {
		t.Error("mixer off with two accounts")
	}
	if (Settings{Mixer: false, Accounts: two}).mixerOn() {
		t.Error("mixer on while disabled")
	}
	if (Settings{Mixer: true, Accounts: []Account{{ID: "a", EtpRt: "x"}, {ID: "b"}}}).mixerOn() {
		t.Error("mixer on with only one usable account")
	}
	if (Settings{Mixer: true, Accounts: []Account{{ID: "a", EtpRt: "x"}, {ID: "b", EtpRt: "y", NoMixer: true}}}).mixerOn() {
		t.Error("mixer on with only one account in it")
	}
}

func TestNextMixedRespectsPinnedAccount(t *testing.T) {
	now := time.Now()
	a := NewApp()
	a.settings = Settings{Mixer: true, Accounts: []Account{{ID: "a", EtpRt: "x"}, {ID: "b", EtpRt: "y"}}}
	a.accState["a"] = &accountState{readyAt: now.Add(time.Minute)}
	a.jobs = []*Job{
		{ID: "1", AccountID: "a", Status: JobQueued},
		{ID: "2", Status: JobQueued},
	}
	job, acc, _ := a.nextMixedLocked(now)
	if job == nil || job.ID != "2" || acc.ID != "b" {
		t.Fatalf("want job 2 on b while a rests, got %+v %+v", job, acc)
	}

	// An account left out of the mixer only takes jobs pinned to it.
	a.settings.Accounts = append(a.settings.Accounts, Account{ID: "c", EtpRt: "z", NoMixer: true})
	a.accState["b"] = &accountState{readyAt: now.Add(time.Minute)}
	if job, acc, _ := a.nextMixedLocked(now); job != nil {
		t.Fatalf("unpinned job given to an account outside the mixer: %+v %+v", job, acc)
	}
	a.jobs = append(a.jobs, &Job{ID: "3", AccountID: "c", Status: JobQueued})
	if job, acc, _ := a.nextMixedLocked(now); job == nil || job.ID != "3" || acc.ID != "c" {
		t.Fatalf("want pinned job 3 on c, got %+v %+v", job, acc)
	}
	delete(a.accState, "b")
	a.settings.Accounts = a.settings.Accounts[:2]
	a.jobs = a.jobs[:1]
	if job, _, wait := a.nextMixedLocked(now); job != nil || !wait.Equal(now.Add(time.Minute)) {
		t.Fatalf("pinned job should wait for a, got %+v %v", job, wait)
	}
}
