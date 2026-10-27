import { useEffect, useRef } from "react"
import { CopyIcon } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { useBackend } from "@/hooks/use-backend"

export function LogsView() {
  const { logs } = useBackend()
  const ref = useRef<HTMLDivElement>(null)
  const stick = useRef(true)

  useEffect(() => {
    const el = ref.current
    if (el && stick.current) el.scrollTop = el.scrollHeight
  }, [logs])

  async function copy() {
    await navigator.clipboard.writeText(logs.map((l) => `${l.time} ${l.text}`).join("\n"))
    toast.success("Copied the log")
  }

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-end justify-between border-b px-8 pt-7 pb-5">
        <div>
          <h1 className="font-heading text-3xl leading-none font-bold tracking-tight">Logs</h1>
          <p className="mt-2 text-sm text-muted-foreground">What the downloader reported, newest at the bottom.</p>
        </div>
        <Button variant="outline" size="sm" onClick={() => void copy()} disabled={!logs.length}>
          <CopyIcon />
          Copy all
        </Button>
      </header>
      <div
        ref={ref}
        onScroll={(e) => {
          const el = e.currentTarget
          stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40
        }}
        className="selectable min-h-0 flex-1 overflow-auto px-8 py-5 font-mono text-[12.5px] leading-6"
      >
        {logs.length === 0 ? (
          <p className="font-sans text-sm text-muted-foreground">Nothing logged yet. Messages appear here once a download starts.</p>
        ) : (
          logs.map((l, i) => (
            <div key={i} className={l.text.startsWith("!") || /error|failed/i.test(l.text) ? "text-destructive" : undefined}>
              <span className="mr-3 text-muted-foreground/70">{l.time}</span>
              <span className="whitespace-pre-wrap">{l.text}</span>
            </div>
          ))
        )}
      </div>
    </div>
  )
}
