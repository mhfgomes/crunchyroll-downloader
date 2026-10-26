import { useEffect, useState } from "react"
import { FolderOpenIcon, RotateCwIcon, XIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { useBackend } from "@/hooks/use-backend"
import { api, episodeCode, type Job, type JobStatus, type Track } from "@/lib/api"
import { cn } from "@/lib/utils"

const STATUS: Record<JobStatus, { label: string; className: string }> = {
  queued: { label: "Queued", className: "text-muted-foreground" },
  downloading: { label: "Downloading", className: "text-primary" },
  merging: { label: "Merging tracks", className: "text-primary" },
  waiting: { label: "Rate-limited", className: "text-amber-300" },
  done: { label: "Saved", className: "text-emerald-400" },
  skipped: { label: "Already saved", className: "text-muted-foreground" },
  failed: { label: "Failed", className: "text-destructive" },
  cancelled: { label: "Cancelled", className: "text-muted-foreground" },
}

export function QueueView({ onBrowse }: { onBrowse: () => void }) {
  const { jobs } = useBackend()
  const finished = jobs.filter((j) => ["done", "skipped", "failed", "cancelled"].includes(j.status))

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-end justify-between border-b px-8 pt-7 pb-5">
        <div>
          <h1 className="font-heading text-3xl leading-none font-bold tracking-tight">Queue</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Episodes download one at a time, in the order you added them.
          </p>
        </div>
        {finished.length > 0 && (
          <Button variant="outline" size="sm" onClick={() => void api.ClearFinished()}>
            Clear finished
          </Button>
        )}
      </header>
      <ScrollArea className="min-h-0 flex-1">
        {jobs.length === 0 ? (
          <div className="max-w-md px-8 pt-16">
            <p className="font-heading text-2xl font-semibold">Nothing queued yet.</p>
            <p className="mt-2 text-sm text-muted-foreground">
              Look up a series or episode and choose what to download.
            </p>
            <Button className="mt-5" onClick={onBrowse}>
              Browse episodes
            </Button>
          </div>
        ) : (
          <ol className="divide-y px-8">
            {jobs.map((job) => (
              <JobRow key={job.id} job={job} />
            ))}
          </ol>
        )}
      </ScrollArea>
    </div>
  )
}

function JobRow({ job }: { job: Job }) {
  const t = job.title
  const status = STATUS[job.status]
  const running = job.status === "downloading" || job.status === "merging" || job.status === "waiting"

  return (
    <li className="flex gap-5 py-4">
      <div className="relative aspect-video w-36 shrink-0 overflow-hidden rounded-md bg-muted">
        {t.thumbnail && <img src={t.thumbnail} alt="" className="size-full object-cover" />}
        <span className="absolute bottom-0.5 left-2 font-heading text-2xl leading-none font-extrabold text-foreground drop-shadow-[0_1px_6px_rgb(0_0_0/0.7)] tabular-nums">
          {t.number}
        </span>
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-start gap-3">
          <div className="min-w-0 flex-1">
            <div className="truncate font-medium">{t.title}</div>
            <div className="mt-0.5 truncate text-sm text-muted-foreground">
              {t.seriesTitle}, {episodeCode(t)}
            </div>
          </div>
          <Actions job={job} />
        </div>

        <div className={cn("mt-2 text-sm", status.className)}>
          {job.status === "waiting" && job.retryAt ? <RetryCountdown at={job.retryAt} /> : status.label}
        </div>

        {job.error && job.status !== "waiting" && (
          <p className="mt-1 line-clamp-3 text-xs text-destructive/90 selectable">{job.error}</p>
        )}

        {running && (job.tracks?.length ?? 0) > 0 && (
          <div className="mt-3 flex flex-col gap-2.5">
            {(job.tracks ?? []).map((tr) => (
              <TrackBar key={tr.id} track={tr} />
            ))}
          </div>
        )}
        {job.status === "merging" && <div className="mt-3 h-1 w-full animate-pulse rounded-full bg-primary/40" />}
      </div>
    </li>
  )
}

function TrackBar({ track }: { track: Track }) {
  const pct = track.total > 0 ? Math.min(100, (track.current / track.total) * 100) : null
  const name = track.title.replace(/^Downloading /, "")
  return (
    <div>
      <div className="mb-1 flex justify-between text-xs text-muted-foreground">
        <span className="first-letter:uppercase">{name}</span>
        <span className="tabular-nums">
          {track.rateMbps > 0 && `${track.rateMbps.toFixed(1)} Mbps  `}
          {pct !== null ? `${Math.floor(pct)}%` : formatBytes(track.current)}
        </span>
      </div>
      <div className="h-1 overflow-hidden rounded-full bg-muted">
        <div
          className={cn("h-full rounded-full bg-primary transition-[width] duration-200", pct === null && "w-1/3 animate-pulse")}
          style={pct !== null ? { width: `${pct}%` } : undefined}
        />
      </div>
    </div>
  )
}

function Actions({ job }: { job: Job }) {
  const active = ["queued", "downloading", "merging", "waiting"].includes(job.status)
  return (
    <div className="flex shrink-0 gap-1">
      {job.output && (job.status === "done" || job.status === "skipped") && (
        <IconAction label="Show in folder" onClick={() => void api.Reveal(job.output!)}>
          <FolderOpenIcon />
        </IconAction>
      )}
      {(job.status === "failed" || job.status === "cancelled") && (
        <IconAction label="Try again" onClick={() => void api.Retry(job.id)}>
          <RotateCwIcon />
        </IconAction>
      )}
      <IconAction
        label={active ? "Cancel" : "Remove from list"}
        onClick={() => void (active ? api.Cancel(job.id) : api.Remove(job.id))}
      >
        <XIcon />
      </IconAction>
    </div>
  )
}

function IconAction({ label, onClick, children }: { label: string; onClick: () => void; children: React.ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger render={<Button variant="ghost" size="icon-sm" aria-label={label} onClick={onClick} />}>
        {children}
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}

function RetryCountdown({ at }: { at: string }) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [])
  const secs = Math.max(0, Math.round((new Date(at).getTime() - now) / 1000))
  const m = Math.floor(secs / 60)
  const s = secs % 60
  return <>Rate-limited by Crunchyroll, retrying in {m > 0 ? `${m}m ${s}s` : `${s}s`}</>
}

function formatBytes(n: number) {
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`
  if (n < 1024 ** 3) return `${(n / 1024 ** 2).toFixed(1)} MB`
  return `${(n / 1024 ** 3).toFixed(2)} GB`
}
