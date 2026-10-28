# Crunchyroll Downloader

[![Tests](https://img.shields.io/github/actions/workflow/status/mhfgomes/crunchyroll-downloader/tests.yml?branch=master&label=tests)](https://github.com/mhfgomes/crunchyroll-downloader/actions/workflows/tests.yml)
[![Latest release](https://img.shields.io/github/v/release/mhfgomes/crunchyroll-downloader)](https://github.com/mhfgomes/crunchyroll-downloader/releases/latest)
[![Downloads](https://img.shields.io/github/downloads/mhfgomes/crunchyroll-downloader/total)](https://github.com/mhfgomes/crunchyroll-downloader/releases)
[![Go version](https://img.shields.io/github/go-mod/go-version/mhfgomes/crunchyroll-downloader)](go.mod)
[![License](https://img.shields.io/github/license/mhfgomes/crunchyroll-downloader)](LICENSE.txt)

Downloads anime from Crunchyroll and outputs them in a MKV file.

This project is a fork of [CuteTenshii/crunchyroll-downloader](https://github.com/CuteTenshii/crunchyroll-downloader), originally created by [CuteTenshii](https://github.com/CuteTenshii). Credit for the original downloader goes to CuteTenshii and the upstream contributors. This fork adds the desktop app and the changes documented in [the changelog](CHANGELOG.md), while preserving the original [MIT license and copyright notice](LICENSE.txt).

## Features

- Supports choosing the audio and subtitles language, including downloading multiple of each into a single file
- Supports choosing the audio and video quality
- Decrypts Widevine DRM (requires: a `.wvd` file or `client_id.bin` and `private_key.pem` files)
- Adds metadata (like episode name) to the MKV container
- Parallel segment downloads (10 workers) for faster downloads
- Retry with backoff on connection errors
- Uses HTTP/1.1 to avoid HTTP/2 stream resets; interrupted subtitle and manifest downloads are retried up to three attempts
- Single-file media uses bounded 8 MiB ranges and resumes interrupted reads, allowing up to five consecutive attempts
- Batch download from a list of URLs

## Requirements

- [FFmpeg](https://www.ffmpeg.org/download.html#get-packages)
- To download Premium-only content, a Crunchyroll Premium account. No, this can't be bypassed and a free trial should be enough
- Either a `.wvd` file, or a `client_id.bin` and `private_key.pem`

## Download

Check the [latest release](https://github.com/mhfgomes/crunchyroll-downloader/releases/latest) and download the file that corresponds to your OS.

## Usage

- Open a Terminal/Command prompt, and go to the folder where you downloaded the binary/cloned the repo
- Run the program with the options you want:
```shell
Usage of ./crunchyroll-downloader:
  -audio-lang string
        Audio language(s), comma-separated (e.g. "ja-JP,en-US"). Add ALL for every available dub; explicit languages lead, the first available track is default, and ALL alone prefers ja-JP (default "ja-JP")
  -audio-quality string
        Audio quality (default "192k")
  -cc-lang string
        Closed caption language(s), comma-separated (e.g. "en-US,ALL"). Add ALL for every available caption; downloaded in addition to --subs-lang
  -debug-manifest
        Log raw episode playback JSON and manifest XML
  -download-delay duration
        Minimum delay between episode downloads, to help avoid Crunchyroll's rate limiting (e.g. "30s", "2m")
  -etp-rt string
        The "etp_rt" cookie value of your account
  -download-delay-max duration
        With -download-delay, wait a random time between the two before each episode (e.g. "30s" to "2m")
  -file string
        Path to a text file with one URL per line
  -season int
        Season number. Not used if an episode link is entered
  -simultaneous int
        Maximum simultaneous playback sessions for audio and subtitle downloads; keep within your account's available stream slots (default 1)
  -subs-lang string
        Subtitle language(s), comma-separated (e.g. "fr-FR,ALL"). Add ALL for every available subtitle; explicit languages lead, the first available track is default, and ALL alone prefers en-US (default "en-US")
  -temp-dir string
        Folder for track downloads before they are merged (default: the system temp folder)
  -url string
        URL of the episode/season to download
  -video-quality string
        Video quality (default "1080p")
```

Ex: to download the first season of *Hell's Paradise*:
```shell
./crunchyroll-downloader --url https://www.crunchyroll.com/series/GJ0H7Q5ZJ/hells-paradise --season 1 --etp-rt replace_this
```

To download a specific episode:
```shell
./crunchyroll-downloader --url https://www.crunchyroll.com/watch/GE00198973JAJP/dawn-and-confusion --etp-rt replace_this
```

To batch download from a file (one URL per line):
```shell
./crunchyroll-downloader --file list.txt --etp-rt replace_this --subs-lang pt-BR
```

To download multiple audio tracks and subtitles into a single file (the first available requested track of each kind is set as the default):
```shell
./crunchyroll-downloader --url https://www.crunchyroll.com/watch/GE00198973JAJP/dawn-and-confusion --etp-rt replace_this --audio-lang ja-JP,en-US --subs-lang en-US,es-419,de-DE
```

Add `ALL` to any language list to include every available language; listed languages come first. For example, `--subs-lang fr-FR,ALL` makes French the default subtitle and adds the rest:
```shell
./crunchyroll-downloader --url https://www.crunchyroll.com/watch/GE00198973JAJP/dawn-and-confusion --etp-rt replace_this --audio-lang ja-JP,ALL --subs-lang fr-FR,ALL --cc-lang ALL
```

Each dub opens a separate playback session, which counts toward your account's simultaneous stream limit. Downloads use one session at a time by default. Set `--simultaneous` to increase this, leaving room for anyone watching on the same account. For example, when all four Mega Fan stream slots are free:
```shell
./crunchyroll-downloader --url https://www.crunchyroll.com/series/GRDV0019R/jujutsu-kaisen --etp-rt replace_this --audio-lang ALL --subs-lang ALL --simultaneous 4 --download-delay 15s
```

The cap also applies to extra dub sessions needed by `--subs-lang ALL` or `--cc-lang ALL`. Each session is released after its tracks finish, before the worker opens another. Video and audio from the same dub share one session; segment downloads remain parallel. `--download-delay` controls the gap between episodes and does not control simultaneous playback sessions. If you see `TOO_MANY_ACTIVE_STREAMS`, lower `--simultaneous` and close other playback sessions.

If you're getting rate-limited while downloading a season/batch, wait at least this long between each episode:
```shell
./crunchyroll-downloader --url https://www.crunchyroll.com/series/GJ0H7Q5ZJ/hells-paradise --season 1 --etp-rt replace_this --download-delay 30s
```

If Crunchyroll rate-limits an episode anyway, it's retried in place (starting at `-download-delay`, or 1 minute if unset, doubling up to 30 minutes on repeated hits) instead of moving on to the next episode and tripping the same limit again.

## Desktop app

Prefer a window to a terminal? Download `crdl-gui-windows-amd64.exe`, `crdl-gui-linux-amd64.tar.gz`, or `crdl-gui-macos-{arm64,amd64}.zip` from the latest release. Extract the macOS archive and open the `.app`. The Linux archive includes `bin/crdl-gui`, a desktop launcher and its icon; install these under `~/.local` to integrate with your desktop (GTK 4 and WebKitGTK 6 are required). Paste a series or episode link, choose the episodes, and follow each download's progress in the queue. The cookie, languages, qualities, output folder and Widevine CDM location are set once in Settings. FFmpeg is still required. See [gui/README.md](gui/README.md) to build it yourself.

## Releases

Push a version tag to build and publish the CLI and GUI for Windows amd64, Linux amd64, and macOS arm64 and amd64:

```shell
git tag v0.1.0
git push origin v0.1.0
```

The release workflow waits for every build before publishing assets and `SHA256SUMS`. Windows executables include icons and version metadata; macOS GUI archives contain an app bundle with its icon and an ad-hoc signature. Builds are not certificate-signed or notarized.

To build release assets locally for your host OS, install the GUI build prerequisites (including Wails v3.0.0 and Python 3), then run `python scripts/release.py v0.1.0`. Outputs are written to `dist/`. Version metadata is taken from the tag and source metadata is restored after the build.

## Building

### Requirements

- [Go](https://go.dev/dl/)

### Guide

- Clone this repository
- Open a Terminal/Command prompt, and go to the folder where you cloned the repo
- Run `go build .`

## Help

### How do I get my `etp_rt` cookie?

- Go to https://crunchyroll.com
- Open Developer Tools
- Firefox: Go to *Storage* then *Cookies*<br />Chrome: Go to *Application* then *Cookies*
- Select the Crunchyroll domain, then copy the `etp_rt` cookie value

![](.github/screenshots/etp-rt-cookie.png)

### What is a `.wvd` file and do I really need one?

Yes, Crunchyroll uses DRM-only content. This file is used to get a Widevine license, which gives the keys to decrypt the media.

If you don't have a rooted Android device or are just lazy, search "ready to use cdms" and you'll find plenty of websites providing those files.

## License

This project is licensed under the MIT License. See [LICENSE.txt](LICENSE.txt)
