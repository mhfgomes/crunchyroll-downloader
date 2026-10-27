import { Events } from "@wailsio/runtime"

import { App } from "../../bindings/crunchyroll-downloader-gui"
import type * as main from "../../bindings/crunchyroll-downloader-gui/models"
import type * as downloader from "../../bindings/crunchyroll-downloader/downloader/models"

export type Title = downloader.Title
export type Options = downloader.Options
export type LookupResult = downloader.LookupResult
export type SearchResult = downloader.SearchResult
export type SeasonSummary = downloader.SeasonSummary
export type Settings = main.Settings
export type Environment = main.Environment
export type Language = main.Language
export type LogLine = main.LogLine
export type Track = main.Track
export type JobStatus =
  | "queued"
  | "downloading"
  | "merging"
  | "waiting"
  | "paused"
  | "done"
  | "skipped"
  | "failed"
  | "cancelled"
export type Job = Omit<main.Job, "status" | "retryAt"> & {
  status: JobStatus
  retryAt?: string
}

export const api = App

export type QueueState = { paused: boolean; jobs: Job[] }

export function onQueue(cb: (q: QueueState) => void) {
  return Events.On("queue", (ev) => cb(ev.data as QueueState))
}

export function onLog(cb: (line: LogLine) => void) {
  return Events.On("log", (ev) => cb(ev.data as LogLine))
}

// Go's time.Duration crosses the bridge as nanoseconds.
export const NS_PER_SECOND = 1_000_000_000

export function errorMessage(err: unknown): string {
  if (typeof err === "string") return err
  if (err instanceof Error) return err.message
  return String(err)
}

export function episodeCode(t: Pick<Title, "season" | "number">) {
  const pad = (n: number) => String(n).padStart(2, "0")
  return `S${pad(t.season)}E${pad(t.number)}`
}

export function audioLocales(t: Title): string[] {
  const locales = (t.versions ?? []).map((v) => v?.audio_locale ?? "").filter(Boolean)
  return locales.length ? locales : t.audioLocale ? [t.audioLocale] : []
}
