package main

import (
	"testing"
	"time"

	"crunchyroll-downloader/downloader"
)

func TestStoreRoundTrip(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	st, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	s := st.loadSettings(Settings{EtpRt: "default"})
	if s.EtpRt != "default" {
		t.Fatalf("defaults not used: %+v", s)
	}
	s.EtpRt = "saved"
	if err := st.saveSettings(s); err != nil {
		t.Fatal(err)
	}
	if got := st.loadSettings(Settings{}); got.EtpRt != "saved" {
		t.Fatalf("settings not saved: %+v", got)
	}

	jobs := []*Job{
		{ID: "7", Title: downloader.Title{ID: "b", Title: "Two"}, Status: JobDownloading},
		{ID: "3", Title: downloader.Title{ID: "a", Title: "One"}, AccountID: "2", Status: JobFailed, Error: "boom"},
		{ID: "9", Title: downloader.Title{ID: "c"}, Status: JobPaused},
	}
	if err := st.saveQueue(jobs, true); err != nil {
		t.Fatal(err)
	}
	got, paused, err := st.loadQueue()
	if err != nil {
		t.Fatal(err)
	}
	if !paused || len(got) != 3 {
		t.Fatalf("paused=%v jobs=%d", paused, len(got))
	}
	if got[0].ID != "7" || got[0].Status != JobQueued || got[0].Title.Title != "Two" {
		t.Errorf("running job not requeued in place: %+v", got[0])
	}
	if got[1].AccountID != "2" {
		t.Errorf("account not kept: %+v", got[1])
	}
	if got[1].Status != JobFailed || got[1].Error != "boom" || got[2].Status != JobPaused {
		t.Errorf("statuses not kept: %+v %+v", got[1], got[2])
	}
}

func TestAccountStateRoundTrip(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	st, err := openStore()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if got, err := st.loadAccountState(); err != nil || len(got) != 0 {
		t.Fatalf("fresh database: %v %v", got, err)
	}
	ready := time.Now().Add(10 * time.Minute).Truncate(time.Second)
	used := ready.Add(-time.Hour)
	in := map[string]*accountState{"a": {readyAt: ready, lastUsed: used, hits: 2, reason: "Rate-limited"}}
	if err := st.saveAccountState(in); err != nil {
		t.Fatal(err)
	}
	got, err := st.loadAccountState()
	if err != nil {
		t.Fatal(err)
	}
	a := got["a"]
	if a == nil || !a.readyAt.Equal(ready) || !a.lastUsed.Equal(used) || a.hits != 2 || a.reason != "Rate-limited" {
		t.Fatalf("state not kept: %+v", a)
	}
}

func TestSettingsNormalize(t *testing.T) {
	s := Settings{EtpRt: "legacy"}
	s.normalize()
	if s.EtpRt != "" || len(s.Accounts) != 1 || s.Accounts[0].EtpRt != "legacy" || s.ActiveAccount != s.Accounts[0].ID {
		t.Fatalf("legacy cookie not migrated: %+v", s)
	}

	s = Settings{Accounts: []Account{{ID: "a", EtpRt: "x"}, {ID: "b", EtpRt: "y"}}, ActiveAccount: "gone"}
	s.normalize()
	if s.ActiveAccount != "a" || s.activeCookie() != "x" {
		t.Fatalf("missing active account not replaced: %+v", s)
	}

	s = Settings{}
	s.normalize()
	if s.Accounts == nil || s.ActiveAccount != "" || s.activeCookie() != "" {
		t.Fatalf("empty settings: %+v", s)
	}
}
