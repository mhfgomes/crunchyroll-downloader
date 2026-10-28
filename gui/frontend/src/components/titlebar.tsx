import { useEffect, useState } from "react"
import { Events, System, Window } from "@wailsio/runtime"
import { CopyIcon, MinusIcon, SquareIcon, XIcon } from "lucide-react"

import { cn } from "@/lib/utils"

/**
 * The window's title bar, drawn by the app so it matches the rest of the UI.
 * The window is frameless on Windows and Linux, so this draws the window
 * buttons too; on macOS the native traffic lights sit over its left end.
 */
export function Titlebar() {
  const mac = System.IsMac()
  const [maximised, setMaximised] = useState(false)

  useEffect(() => {
    if (mac) return
    const sync = () => void Window.IsMaximised().then(setMaximised)
    sync()
    const offs = [
      Events.On(Events.Types.Common.WindowMaximise, sync),
      Events.On(Events.Types.Common.WindowUnMaximise, sync),
      Events.On(Events.Types.Common.WindowRestore, sync),
    ]
    return () => offs.forEach((off) => off())
  }, [mac])

  return (
    <div
      className="flex h-9 shrink-0 select-none [--wails-draggable:drag]"
      onDoubleClick={() => !mac && void Window.ToggleMaximise()}
    >
      {/* Continues the sidebar up into the title bar. */}
      <div className="w-56 shrink-0 border-r bg-sidebar" />
      <div className="flex-1" />
      {!mac && (
        <div
          className="flex [--wails-draggable:no-drag]"
          onDoubleClick={(e) => e.stopPropagation()}
        >
          <WindowButton label="Minimise" onClick={() => void Window.Minimise()}>
            <MinusIcon />
          </WindowButton>
          <WindowButton
            label={maximised ? "Restore" : "Maximise"}
            onClick={() => void Window.ToggleMaximise()}
          >
            {maximised ? (
              <CopyIcon className="!size-3.5 -scale-x-100" />
            ) : (
              <SquareIcon className="!size-3.5" />
            )}
          </WindowButton>
          <WindowButton
            label="Close"
            danger
            onClick={() => void Window.Close()}
          >
            <XIcon />
          </WindowButton>
        </div>
      )}
    </div>
  )
}

function WindowButton({
  label,
  danger,
  onClick,
  children,
}: {
  label: string
  danger?: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      onClick={onClick}
      className={cn(
        "flex h-9 w-12 items-center justify-center text-muted-foreground transition-colors [&_svg]:size-4 [&_svg]:stroke-[1.5]",
        danger
          ? "hover:bg-[#c42b1c] hover:text-white"
          : "hover:bg-accent hover:text-foreground"
      )}
    >
      {children}
    </button>
  )
}
