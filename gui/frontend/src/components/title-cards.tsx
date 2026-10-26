import { useState } from "react"
import {
  CheckIcon,
  ExternalLinkIcon,
  LinkIcon,
  ListPlusIcon,
  Loader2Icon,
  MousePointerClickIcon,
  SquareCheckIcon,
} from "lucide-react"
import { toast } from "sonner"

import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
  ContextMenuTrigger,
} from "@/components/ui/context-menu"
import { useBackend } from "@/hooks/use-backend"
import { api, audioLocales, episodeCode, type SearchResult, type SeasonSummary, type Title } from "@/lib/api"
import { cn } from "@/lib/utils"

export const seriesLink = (id: string) => `https://www.crunchyroll.com/series/${id}`
export const episodeLink = (id: string) => `https://www.crunchyroll.com/watch/${id}`

function LinkItems({ link }: { link: string }) {
  return (
    <>
      <ContextMenuItem
        onClick={() => {
          void navigator.clipboard.writeText(link)
          toast.success("Link copied")
        }}
      >
        <LinkIcon />
        Copy link
      </ContextMenuItem>
      <ContextMenuItem onClick={() => void api.OpenURL(link)}>
        <ExternalLinkIcon />
        Open on Crunchyroll
      </ContextMenuItem>
    </>
  )
}

/** An episode still; click toggles selection, right-click opens actions. */
export function EpisodeTile({
  title,
  selected,
  selectedCount,
  onToggle,
  onQueue,
  onQueueSelected,
}: {
  title: Title
  selected: boolean
  selectedCount: number
  onToggle: () => void
  onQueue: () => void
  onQueueSelected: () => void
}) {
  const { languageName } = useBackend()
  const dubs = audioLocales(title)

  return (
    <ContextMenu>
      <ContextMenuTrigger
        render={<button onClick={onToggle} aria-pressed={selected} className="group text-left outline-none" />}
      >
        <div
          className={cn(
            "relative aspect-video overflow-hidden rounded-md bg-muted ring-2 ring-transparent transition-shadow group-focus-visible:ring-ring group-data-[popup-open]:ring-select/60",
            selected && "ring-select"
          )}
        >
          {title.thumbnail && (
            <img
              src={title.thumbnail}
              alt=""
              loading="lazy"
              className={cn(
                "size-full object-cover transition-opacity",
                selected ? "opacity-60" : "opacity-90 group-hover:opacity-100"
              )}
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
      </ContextMenuTrigger>
      <ContextMenuContent className="w-60">
        <ContextMenuItem onClick={onQueue}>
          <ListPlusIcon />
          Add to queue
        </ContextMenuItem>
        {selected && selectedCount > 1 && (
          <ContextMenuItem onClick={onQueueSelected}>
            <ListPlusIcon />
            Add {selectedCount} selected to queue
          </ContextMenuItem>
        )}
        <ContextMenuItem onClick={onToggle}>
          <SquareCheckIcon />
          {selected ? "Deselect" : "Select"}
        </ContextMenuItem>
        <ContextMenuSeparator />
        <LinkItems link={episodeLink(title.id)} />
      </ContextMenuContent>
    </ContextMenu>
  )
}

/** A search result poster; click opens the series, right-click queues it. */
export function SeriesCard({
  series: r,
  onOpen,
  onQueue,
}: {
  series: SearchResult
  onOpen: () => void
  onQueue: (seriesId: string, seasons?: SeasonSummary[]) => void
}) {
  // Seasons load the first time the menu opens, for the per-season submenu.
  const [seasons, setSeasons] = useState<SeasonSummary[] | null>(null)

  return (
    <ContextMenu
      onOpenChange={(open) => {
        if (open && seasons === null && r.seasonCount > 1) {
          api
            .Lookup(seriesLink(r.id))
            .then((res) => setSeasons(res.seasons ?? []))
            .catch(() => setSeasons([]))
        }
      }}
    >
      <ContextMenuTrigger render={<button onClick={onOpen} className="group text-left outline-none" />}>
        <div className="aspect-[2/3] overflow-hidden rounded-md bg-muted ring-2 ring-transparent transition-shadow group-hover:ring-select/60 group-focus-visible:ring-ring group-data-[popup-open]:ring-select">
          {r.poster && <img src={r.poster} alt="" loading="lazy" className="size-full object-cover" />}
        </div>
        <div className="mt-2.5 line-clamp-2 text-sm leading-snug font-medium">{r.title}</div>
        <div className="mt-1 text-xs text-muted-foreground">
          {r.episodeCount > 0 && `${r.episodeCount} episodes`}
          {r.seasonCount > 1 && `, ${r.seasonCount} seasons`}
        </div>
      </ContextMenuTrigger>
      <ContextMenuContent className="w-64">
        <ContextMenuItem onClick={onOpen}>
          <MousePointerClickIcon />
          Choose episodes
        </ContextMenuItem>
        <ContextMenuItem onClick={() => onQueue(r.id)}>
          <ListPlusIcon />
          {r.episodeCount > 0 ? `Add all ${r.episodeCount} episodes to queue` : "Add all episodes to queue"}
        </ContextMenuItem>
        {r.seasonCount > 1 && (
          <ContextMenuSub>
            <ContextMenuSubTrigger>
              <ListPlusIcon />
              Add a season to queue
            </ContextMenuSubTrigger>
            <ContextMenuSubContent className="max-h-80 w-64 overflow-y-auto">
              {seasons === null ? (
                <ContextMenuItem disabled>
                  <Loader2Icon className="animate-spin" />
                  Loading seasons…
                </ContextMenuItem>
              ) : (
                seasons.map((s) => (
                  <ContextMenuItem key={s.id} onClick={() => onQueue(r.id, [s])}>
                    <span className="truncate">{s.title || `Season ${s.number}`}</span>
                    {s.episodeCount > 0 && (
                      <span className="ml-auto text-xs text-muted-foreground tabular-nums">{s.episodeCount}</span>
                    )}
                  </ContextMenuItem>
                ))
              )}
            </ContextMenuSubContent>
          </ContextMenuSub>
        )}
        <ContextMenuSeparator />
        <LinkItems link={seriesLink(r.id)} />
      </ContextMenuContent>
    </ContextMenu>
  )
}
