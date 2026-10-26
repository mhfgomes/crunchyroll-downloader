import * as App from "../../wailsjs/go/main/App"
import { EventsOn } from "../../wailsjs/runtime/runtime"
import type { downloader, main } from "../../wailsjs/go/models"

export type Title = downloader.Title
export type Options = downloader.Options
export type LookupResult = downloader.LookupResult
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
  | "done"
  | "skipped"
  | "failed"
  | "cancelled"
export type Job = Omit<main.Job, "status" | "retryAt"> & {
  status: JobStatus
  retryAt?: string
}

export const api = App

export function onQueue(cb: (jobs: Job[]) => void) {
  return EventsOn("queue", cb)
}

export function onLog(cb: (line: LogLine) => void) {
  return EventsOn("log", cb)
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
  const locales = (t.versions ?? []).map((v) => v.audio_locale).filter(Boolean)
  return locales.length ? locales : t.audioLocale ? [t.audioLocale] : []
}
