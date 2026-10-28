# Changelog

## 0.1.1

- Fixed audio starting ahead of video and subtitles on some episodes by preserving the source timestamps when merging tracks. This affects both the CLI and desktop app.
- Added an FFmpeg regression test covering audio offsets, AAC priming timestamps, and subtitle timing.

Existing downloads with this issue need to be remuxed or downloaded again with this version.

## 0.1.0

First release of this fork, including the CLI and desktop app.

Based on [CuteTenshii/crunchyroll-downloader](https://github.com/CuteTenshii/crunchyroll-downloader), originally created by [CuteTenshii](https://github.com/CuteTenshii). Thanks to CuteTenshii and the upstream contributors for the original downloader, released under the [MIT license](LICENSE.txt).

- Added a desktop GUI for Windows, Linux and macOS, with series and episode browsing, episode selection, download progress and a persistent queue.
- Added saved accounts, per-episode account selection and an account shuffler with per-account rest periods and cooldowns.
- Moved the CLI downloader into a reusable package shared with the GUI.
- Added a configurable simultaneous playback session limit and randomized delays between episodes.
- Improved download reliability with HTTP/1.1, retries for interrupted subtitles and manifests, and resumable bounded-range media downloads.
- Added automated CLI and GUI release builds for Windows amd64, Linux amd64, and macOS arm64 and amd64, with SHA256 checksums.

FFmpeg and Widevine CDM files are required. Linux GUI builds require GTK 4 and WebKitGTK 6. Binaries are not certificate-signed or notarized.

## Upstream history

The entries below describe releases inherited from [CuteTenshii/crunchyroll-downloader](https://github.com/CuteTenshii/crunchyroll-downloader).

## 1.6.0

- Added `--download-delay` to wait a minimum time between episode downloads, which helps avoid Crunchyroll's rate limiting on season and batch downloads ([#42](https://github.com/CuteTenshii/crunchyroll-downloader/issues/42), [#57](https://github.com/CuteTenshii/crunchyroll-downloader/issues/57))
- A rate-limited episode is now retried in place, waiting `--download-delay` (or 1 minute if unset) and doubling up to 30 minutes on repeated hits, instead of moving on and hitting the same limit on the next episode
- `ALL` can now be combined with specific languages in `--audio-lang`, `--subs-lang` and `--cc-lang` (e.g. `--subs-lang fr-FR,ALL`). The listed languages come first and the first one is the default track. `ALL` on its own prefers `ja-JP` audio and `en-US` subtitles
- Subtitles and closed captions are now collected from every downloaded audio version, so captions that only exist on a dub (like the English dub's English captions) are no longer reported as unavailable
- FFmpeg is now found when it's in the same folder as the downloader, which previously failed on Windows with "cannot run executable found relative to current directory". A missing FFmpeg is reported at startup instead of after every track has downloaded ([#61](https://github.com/CuteTenshii/crunchyroll-downloader/issues/61))
- An invalid or expired `--etp-rt` cookie now exits with a clear message instead of printing "Access token expired. Refetching one..." forever
- Fixed a crash when removing a player stream failed during cleanup

## 1.5.1

- Audio dubs, subtitles, closed captions and the video track now download concurrently instead of one after another, making multi-language downloads much faster
- Added progress bars showing percentage, segment count and download speed (Mbps) for every active download
- Playback errors now surface the API's error reason, and rate-limited (429) responses print a hint to wait or use a different account ([#47](https://github.com/CuteTenshii/crunchyroll-downloader/issues/47))
- Fixed a bug where retrying a request after refreshing an expired access token sent an empty or truncated body, breaking license requests

## 1.4.0

- Added support for on-demand (single-file) manifests and normalized non-Widevine PSSH system IDs, fixing "PSSH not found" and PlayReady errors on some episodes ([#8](https://github.com/CuteTenshii/crunchyroll-downloader/issues/8), [#38](https://github.com/CuteTenshii/crunchyroll-downloader/issues/38), [#39](https://github.com/CuteTenshii/crunchyroll-downloader/issues/39))
- Fixed a crash when the playback response "error" field is a boolean or number instead of a string ([#29](https://github.com/CuteTenshii/crunchyroll-downloader/issues/29))
- Fixed audio language selection so dubs resolve from the versions list instead of silently downloading the original language ([#35](https://github.com/CuteTenshii/crunchyroll-downloader/issues/35))
- Unavailable subtitle and caption locales are now skipped instead of aborting the whole episode ([#27](https://github.com/CuteTenshii/crunchyroll-downloader/issues/27))
- Unavailable audio locales are now skipped, downloading whichever of the requested languages are available ([#28](https://github.com/CuteTenshii/crunchyroll-downloader/issues/28))
- Fixed URL parsing for locale-prefixed links such as /fr/series/...

## 1.3.0

- Large downloads no longer buffer the entire file in memory. Segments are written to disk as they arrive, so memory use no longer scales with the length of the title — movie-length media previously crashed with an out-of-memory error
- The decryption key is now selected by matching each track's key ID, instead of always using the first key in the Widevine license, which could decrypt to garbage
- Added `--cc-lang` to download closed captions (dubtitles) in addition to `--subs-lang`
- `--audio-lang all` and `--subs-lang all` download every available track
- Subtitles no longer depend on which audio track came first
- Added CI: tests, cross-platform builds, vet and gofmt on every push and pull request

## 1.2.0

- Added support for downloading multiple audio tracks and multiple subtitles into a single file (`--audio-lang ja-JP,en-US`, `--subs-lang en-US,es-419`). The first of each is marked as the default track, and each track is tagged with its language so media servers can select them.
- An episode is skipped if any requested audio or subtitle language is unavailable for it.
- Parallel segment downloads (10 workers) for much faster downloads
- Retry with backoff on connection errors instead of crashing
- Added `--urls` flag to batch download from a text file with one URL per line
- Invalid URLs in batch mode are skipped instead of stopping the whole process

## 1.1.1

- Optimized code, tried to handle errors
- Some random fixes
- Added a way to automatically refetch an access token if the current one expires

## 1.1.0

- Added support for downloading entire seasons
- Fixed MPD parsing
- Temporary downloaded files (video, audio segments and subtitles) are now stored in the OS temporary files then deleted
- Fixed FFmpeg merge command
- Docs improvements
- Support for `device_id.bin` and `private_key.pem` files

## 1.0.0

Initial release
