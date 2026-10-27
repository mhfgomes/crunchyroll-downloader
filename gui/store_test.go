package main

import (
	"testing"

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
		{ID: "3", Title: downloader.Title{ID: "a", Title: "One"}, Status: JobFailed, Error: "boom"},
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
	if got[1].Status != JobFailed || got[1].Error != "boom" || got[2].Status != JobPaused {
		t.Errorf("statuses not kept: %+v %+v", got[1], got[2])
	}
}
