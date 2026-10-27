import { useEffect, useState } from "react"
import {
  ArrowDownIcon,
  ArrowUpDownIcon,
  ArrowDownToLineIcon,
  ArrowUpIcon,
  ArrowUpToLineIcon,
  ExternalLinkIcon,
  FolderOpenIcon,
  GripVerticalIcon,
  PauseIcon,
  PlayIcon,
  RotateCwIcon,
  SkipForwardIcon,
  XIcon,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/context-menu"
import { ScrollArea } from "@/components/ui/scroll-area"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
} from "@/components/ui/select"
import { episodeLink } from "@/components/title-cards"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { useBackend } from "@/hooks/use-backend"
import {
  api,
  episodeCode,
  type Job,
  type JobStatus,
  type Track,
} from "@/lib/api"
import { cn } from "@/lib/utils"

const STATUS: Record<JobStatus, { label: string; className: string }> = {
  queued: { label: "Queued", className: "text-muted-foreground" },
  downloading: { label: "Downloading", className: "text-primary" },
  merging: { label: "Merging tracks", className: "text-primary" },
  waiting: { label: "Rate-limited", className: "text-amber-300" },
  paused: { label: "Paused", className: "text-select" },
  done: { label: "Saved", className: "text-emerald-400" },
  skipped: { label: "Already saved", className: "text-muted-foreground" },
  failed: { label: "Failed", className: "text-destructive" },
  cancelled: { label: "Cancelled", className: "text-muted-foreground" },
}

export function QueueView({ onBrowse }: { onBrowse: () => void }) {
  const { jobs, paused } = useBackend()
  const [drag, setDrag] = useState<{ id: string; over: number } | null>(null)
  const sortable =
    jobs.filter((j) => j.status === "queued" || j.status === "paused").length >
    1
  const finished = jobs.filter((j) =>
    ["done", "skipped", "failed", "cancelled"].includes(j.status)
  )
  const retryable = jobs.filter(
    (j) => j.status === "failed" || j.status === "cancelled"
  ).length
  const pending = jobs.some((j) => ACTIVE.includes(j.status))

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-end justify-between border-b px-8 pt-7 pb-5">
        <div>
          <h1 className="font-heading text-3xl leading-none font-bold tracking-tight">
            Queue
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {paused
              ? "Paused. Nothing new starts until you resume."
              : "Episodes download one at a time, in order. Right-click one to reorder it."}
          </p>
        </div>
        <div className="flex gap-2">
          {sortable && (
            <Select
              value={null}
              onValueChange={(by) => by && void api.SortPending(by as string)}
            >
              <SelectTrigger size="sm" aria-label="Sort waiting episodes">
                <ArrowUpDownIcon className="size-3.5" />
                Sort
              </SelectTrigger>
              <SelectContent>
                {SORTS.map((o) => (
                  <SelectItem key={o.value} value={o.value}>
                    {o.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
          {retryable > 0 && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => void api.RetryFailed()}
            >
              <RotateCwIcon />
              Retry {retryable} failed
            </Button>
          )}
          {finished.length > 0 && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => void api.ClearFinished()}
            >
              Clear finished
            </Button>
          )}
          {(paused || pending) && (
            <Button
              size="sm"
              variant={paused ? "default" : "outline"}
              onClick={() => void (paused ? api.ResumeAll() : api.PauseAll())}
            >
              {paused ? <PlayIcon /> : <PauseIcon />}
              {paused ? "Resume queue" : "Pause queue"}
            </Button>
          )}
        </div>
      </header>
      <ScrollArea className="min-h-0 flex-1">
        {jobs.length === 0 ? (
          <div className="max-w-md px-8 pt-16">
            <p className="font-heading text-2xl font-semibold">
              Nothing queued yet.
            </p>
            <p className="mt-2 text-sm text-muted-foreground">
              Look up a series or episode and choose what to download.
            </p>
            <Button className="mt-5" onClick={onBrowse}>
              Browse episodes
            </Button>
          </div>
        ) : (
          <ol className="divide-y px-8" onDragEnd={() => setDrag(null)}>
            {jobs.map((job, i) => (
              <JobRow
                key={job.id}
                job={job}
                index={i}
                last={jobs.length - 1}
                dragging={drag?.id === job.id}
                dropEdge={
                  drag && drag.id !== job.id && drag.over === i
                    ? jobs.findIndex((j) => j.id === drag.id) < i
                      ? "bottom"
                      : "top"
                    : null
                }
                onDragStart={() => setDrag({ id: job.id, over: i })}
                onDragOver={() =>
                  drag && drag.over !== i && setDrag({ ...drag, over: i })
                }
                onDrop={() => {
                  if (drag && drag.id !== job.id) void api.Move(drag.id, i)
                  setDrag(null)
                }}
              />
            ))}
          </ol>
        )}
      </ScrollArea>
    </div>
  )
}

const SORTS = [
  { value: "episode", label: "By series and episode" },
  { value: "title", label: "By episode title" },
  { value: "added", label: "By date added" },
  { value: "reverse", label: "Reverse order" },
]

function JobRow({
  job,
  index,
  last,
  dragging,
  dropEdge,
  onDragStart,
  onDragOver,
  onDrop,
}: {
  job: Job
  index: number
  last: number
  dragging: boolean
  dropEdge: "top" | "bottom" | null
  onDragStart: () => void
  onDragOver: () => void
  onDrop: () => void
}) {
  const t = job.title
  const status = STATUS[job.status]
  const running =
    job.status === "downloading" ||
    job.status === "merging" ||
    job.status === "waiting"

  return (
    <ContextMenu>
      <ContextMenuTrigger
        render={
          <li
            className={cn(
              "group/row relative -ml-3 flex gap-3 py-4 transition-opacity",
              dragging && "opacity-40",
              dropEdge === "top" &&
                "before:absolute before:inset-x-0 before:-top-px before:h-0.5 before:bg-primary",
              dropEdge === "bottom" &&
                "after:absolute after:inset-x-0 after:-bottom-px after:h-0.5 after:bg-primary"
            )}
            onDragOver={(e) => {
              e.preventDefault()
              e.dataTransfer.dropEffect = "move"
              onDragOver()
            }}
            onDrop={(e) => {
              e.preventDefault()
              onDrop()
            }}
          />
        }
      >
        <div
          draggable
          aria-label="Drag to reorder"
          title="Drag to reorder"
          className="flex w-6 shrink-0 cursor-grab items-center justify-center text-muted-foreground/40 transition-colors group-hover/row:text-muted-foreground active:cursor-grabbing"
          onDragStart={(e) => {
            e.dataTransfer.effectAllowed = "move"
            e.dataTransfer.setData("text/plain", job.id)
            const row = e.currentTarget.parentElement
            if (row) e.dataTransfer.setDragImage(row, 16, 16)
            onDragStart()
          }}
        >
          <GripVerticalIcon className="size-4" />
        </div>
        <div className="relative aspect-video w-36 shrink-0 overflow-hidden rounded-md bg-muted">
          {t.thumbnail && (
            <img src={t.thumbnail} alt="" className="size-full object-cover" />
          )}
          <span className="absolute bottom-0.5 left-2 font-heading text-2xl leading-none font-extrabold text-foreground tabular-nums drop-shadow-[0_1px_6px_rgb(0_0_0/0.7)]">
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
            {job.status === "waiting" && job.retryAt ? (
              <RetryCountdown at={job.retryAt} />
            ) : (
              status.label
            )}
          </div>

          {job.error && job.status !== "waiting" && (
            <p className="selectable mt-1 line-clamp-3 text-xs text-destructive/90">
              {job.error}
            </p>
          )}

          {running && (job.tracks?.length ?? 0) > 0 && (
            <div className="mt-3 flex flex-col gap-2.5">
              {(job.tracks ?? []).map((tr) => (
                <TrackBar key={tr.id} track={tr} />
              ))}
            </div>
          )}
          {job.status === "merging" && (
            <div className="mt-3 h-1 w-full animate-pulse rounded-full bg-primary/40" />
          )}
        </div>
      </ContextMenuTrigger>
      <JobMenu job={job} index={index} last={last} />
    </ContextMenu>
  )
}

function TrackBar({ track }: { track: Track }) {
  const pct =
    track.total > 0 ? Math.min(100, (track.current / track.total) * 100) : null
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
          className={cn(
            "h-full rounded-full bg-primary transition-[width] duration-200",
            pct === null && "w-1/3 animate-pulse"
          )}
          style={pct !== null ? { width: `${pct}%` } : undefined}
        />
      </div>
    </div>
  )
}

const ACTIVE: JobStatus[] = [
  "queued",
  "downloading",
  "merging",
  "waiting",
  "paused",
]
const RUNNING: JobStatus[] = ["downloading", "merging", "waiting"]

/** The controls a job offers, shared by the row buttons and its context menu. */
function jobActions(job: Job) {
  const active = ACTIVE.includes(job.status)
  const finished = job.status === "done" || job.status === "skipped"
  return {
    active,
    reveal: finished && !!job.output,
    retry: job.status === "failed" || job.status === "cancelled",
    resume: job.status === "paused",
    pause: job.status === "queued" || RUNNING.includes(job.status),
    skip: active,
    close: () => void (active ? api.Cancel(job.id) : api.Remove(job.id)),
    closeLabel: active ? "Cancel" : "Remove from list",
  }
}

function Actions({ job }: { job: Job }) {
  const a = jobActions(job)
  return (
    <div className="flex shrink-0 gap-1">
      {a.reveal && (
        <IconAction
          label="Show in folder"
          onClick={() => void api.Reveal(job.output!)}
        >
          <FolderOpenIcon />
        </IconAction>
      )}
      {a.retry && (
        <IconAction label="Try again" onClick={() => void api.Retry(job.id)}>
          <RotateCwIcon />
        </IconAction>
      )}
      {a.resume && (
        <IconAction label="Resume" onClick={() => void api.Resume(job.id)}>
          <PlayIcon />
        </IconAction>
      )}
      {a.pause && (
        <IconAction label="Pause" onClick={() => void api.Pause(job.id)}>
          <PauseIcon />
        </IconAction>
      )}
      {a.skip && (
        <IconAction label="Skip" onClick={() => void api.Skip(job.id)}>
          <SkipForwardIcon />
        </IconAction>
      )}
      <IconAction label={a.closeLabel} onClick={a.close}>
        <XIcon />
      </IconAction>
    </div>
  )
}

function JobMenu({
  job,
  index,
  last,
}: {
  job: Job
  index: number
  last: number
}) {
  const a = jobActions(job)
  const move = (to: number) => void api.Move(job.id, to)
  return (
    <ContextMenuContent className="min-w-48">
      {a.resume && (
        <ContextMenuItem onClick={() => void api.Resume(job.id)}>
          <PlayIcon /> Resume
        </ContextMenuItem>
      )}
      {a.pause && (
        <ContextMenuItem onClick={() => void api.Pause(job.id)}>
          <PauseIcon /> Pause
        </ContextMenuItem>
      )}
      {a.skip && (
        <ContextMenuItem onClick={() => void api.Skip(job.id)}>
          <SkipForwardIcon /> Skip
        </ContextMenuItem>
      )}
      {a.retry && (
        <ContextMenuItem onClick={() => void api.Retry(job.id)}>
          <RotateCwIcon /> Try again
        </ContextMenuItem>
      )}
      {a.reveal && (
        <ContextMenuItem onClick={() => void api.Reveal(job.output!)}>
          <FolderOpenIcon /> Show in folder
        </ContextMenuItem>
      )}
      <ContextMenuSeparator />
      <ContextMenuItem disabled={index === 0} onClick={() => move(0)}>
        <ArrowUpToLineIcon /> Move to top
      </ContextMenuItem>
      <ContextMenuItem disabled={index === 0} onClick={() => move(index - 1)}>
        <ArrowUpIcon /> Move up
      </ContextMenuItem>
      <ContextMenuItem
        disabled={index === last}
        onClick={() => move(index + 1)}
      >
        <ArrowDownIcon /> Move down
      </ContextMenuItem>
      <ContextMenuItem disabled={index === last} onClick={() => move(last)}>
        <ArrowDownToLineIcon /> Move to bottom
      </ContextMenuItem>
      <ContextMenuSeparator />
      <ContextMenuItem
        onClick={() => void api.OpenURL(episodeLink(job.title.id))}
      >
        <ExternalLinkIcon /> Open on Crunchyroll
      </ContextMenuItem>
      <ContextMenuItem variant="destructive" onClick={a.close}>
        <XIcon /> {a.closeLabel}
      </ContextMenuItem>
    </ContextMenuContent>
  )
}

function IconAction({
  label,
  onClick,
  children,
}: {
  label: string
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={label}
            onClick={onClick}
          />
        }
      >
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
  return (
    <>
      Rate-limited by Crunchyroll, retrying in {m > 0 ? `${m}m ${s}s` : `${s}s`}
    </>
  )
}

function formatBytes(n: number) {
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`
  if (n < 1024 ** 3) return `${(n / 1024 ** 2).toFixed(1)} MB`
  return `${(n / 1024 ** 3).toFixed(2)} GB`
}
