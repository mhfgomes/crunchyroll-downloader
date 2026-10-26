import { useEffect, useMemo, useState } from "react"
import { CheckIcon, ClipboardPasteIcon, Loader2Icon, SearchIcon } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Skeleton } from "@/components/ui/skeleton"
import { useBackend } from "@/hooks/use-backend"
import {
  api,
  audioLocales,
  episodeCode,
  errorMessage,
  type LookupResult,
  type Title,
} from "@/lib/api"
import { cn } from "@/lib/utils"

export function BrowseView({ onQueued }: { onQueued: () => void }) {
  const { env } = useBackend()
  const [url, setUrl] = useState("")
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState("")
  const [result, setResult] = useState<LookupResult | null>(null)
  const [seasonId, setSeasonId] = useState("")
  const [episodes, setEpisodes] = useState<Record<string, Title[]>>({})
  const [loadingSeason, setLoadingSeason] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())

  async function lookup(target = url) {
    const value = target.trim()
    if (!value) return
    setLoading(true)
    setError("")
    setResult(null)
    setEpisodes({})
    setSelected(new Set())
    try {
      const res = await api.Lookup(value)
      setResult(res)
      if (res.kind === "episode" && res.episode) {
        setSelected(new Set([res.episode.id]))
      } else if (res.seasons?.length) {
        setSeasonId(res.seasons[0].id)
      }
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setLoading(false)
    }
  }

  async function paste() {
    try {
      const text = await navigator.clipboard.readText()
      setUrl(text)
      void lookup(text)
    } catch {
      toast.error("Couldn't read the clipboard. Paste the link with Ctrl+V instead.")
    }
  }

  useEffect(() => {
    if (!seasonId || episodes[seasonId]) return
    let stale = false
    setLoadingSeason(true)
    api
      .SeasonEpisodes(seasonId)
      .then((eps) => !stale && setEpisodes((prev) => ({ ...prev, [seasonId]: eps ?? [] })))
      .catch((err) => !stale && setError(errorMessage(err)))
      .finally(() => !stale && setLoadingSeason(false))
    return () => {
      stale = true
    }
  }, [seasonId, episodes])

  const visible: Title[] = useMemo(() => {
    if (result?.kind === "episode" && result.episode) return [result.episode]
    return episodes[seasonId] ?? []
  }, [result, episodes, seasonId])

  const allTitles = useMemo(() => {
    const map = new Map<string, Title>()
    if (result?.episode) map.set(result.episode.id, result.episode)
    Object.values(episodes).flat().forEach((t) => map.set(t.id, t))
    return map
  }, [result, episodes])

  const seriesTitle = result?.seriesTitle || visible[0]?.seriesTitle || ""
  const allVisibleSelected = visible.length > 0 && visible.every((t) => selected.has(t.id))

  function toggle(id: string) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  function toggleAll() {
    setSelected((prev) => {
      const next = new Set(prev)
      visible.forEach((t) => (allVisibleSelected ? next.delete(t.id) : next.add(t.id)))
      return next
    })
  }

  async function download() {
    const titles = [...selected].map((id) => allTitles.get(id)).filter(Boolean) as Title[]
    titles.sort((a, b) => a.season - b.season || a.number - b.number)
    await api.Enqueue(titles)
    toast.success(
      titles.length === 1 ? `Queued ${titles[0].title}` : `Queued ${titles.length} episodes`
    )
    setSelected(new Set())
    onQueued()
  }

  const blocked = env && !env.loggedIn

  return (
    <div className="flex h-full flex-col">
      <form
        className="flex items-center gap-2 border-b px-8 py-5"
        onSubmit={(e) => {
          e.preventDefault()
          void lookup()
        }}
      >
        <div className="relative flex-1">
          <SearchIcon className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder="Paste a crunchyroll.com/series/… or /watch/… link"
            className="h-10 pl-9 text-sm"
            autoFocus
          />
        </div>
        <Button type="button" variant="outline" size="lg" className="h-10" onClick={() => void paste()}>
          <ClipboardPasteIcon />
          Paste
        </Button>
        <Button type="submit" size="lg" className="h-10 px-4" disabled={loading || !url.trim() || !!blocked}>
          {loading && <Loader2Icon className="animate-spin" />}
          Look up
        </Button>
      </form>

      {blocked && (
        <p className="border-b bg-destructive/10 px-8 py-2.5 text-sm text-destructive">
          {env.loginError} Add it in Settings to look up episodes.
        </p>
      )}

      <ScrollArea className="min-h-0 flex-1">
        <div className="px-8 pt-7 pb-28">
          {error && <p className="mb-6 text-sm text-destructive selectable">{error}</p>}

          {!result && !loading && !error && <Intro />}

          {loading && <GridSkeleton />}

          {result && (
            <>
              <h1 className="font-heading text-4xl leading-none font-bold tracking-tight">
                {seriesTitle || <Skeleton className="h-9 w-72" />}
              </h1>

              {result.kind === "series" && (
                <div className="mt-6 flex flex-wrap gap-2">
                  {result.seasons?.map((s) => (
                    <button
                      key={s.id}
                      onClick={() => setSeasonId(s.id)}
                      className={cn(
                        "rounded-full border px-3.5 py-1.5 text-sm transition-colors",
                        s.id === seasonId
                          ? "border-select bg-select/15 text-foreground"
                          : "text-muted-foreground hover:text-foreground"
                      )}
                    >
                      {s.title || `Season ${s.number}`}
                      {s.episodeCount > 0 && (
                        <span className="ml-1.5 text-muted-foreground tabular-nums">{s.episodeCount}</span>
                      )}
                    </button>
                  ))}
                </div>
              )}

              {visible.length > 0 && (
                <div className="mt-8 mb-4 flex items-center justify-between">
                  <p className="text-sm text-muted-foreground">
                    {visible.length === 1 ? "1 episode" : `${visible.length} episodes`}
                  </p>
                  {visible.length > 1 && (
                    <Button variant="ghost" size="sm" onClick={toggleAll}>
                      {allVisibleSelected ? "Clear selection" : "Select all"}
                    </Button>
                  )}
                </div>
              )}

              {loadingSeason && !visible.length ? (
                <GridSkeleton />
              ) : (
                <div className="grid grid-cols-[repeat(auto-fill,minmax(240px,1fr))] gap-x-5 gap-y-7">
                  {visible.map((t) => (
                    <EpisodeTile key={t.id} title={t} selected={selected.has(t.id)} onToggle={() => toggle(t.id)} />
                  ))}
                </div>
              )}
            </>
          )}
        </div>
      </ScrollArea>

      {selected.size > 0 && (
        <div className="pointer-events-none absolute inset-x-0 bottom-6 flex justify-center">
          <div className="pointer-events-auto flex items-center gap-4 rounded-full border bg-popover py-2 pr-2 pl-5 shadow-2xl shadow-black/40">
            <span className="text-sm">
              {selected.size === 1 ? "1 episode selected" : `${selected.size} episodes selected`}
            </span>
            <Button variant="ghost" size="sm" onClick={() => setSelected(new Set())}>
              Clear
            </Button>
            <Button className="rounded-full px-4" onClick={() => void download()} disabled={!!blocked}>
              Download
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}

function EpisodeTile({ title, selected, onToggle }: { title: Title; selected: boolean; onToggle: () => void }) {
  const { languageName } = useBackend()
  const dubs = audioLocales(title)
  return (
    <button
      onClick={onToggle}
      aria-pressed={selected}
      className="group text-left outline-none"
    >
      <div
        className={cn(
          "relative aspect-video overflow-hidden rounded-md bg-muted ring-2 ring-transparent transition-shadow group-focus-visible:ring-ring",
          selected && "ring-select"
        )}
      >
        {title.thumbnail && (
          <img
            src={title.thumbnail}
            alt=""
            loading="lazy"
            className={cn("size-full object-cover transition-opacity", selected ? "opacity-60" : "opacity-90 group-hover:opacity-100")}
          />
        )}
        <div className="absolute inset-0 bg-gradient-to-t from-background/90 via-background/10 to-transparent" />
        <span className="absolute bottom-1 left-3 font-heading text-5xl leading-none font-extrabold tracking-tighter text-foreground/95 tabular-nums">
          {title.number || "—"}
        </span>
        <span
          className={cn(
            "absolute top-2 right-2 flex size-6 items-center justify-center rounded-full border border-white/40 bg-background/40 backdrop-blur transition-colors",
            selected && "border-select bg-select text-background"
          )}
        >
          {selected && <CheckIcon className="size-3.5" strokeWidth={3} />}
        </span>
      </div>
      <div className="mt-2.5 line-clamp-2 text-sm leading-snug font-medium">{title.title}</div>
      <div className="mt-1 truncate text-xs text-muted-foreground">
        {episodeCode(title)}
        {dubs.length > 0 && `, ${dubs.length === 1 ? languageName(dubs[0]) : `${dubs.length} audio languages`}`}
      </div>
    </button>
  )
}

function GridSkeleton() {
  return (
    <div className="mt-8 grid grid-cols-[repeat(auto-fill,minmax(240px,1fr))] gap-x-5 gap-y-7">
      {Array.from({ length: 8 }, (_, i) => (
        <div key={i}>
          <Skeleton className="aspect-video rounded-md" />
          <Skeleton className="mt-2.5 h-4 w-3/4" />
          <Skeleton className="mt-1.5 h-3 w-1/3" />
        </div>
      ))}
    </div>
  )
}

function Intro() {
  return (
    <div className="max-w-lg pt-16">
      <h1 className="font-heading text-5xl leading-[1.05] font-bold tracking-tight">
        What are we watching tonight?
      </h1>
      <p className="mt-4 text-base leading-relaxed text-muted-foreground">
        Paste a series link to pick episodes from any season, or an episode link to grab just that one.
        Files are saved as MKV with every audio and subtitle track you chose in Settings.
      </p>
    </div>
  )
}
