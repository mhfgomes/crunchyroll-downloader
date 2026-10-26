import { useState } from "react"
import {
  ListOrderedIcon,
  ScrollTextIcon,
  SearchIcon,
  SettingsIcon,
} from "lucide-react"
import { cn } from "@/lib/utils"

import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { EnvironmentStatus } from "@/components/environment-status"
import { BackendContext, useBackendState } from "@/hooks/use-backend"
import { BrowseView } from "@/views/browse"
import { LogsView } from "@/views/logs"
import { QueueView } from "@/views/queue"
import { SettingsView } from "@/views/settings"

export type View = "browse" | "queue" | "settings" | "logs"

const NAV: { id: View; label: string; icon: typeof SearchIcon }[] = [
  { id: "browse", label: "Browse", icon: SearchIcon },
  { id: "queue", label: "Queue", icon: ListOrderedIcon },
  { id: "settings", label: "Settings", icon: SettingsIcon },
  { id: "logs", label: "Logs", icon: ScrollTextIcon },
]

export function App() {
  const backend = useBackendState()
  const [view, setView] = useState<View>("browse")
  const active = backend.jobs.filter((j) =>
    ["queued", "downloading", "merging", "waiting"].includes(j.status)
  ).length

  return (
    <BackendContext.Provider value={backend}>
      <TooltipProvider>
        <div className="flex h-full">
          <aside className="flex w-56 shrink-0 flex-col border-r bg-sidebar">
            <div className="px-4 pt-6 pb-7 font-heading text-[22px] leading-[0.95] font-extrabold tracking-tight">
              Crunchyroll
              <br />
              <span className="text-primary">downloader</span>
            </div>
            <nav className="flex flex-col gap-0.5 px-2">
              {NAV.map(({ id, label, icon: Icon }) => (
                <button
                  key={id}
                  onClick={() => setView(id)}
                  className={cn(
                    "flex h-9 items-center gap-2.5 rounded-md px-2.5 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-foreground",
                    view === id && "bg-accent font-medium text-foreground"
                  )}
                >
                  <Icon className="size-4" />
                  {label}
                  {id === "queue" && active > 0 && (
                    <span className="ml-auto rounded-full bg-primary px-1.5 text-[11px] leading-[18px] font-semibold text-primary-foreground tabular-nums">
                      {active}
                    </span>
                  )}
                </button>
              ))}
            </nav>
            <div className="mt-auto p-3">
              <EnvironmentStatus onOpenSettings={() => setView("settings")} />
            </div>
          </aside>
          <main className="relative min-w-0 flex-1">
            {view === "browse" && (
              <BrowseView onQueued={() => setView("queue")} onOpenSettings={() => setView("settings")} />
            )}
            {view === "queue" && <QueueView onBrowse={() => setView("browse")} />}
            {view === "settings" && <SettingsView />}
            {view === "logs" && <LogsView />}
          </main>
        </div>
        <Toaster position="bottom-right" />
      </TooltipProvider>
    </BackendContext.Provider>
  )
}

export default App
