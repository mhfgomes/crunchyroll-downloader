import { useEffect, useMemo, useState } from "react"
import { ArrowLeftIcon, Loader2Icon, SearchIcon, XIcon } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Skeleton } from "@/components/ui/skeleton"
import { EpisodeTile, SeriesCard, seriesLink } from "@/components/title-cards"
import { Home } from "@/views/home"
import { useBackend } from "@/hooks/use-backend"
import {
  api,
  errorMessage,
  type LookupResult,
  type SearchResult,
  type SeasonSummary,
  type Title,
} from "@/lib/api"
import { cn } from "@/lib/utils"

const isLink = (s: string) => /crunchyroll\.com\/|\/(watch|series)\//i.test(s)

export function BrowseView({ onQueued, onOpenSettings }: { onQueued: () => void; onOpenSettings: () => void }) {
  const { env } = useBackend()
  const [url, setUrl] = useState("")
  const [searchResults, setSearchResults] = useState<SearchResult[] | null>(null)
  const [searching, setSearching] = useState(false)
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

  const query = url.trim()
  const loggedIn = !!env?.loggedIn

  // Search titles as the user types; links are looked up directly instead.
  useEffect(() => {
    if (query.length < 2 || isLink(query)) {
      setSearching(false)
      return
    }
    let stale = false
    setSearching(true)
    const timer = setTimeout(async () => {
      // Awaited inside try so a synchronous throw from the bridge (e.g. a
      // stale dev build missing the binding) still clears the spinner.
      try {
        const res = await api.Search(query)
        if (stale) return
        setSearchResults(res ?? [])
        setResult(null)
        setError("")
      } catch (err) {
        if (!stale) setError(`Search failed: ${errorMessage(err)}`)
      } finally {
        if (!stale) setSearching(false)
      }
    }, 300)
    return () => {
      stale = true
      clearTimeout(timer)
    }
  }, [query])

  function openSeries(r: SearchResult) {
    void lookup(seriesLink(r.id))
  }

  function clear() {
    setUrl("")
    setSearchResults(null)
    setResult(null)
    setError("")
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

  async function queue(titles: Title[]) {
    if (!loggedIn) {
      toast.error("Add your etp_rt cookie in Settings before downloading.", {
        action: { label: "Settings", onClick: onOpenSettings },
      })
      return false
    }
    titles = [...titles].sort((a, b) => a.season - b.season || a.number - b.number)
    await api.Enqueue(titles)
    toast.success(titles.length === 1 ? `Queued ${titles[0].title}` : `Queued ${titles.length} episodes`, {
      action: { label: "View queue", onClick: onQueued },
    })
    return true
  }

  async function download() {
    const titles = [...selected].map((id) => allTitles.get(id)).filter(Boolean) as Title[]
    if (await queue(titles)) setSelected(new Set())
  }

  // Queues every episode of a series, or only the given seasons.
  async function queueSeries(seriesId: string, seasons?: SeasonSummary[]) {
    if (!loggedIn) return void queue([])
    const id = toast.loading("Collecting episodes…")
    try {
      seasons ??= (await api.Lookup(seriesLink(seriesId))).seasons ?? []
      const lists = await Promise.all(seasons.map((s) => api.SeasonEpisodes(s.id)))
      toast.dismiss(id)
      await queue(lists.flat().filter((t): t is Title => !!t))
    } catch (err) {
      toast.error(errorMessage(err), { id })
    }
  }

  const blocked = !loggedIn
  const home = !result && !loading && searchResults === null && !error

  return (
    <div className="flex h-full flex-col">
      <form
        className={cn(
          "z-10 px-8 py-5",
          home ? "pointer-events-none absolute inset-x-0 top-0 [&>*]:pointer-events-auto" : "border-b"
        )}
        onSubmit={(e) => {
          e.preventDefault()
          if (isLink(query)) void lookup()
          else if (searchResults?.length) openSeries(searchResults[0])
        }}
      >
        <div className="relative">
          {searching || loading ? (
            <Loader2Icon className="pointer-events-none absolute top-1/2 left-3.5 z-10 size-4 -translate-y-1/2 animate-spin text-muted-foreground" />
          ) : (
            <SearchIcon className="pointer-events-none absolute top-1/2 left-3.5 z-10 size-4 -translate-y-1/2 text-muted-foreground" />
          )}
          <Input
            type="search"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder="Search Crunchyroll, or paste a series or episode link"
            className={cn("h-11 rounded-full pr-10 pl-10 text-[15px] [&::-webkit-search-cancel-button]:hidden", home && "border-white/15 bg-background/50 shadow-lg shadow-black/20 backdrop-blur-md dark:bg-background/50")}
            autoFocus
          />
          {url && (
            <button
              type="button"
              aria-label="Clear search"
              onClick={clear}
              className="absolute top-1/2 right-2.5 z-10 flex size-6 -translate-y-1/2 items-center justify-center rounded-full text-muted-foreground hover:bg-accent hover:text-foreground"
            >
              <XIcon className="size-3.5" />
            </button>
          )}
        </div>
        {env && !env.loggedIn && (
          <p className="mt-3 flex w-fit items-center gap-2 rounded-full pr-3 pl-4 text-sm text-muted-foreground">
            <span className="size-1.5 rounded-full bg-destructive" />
            {env.loginError}
            <Button variant="link" size="sm" className="h-auto px-0" onClick={onOpenSettings}>
              Add your cookie in Settings to download
            </Button>
          </p>
        )}
      </form>

      <ScrollArea className="min-h-0 flex-1">
        {home && <Home onOpen={openSeries} onQueue={(id, seasons) => void queueSeries(id, seasons)} />}
        <div className={cn("px-8 pt-7 pb-28", home && "hidden")}>
          {error && <p className="mb-6 text-sm text-destructive selectable">{error}</p>}

          {!result && !loading && searchResults !== null && (
            <SearchResults results={searchResults} query={query} onOpen={openSeries} onQueue={(id, seasons) => void queueSeries(id, seasons)} />
          )}

          {loading && <GridSkeleton />}

          {result && (
            <>
              {searchResults !== null && (
                <Button variant="ghost" size="sm" className="-ml-2 mb-3" onClick={() => setResult(null)}>
                  <ArrowLeftIcon />
                  Search results
                </Button>
              )}
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
                    <EpisodeTile
                      key={t.id}
                      title={t}
                      selected={selected.has(t.id)}
                      selectedCount={selected.size}
                      onToggle={() => toggle(t.id)}
                      onQueue={() => void queue([t])}
                      onQueueSelected={() => void download()}
                    />
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

function SearchResults({
  results,
  query,
  onOpen,
  onQueue,
}: {
  results: SearchResult[]
  query: string
  onOpen: (r: SearchResult) => void
  onQueue: (seriesId: string, seasons?: SeasonSummary[]) => void
}) {
  if (results.length === 0) {
    return (
      <p className="pt-10 text-sm text-muted-foreground">
        No series match “{query}”. Try the original or English title, or paste a link.
      </p>
    )
  }
  return (
    <>
      <p className="mb-5 text-sm text-muted-foreground">Right-click a series to queue it without opening it.</p>
      <div className="grid grid-cols-[repeat(auto-fill,minmax(150px,1fr))] gap-x-5 gap-y-7">
        {results.map((r) => (
          <SeriesCard key={r.id} series={r} onOpen={() => onOpen(r)} onQueue={onQueue} />
        ))}
      </div>
    </>
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
