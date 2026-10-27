import { PlusIcon, XIcon } from "lucide-react"

import { Select, SelectContent, SelectItem, SelectTrigger } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { useBackend } from "@/hooks/use-backend"

function split(value: string) {
  return value
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean)
}

/**
 * Edits a comma-separated locale list. Order matters: the first language
 * becomes the default track, and ALL appends every other available one.
 */
export function LangPicker({
  value,
  onChange,
  allLabel,
  emptyLabel,
}: {
  value: string
  onChange: (v: string) => void
  allLabel: string
  emptyLabel: string
}) {
  const { languages, languageName } = useBackend()
  const parts = split(value)
  const hasAll = parts.some((p) => p.toUpperCase() === "ALL")
  const langs = parts.filter((p) => p.toUpperCase() !== "ALL")
  const available = languages.filter((l) => !langs.includes(l.code))

  const emit = (next: string[], all = hasAll) => onChange([...next, ...(all ? ["ALL"] : [])].join(","))

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-1.5">
        {langs.length === 0 && <span className="text-sm text-muted-foreground">{emptyLabel}</span>}
        {langs.map((code, i) => (
          <span
            key={code}
            className="flex h-7 items-center gap-1 rounded-full border bg-secondary pr-1 pl-3 text-sm"
          >
            {languageName(code)}
            {i === 0 && <span className="text-xs text-muted-foreground">default</span>}
            <button
              type="button"
              aria-label={`Remove ${languageName(code)}`}
              className="flex size-5 items-center justify-center rounded-full text-muted-foreground hover:bg-accent hover:text-foreground"
              onClick={() => emit(langs.filter((c) => c !== code))}
            >
              <XIcon className="size-3" />
            </button>
          </span>
        ))}
        <Select value={null} onValueChange={(code) => code && emit([...langs, code as string])}>
          <SelectTrigger size="sm" className="rounded-full" aria-label="Add language">
            <PlusIcon className="size-3.5" />
            Add
          </SelectTrigger>
          <SelectContent>
            {available.map((l) => (
              <SelectItem key={l.code} value={l.code}>
                {l.name}
                <span className="text-muted-foreground">{l.code}</span>
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <label className="flex items-center gap-2.5 text-sm text-muted-foreground">
        <Switch checked={hasAll} onCheckedChange={(on) => emit(langs, on)} />
        {allLabel}
      </label>
    </div>
  )
}
