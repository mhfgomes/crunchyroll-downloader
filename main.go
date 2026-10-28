package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"crunchyroll-downloader/downloader"
)

func main() {
	defaults := downloader.DefaultOptions()
	audioLang := flag.String("audio-lang", defaults.AudioLangs, "Audio language(s), comma-separated (e.g. \"ja-JP,en-US\"). Add ALL for every available dub; explicit languages lead, the first available track is default, and ALL alone prefers ja-JP")
	subtitlesLang := flag.String("subs-lang", defaults.SubsLangs, "Subtitle language(s), comma-separated (e.g. \"fr-FR,ALL\"). Add ALL for every available subtitle; explicit languages lead, the first available track is default, and ALL alone prefers en-US")
	ccLang := flag.String("cc-lang", "", "Closed caption language(s), comma-separated (e.g. \"en-US,ALL\"). Add ALL for every available caption; downloaded in addition to --subs-lang")
	videoQuality := flag.String("video-quality", defaults.VideoQuality, "Video quality")
	audioQuality := flag.String("audio-quality", defaults.AudioQuality, "Audio quality")
	seasonNumber := flag.Int("season", 0, "Season number. Not used if an episode link is entered")
	etpRt := flag.String("etp-rt", "", "The \"etp_rt\" cookie value of your account")
	debug := flag.Bool("debug-manifest", false, "Log raw episode playback JSON and manifest XML")
	downloadDelay := flag.Duration("download-delay", 0, "Minimum delay between episode downloads, to help avoid Crunchyroll's rate limiting (e.g. \"30s\", \"2m\")")
	downloadDelayMax := flag.Duration("download-delay-max", 0, "With -download-delay, wait a random time between the two before each episode (e.g. \"30s\" to \"2m\")")
	simultaneous := flag.Int("simultaneous", defaults.Simultaneous, "Maximum simultaneous playback sessions for audio and subtitle downloads; keep within your account's available stream slots")
	output := flag.String("output-dir", defaults.OutputDir, "Target base path for downloading")
	tempDir := flag.String("temp-dir", "", "Folder for track downloads before they are merged (default: the system temp folder)")
	url := flag.String("url", "", "URL of the episode/season to download")
	urlsFile := flag.String("file", "", "Path to a text file with one URL per line")
	flag.Parse()

	err := downloader.Configure(downloader.Options{
		AudioLangs:       *audioLang,
		SubsLangs:        *subtitlesLang,
		CCLangs:          *ccLang,
		VideoQuality:     *videoQuality,
		AudioQuality:     *audioQuality,
		OutputDir:        *output,
		TempDir:          *tempDir,
		Simultaneous:     *simultaneous,
		DownloadDelay:    *downloadDelay,
		DownloadDelayMax: *downloadDelayMax,
		DebugManifest:    *debug,
	})
	if err != nil {
		fmt.Printf("-%s\n", err)
		os.Exit(1)
	}

	if *url == "" && *urlsFile == "" {
		flag.Usage()
		os.Exit(1)
	}

	if *etpRt == "" {
		fmt.Println("You must specify the \"-etp-rt\" option!\n- Open Crunchyroll on your browser and log in.\n- Open developer tools (Ctrl+Shift+I), go to \"Application\", and then \"Cookies\".\n- The value of the \"ept_rt\" cookie is what you need to input into this option.")
		os.Exit(1)
	}

	// Checked up front so a missing ffmpeg fails before anything is downloaded,
	// not at merge time after every track has been fetched.
	if _, err := downloader.FindFFmpeg(); err != nil {
		fmt.Println("FFmpeg wasn't found. Install it and add it to your PATH, or put the ffmpeg executable in the same folder as this program.\nDownload it from https://www.ffmpeg.org/download.html")
		os.Exit(1)
	}

	err = downloader.Login(*etpRt)
	if errors.Is(err, downloader.ErrAuthRejected) {
		fmt.Printf("Couldn't log in: %s\nYour \"-etp-rt\" cookie is wrong or expired. Copy a fresh one from your browser (see the README).\n", err)
		os.Exit(1)
	}
	if err != nil {
		fmt.Printf("Couldn't log in: %s\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	process := func(u string) {
		if err := downloader.ProcessURL(ctx, u, *seasonNumber); err != nil {
			fmt.Println(err)
		}
	}

	if *urlsFile != "" {
		file, err := os.Open(*urlsFile)
		if err != nil {
			fmt.Printf("Failed to open URLs file: %s\n", err)
			os.Exit(1)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		var urls []string
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && strings.HasPrefix(line, "http") {
				urls = append(urls, line)
			}
		}

		fmt.Printf("Found %d URLs to download\n\n", len(urls))
		for i, u := range urls {
			fmt.Printf("=== [%d/%d] %s ===\n", i+1, len(urls), u)
			process(u)
			fmt.Println()
		}
	} else {
		process(*url)
	}
}
