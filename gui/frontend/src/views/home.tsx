import { useEffect, useRef, useState } from "react"
import { ChevronLeftIcon, ChevronRightIcon, ListPlusIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { SeriesCard } from "@/components/title-cards"
import { api, type SearchResult, type SeasonSummary } from "@/lib/api"

type Shelves = { popular: SearchResult[]; fresh: SearchResult[] }

// The catalogue barely changes within a session, so keep it across visits.
let cache: Shelves | null = null

/**
 * The browse landing page: the most-watched series as a full-bleed backdrop,
 * followed by shelves of popular and newly added series.
 */
export function Home({
  onOpen,
  onQueue,
}: {
  onOpen: (r: SearchResult) => void
  onQueue: (seriesId: string, seasons?: SeasonSummary[]) => void
}) {
  const [shelves, setShelves] = useState<Shelves | null>(cache)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    if (cache) return
    Promise.all([api.Browse("popularity", 20), api.Browse("newly_added", 20)])
      .then(([popular, fresh]) => {
        cache = { popular: popular ?? [], fresh: fresh ?? [] }
        setShelves(cache)
      })
      .catch(() => setFailed(true))
  }, [])

  if (failed) {
    return (
      <div className="px-8 pt-36 text-sm text-muted-foreground">
        Couldn't load the Crunchyroll catalogue. Search above, or paste a series or episode link.
      </div>
    )
  }

  const hero = shelves?.popular.find((s) => s.backdrop)

  return (
    <div className="pb-16">
      {hero ? <Hero series={hero} onOpen={() => onOpen(hero)} onQueue={() => onQueue(hero.id)} /> : <HeroSkeleton />}
      <Shelf
        title="Popular on Crunchyroll"
        items={shelves?.popular.filter((s) => s !== hero)}
        onOpen={onOpen}
        onQueue={onQueue}
      />
      <Shelf title="Just added" items={shelves?.fresh} onOpen={onOpen} onQueue={onQueue} />
    </div>
  )
}

function Hero({ series, onOpen, onQueue }: { series: SearchResult; onOpen: () => void; onQueue: () => void }) {
  return (
    <section className="relative isolate flex min-h-[min(72vh,620px)] items-end overflow-hidden pt-36">
      <img
        src={series.backdrop}
        alt=""
        className="absolute inset-0 -z-10 size-full animate-in object-cover object-top duration-700 fade-in"
      />
      {/* Grade the art into the page: darker toward the copy and the bottom edge. */}
      <div className="absolute inset-0 -z-10 bg-gradient-to-r from-background via-background/55 to-background/0" />
      <div className="absolute inset-0 -z-10 bg-gradient-to-t from-background via-background/10 to-background/40" />

      <div className="max-w-xl px-8 pb-10">
        <p className="text-sm text-muted-foreground">Most watched right now</p>
        <h1 className="mt-2 font-heading text-[clamp(2.5rem,5vw,4.25rem)] leading-[0.95] font-extrabold tracking-tight text-balance">
          {series.title}
        </h1>
        {series.description && (
          <p className="mt-4 line-clamp-3 max-w-[60ch] text-[15px] leading-relaxed text-foreground/75">
            {series.description}
          </p>
        )}
        <div className="mt-6 flex flex-wrap items-center gap-2.5">
          <Button size="lg" className="h-10 rounded-full px-5" onClick={onOpen}>
            Choose episodes
          </Button>
          <Button
            size="lg"
            variant="outline"
            className="h-10 rounded-full border-white/20 bg-background/30 px-5 backdrop-blur"
            onClick={onQueue}
          >
            <ListPlusIcon />
            {series.episodeCount > 0 ? `Add all ${series.episodeCount} episodes` : "Add all episodes"}
          </Button>
        </div>
      </div>
    </section>
  )
}

function HeroSkeleton() {
  return (
    <section className="flex min-h-[min(72vh,620px)] items-end px-8 pt-36 pb-10">
      <div className="w-full max-w-xl">
        <Skeleton className="h-4 w-40" />
        <Skeleton className="mt-3 h-14 w-4/5" />
        <Skeleton className="mt-5 h-4 w-full" />
        <Skeleton className="mt-2 h-4 w-2/3" />
      </div>
    </section>
  )
}

function Shelf({
  title,
  items,
  onOpen,
  onQueue,
}: {
  title: string
  items?: SearchResult[]
  onOpen: (r: SearchResult) => void
  onQueue: (seriesId: string, seasons?: SeasonSummary[]) => void
}) {
  const row = useRef<HTMLDivElement>(null)
  const scroll = (dir: number) => row.current?.scrollBy({ left: dir * row.current.clientWidth * 0.8, behavior: "smooth" })

  return (
    <section className="mt-8">
      <div className="flex items-center justify-between px-8">
        <h2 className="font-heading text-xl font-bold tracking-tight">{title}</h2>
        <div className="flex gap-1">
          <Button variant="ghost" size="icon-sm" aria-label={`Scroll ${title} left`} onClick={() => scroll(-1)}>
            <ChevronLeftIcon />
          </Button>
          <Button variant="ghost" size="icon-sm" aria-label={`Scroll ${title} right`} onClick={() => scroll(1)}>
            <ChevronRightIcon />
          </Button>
        </div>
      </div>
      <div
        ref={row}
        className="mt-4 flex snap-x scroll-px-8 gap-4 overflow-x-auto px-8 pb-2 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
      >
        {items
          ? items.map((r) => (
              <div key={r.id} className="w-[152px] shrink-0 snap-start">
                <SeriesCard series={r} onOpen={() => onOpen(r)} onQueue={onQueue} />
              </div>
            ))
          : Array.from({ length: 8 }, (_, i) => (
              <div key={i} className="w-[152px] shrink-0">
                <Skeleton className="aspect-[2/3] rounded-md" />
                <Skeleton className="mt-2.5 h-4 w-3/4" />
              </div>
            ))}
      </div>
    </section>
  )
}
