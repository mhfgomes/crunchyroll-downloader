import { useEffect, useRef, useState } from "react"
import {
  LockIcon,
  PauseIcon,
  CheckIcon,
  EyeIcon,
  EyeOffIcon,
  FolderIcon,
  Loader2Icon,
  PlusIcon,
  Trash2Icon,
} from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { LangPicker } from "@/components/lang-picker"
import { NumberStepper } from "@/components/number-stepper"
import { useBackend } from "@/hooks/use-backend"
import {
  api,
  errorMessage,
  NS_PER_SECOND,
  type Account,
  type Settings,
} from "@/lib/api"

const VIDEO_QUALITIES = ["1080p", "720p", "480p", "360p", "240p"]
const AUDIO_QUALITIES = ["192k", "128k", "96k"]

export function SettingsView() {
  const { settings, save, env, locked, paused } = useBackend()
  const [draft, setDraft] = useState<Settings | null>(settings)
  const [saving, setSaving] = useState(false)
  const synced = useRef(settings)

  // Follow outside changes, like switching account from the sidebar, unless
  // there are unsaved edits.
  useEffect(() => {
    if (
      settings &&
      (!draft || JSON.stringify(draft) === JSON.stringify(synced.current))
    )
      setDraft(settings)
    synced.current = settings
  }, [settings, draft])

  if (!draft) return null
  const o = draft.options
  const dirty = JSON.stringify(draft) !== JSON.stringify(settings)
  const set = (patch: Partial<Settings>) =>
    setDraft({ ...draft, ...patch } as Settings)
  const setOpt = (patch: Partial<Settings["options"]>) =>
    setDraft({ ...draft, options: { ...o, ...patch } } as Settings)

  async function submit() {
    setSaving(true)
    try {
      const next = await save(draft!)
      if (next.loginError) toast.error(next.loginError)
      else toast.success("Settings saved")
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setSaving(false)
    }
  }

  async function pickDir(
    title: string,
    current: string,
    apply: (p: string) => void
  ) {
    const path = await api.PickDirectory(title, current)
    if (path) apply(path)
  }

  async function pickFile(
    title: string,
    current: string,
    pattern: string,
    apply: (p: string) => void
  ) {
    const path = await api.PickFile(title, current, pattern)
    if (path) apply(path)
  }

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-end justify-between border-b px-8 pt-7 pb-5">
        <div>
          <h1 className="font-heading text-3xl leading-none font-bold tracking-tight">
            Settings
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Applied to every episode you queue from now on.
          </p>
        </div>
        <div className="flex gap-2">
          {dirty && (
            <Button variant="ghost" onClick={() => setDraft(settings)}>
              Discard
            </Button>
          )}
          <Button
            onClick={() => void submit()}
            disabled={!dirty || saving || locked}
          >
            {saving && <Loader2Icon className="animate-spin" />}
            Save changes
          </Button>
        </div>
      </header>

      {locked && (
        <div className="flex items-center gap-3 border-b bg-amber-300/10 px-8 py-2.5 text-sm text-amber-200">
          <LockIcon className="size-4 shrink-0" />
          <span className="flex-1">
            {paused
              ? "The current episode is stopping. Settings unlock in a moment."
              : "Settings are locked while the queue is running, so downloads don't change halfway."}
          </span>
          {!paused && (
            <Button
              size="sm"
              variant="outline"
              onClick={() => void api.PauseAll()}
            >
              <PauseIcon /> Pause queue
            </Button>
          )}
        </div>
      )}
      <ScrollArea className="min-h-0 flex-1">
        <fieldset
          disabled={locked}
          className="max-w-3xl px-8 pb-16 disabled:opacity-60"
        >
          <Section
            title="Accounts"
            description="The downloader signs in with your browser's Crunchyroll session cookie. New episodes are queued with the active account."
          >
            <AccountsEditor
              accounts={draft.accounts ?? []}
              active={draft.activeAccount}
              loginError={env?.loginError}
              mixer={draft.mixer}
              onChange={(accounts, activeAccount) =>
                set({ accounts, activeAccount })
              }
            />
          </Section>

          <Section
            title="Languages"
            description="The first language in each list becomes the default track in the file."
          >
            <Row label="Audio">
              <LangPicker
                value={o.audioLangs}
                onChange={(v) => setOpt({ audioLangs: v })}
                allLabel="Also add every other dub"
                emptyLabel="Japanese"
              />
            </Row>
            <Row label="Subtitles">
              <LangPicker
                value={o.subsLangs}
                onChange={(v) => setOpt({ subsLangs: v })}
                allLabel="Also add every other subtitle language"
                emptyLabel="No subtitles"
              />
            </Row>
            <Row
              label="Closed captions"
              hint="Captions transcribe a dub, and are added alongside subtitles."
            >
              <LangPicker
                value={o.ccLangs}
                onChange={(v) => setOpt({ ccLangs: v })}
                allLabel="Also add every other caption language"
                emptyLabel="No captions"
              />
            </Row>
          </Section>

          <Section title="Quality">
            <Row label="Video">
              <ChoiceSelect
                value={o.videoQuality}
                options={VIDEO_QUALITIES}
                onChange={(v) => setOpt({ videoQuality: v })}
              />
            </Row>
            <Row label="Audio">
              <ChoiceSelect
                value={o.audioQuality}
                options={AUDIO_QUALITIES}
                onChange={(v) => setOpt({ audioQuality: v })}
              />
            </Row>
          </Section>

          <Section title="Files">
            <Row
              label="Save to"
              hint="Each series gets its own folder inside this one."
            >
              <PathInput
                value={o.outputDir}
                onChange={(v) => setOpt({ outputDir: v })}
                onBrowse={() =>
                  void pickDir(
                    "Choose where to save episodes",
                    o.outputDir,
                    (p) => setOpt({ outputDir: p })
                  )
                }
              />
            </Row>
            <Row
              label="Temporary files"
              hint="Tracks are downloaded here and merged into the final file. Pick a drive with room for a few episodes. Leave empty to use the system temp folder."
            >
              <PathInput
                value={o.tempDir}
                placeholder="System temp folder"
                onChange={(v) => setOpt({ tempDir: v })}
                onBrowse={() =>
                  void pickDir(
                    "Choose a folder for temporary files",
                    o.tempDir,
                    (p) => setOpt({ tempDir: p })
                  )
                }
              />
            </Row>
            <Row
              label="Widevine CDM"
              hint="A .wvd file, or a folder holding one (or client_id.bin and private_key.pem). Leave empty to look next to the app."
            >
              <PathInput
                value={o.widevinePath}
                placeholder="Next to the app"
                onChange={(v) => setOpt({ widevinePath: v })}
                onBrowse={() =>
                  void pickFile(
                    "Choose a .wvd file",
                    o.widevinePath,
                    "*.wvd",
                    (p) => setOpt({ widevinePath: p })
                  )
                }
                onBrowseFolder={() =>
                  void pickDir(
                    "Choose the folder with your CDM files",
                    o.widevinePath,
                    (p) => setOpt({ widevinePath: p })
                  )
                }
              />
            </Row>
            <Row
              label="FFmpeg"
              hint={
                env?.ffmpegPath
                  ? `Using ${env.ffmpegPath}`
                  : "Leave empty to find it on PATH."
              }
            >
              <PathInput
                value={draft.ffmpegPath}
                placeholder="Find automatically"
                onChange={(v) => set({ ffmpegPath: v })}
                onBrowse={() =>
                  void pickFile(
                    "Choose the ffmpeg executable",
                    draft.ffmpegPath,
                    "",
                    (p) => set({ ffmpegPath: p })
                  )
                }
              />
            </Row>
          </Section>

          <Section
            title="Rate limits"
            description="Crunchyroll limits how many streams an account can open, and how quickly."
          >
            <Row
              label="Account shuffler"
              hint={
                (draft.accounts ?? []).filter((a) => a.etpRt).length > 1
                  ? "Spreads episodes over the accounts marked Use in shuffler above, each resting for the pause below after an episode. A rate-limited account is benched and the episode moves on to the next rested one."
                  : "Add a second account to spread episodes over accounts and avoid rate limits."
              }
            >
              <Switch
                checked={draft.mixer}
                onCheckedChange={(v) => set({ mixer: v })}
              />
            </Row>
            <Row
              label="Simultaneous streams"
              hint="Keep this within your plan's stream limit."
            >
              <NumberStepper
                label="simultaneous streams"
                value={o.simultaneous}
                min={1}
                max={10}
                onChange={(v) => setOpt({ simultaneous: v })}
              />
            </Row>
            <Row
              label="Pause between episodes"
              hint={
                draft.mixer
                  ? "With the shuffler on, how long each account rests after an episode."
                  : "How long to wait before starting the next episode."
              }
            >
              <DurationRange
                label="pause between episodes"
                min={o.downloadDelay}
                max={o.downloadDelayMax}
                onChange={(min, max) =>
                  setOpt({ downloadDelay: min, downloadDelayMax: max })
                }
              />
            </Row>
            <Row
              label="Rate-limit timeout"
              hint={
                draft.rateLimitTimeout > 0
                  ? draft.mixer
                    ? "How long an account sits out after Crunchyroll rate-limits it. Its episode moves on to another account meanwhile."
                    : "How long an account sits out after Crunchyroll rate-limits it before the queue tries it again."
                  : "Automatic: 1 minute, doubling on repeated rate limits up to 30 minutes. Set a time to always use that instead."
              }
            >
              <DurationRange
                label="rate-limit timeout"
                min={draft.rateLimitTimeout}
                max={draft.rateLimitTimeoutMax}
                onChange={(min, max) =>
                  set({ rateLimitTimeout: min, rateLimitTimeoutMax: max })
                }
              />
            </Row>
            <Row
              label="Log raw manifests"
              hint="Writes playback JSON and manifest XML to Logs, for bug reports."
            >
              <Switch
                checked={o.debugManifest}
                onCheckedChange={(v) => setOpt({ debugManifest: v })}
              />
            </Row>
          </Section>
        </fieldset>
      </ScrollArea>
    </div>
  )
}

function AccountsEditor({
  accounts,
  active,
  loginError,
  mixer,
  onChange,
}: {
  accounts: Account[]
  active: string
  loginError?: string
  mixer: boolean
  onChange: (accounts: Account[], active: string) => void
}) {
  const update = (id: string, patch: Partial<Account>) =>
    onChange(
      accounts.map((a) => (a.id === id ? { ...a, ...patch } : a)),
      active
    )
  const add = () => {
    const id = String(
      Math.max(0, ...accounts.map((a) => Number(a.id) || 0)) + 1
    )
    onChange(
      [...accounts, { id, name: `Account ${accounts.length + 1}`, etpRt: "" }],
      active || id
    )
  }
  const remove = (id: string) => {
    const rest = accounts.filter((a) => a.id !== id)
    onChange(rest, active === id ? (rest[0]?.id ?? "") : active)
  }

  return (
    <div className="flex flex-col gap-3">
      {accounts.map((acc) => (
        <AccountRow
          key={acc.id}
          account={acc}
          active={acc.id === active}
          error={acc.id === active ? loginError : undefined}
          showMixer={mixer && accounts.length > 1}
          onChange={(patch) => update(acc.id, patch)}
          onActivate={() => onChange(accounts, acc.id)}
          onRemove={() => remove(acc.id)}
        />
      ))}
      {accounts.length === 0 && loginError && (
        <p className="text-xs text-destructive">{loginError}</p>
      )}
      <div>
        <Button variant="outline" onClick={add}>
          <PlusIcon /> Add account
        </Button>
      </div>
      <p className="text-xs leading-relaxed text-muted-foreground">
        To get a cookie, sign in on crunchyroll.com, open developer tools
        (Ctrl+Shift+I), then go to Application, Cookies and copy the value of
        etp_rt. Use a private window for each extra account, so signing in
        doesn't replace the other account's cookie.
      </p>
    </div>
  )
}

function AccountRow({
  account,
  active,
  error,
  showMixer,
  onChange,
  onActivate,
  onRemove,
}: {
  account: Account
  active: boolean
  error?: string
  showMixer: boolean
  onChange: (patch: Partial<Account>) => void
  onActivate: () => void
  onRemove: () => void
}) {
  const [showCookie, setShowCookie] = useState(false)
  return (
    <div className="rounded-lg border bg-card/50 p-3">
      <div className="flex gap-2">
        <Input
          className="w-44 shrink-0"
          value={account.name}
          onChange={(e) => onChange({ name: e.target.value })}
          placeholder="Name"
          aria-label="Account name"
        />
        <Input
          type={showCookie ? "text" : "password"}
          value={account.etpRt}
          onChange={(e) => onChange({ etpRt: e.target.value.trim() })}
          placeholder="Paste the etp_rt cookie value"
          aria-label="etp_rt cookie"
          spellCheck={false}
        />
        <Button
          variant="outline"
          size="icon"
          aria-label={showCookie ? "Hide cookie" : "Show cookie"}
          onClick={() => setShowCookie(!showCookie)}
        >
          {showCookie ? <EyeOffIcon /> : <EyeIcon />}
        </Button>
        <Button
          variant="ghost"
          size="icon"
          aria-label="Remove account"
          onClick={onRemove}
        >
          <Trash2Icon />
        </Button>
      </div>
      <div className="mt-2 flex items-center gap-2 text-xs">
        {active ? (
          <span className="flex items-center gap-1 text-emerald-400">
            <CheckIcon className="size-3.5" /> Active
          </span>
        ) : (
          <Button
            variant="link"
            size="xs"
            className="h-auto px-0"
            onClick={onActivate}
          >
            Make active
          </Button>
        )}
        {error && <span className="text-destructive">{error}</span>}
        {showMixer && (
          <label className="ml-auto flex items-center gap-2 text-muted-foreground">
            Use in shuffler
            <Switch
              size="sm"
              checked={!account.noMixer}
              onCheckedChange={(v) => onChange({ noMixer: !v })}
            />
          </label>
        )}
      </div>
    </div>
  )
}

function Section({
  title,
  description,
  children,
}: {
  title: string
  description?: string
  children: React.ReactNode
}) {
  return (
    <section className="border-b py-8 last:border-b-0">
      <h2 className="font-heading text-lg font-semibold">{title}</h2>
      {description && (
        <p className="mt-1 text-sm text-muted-foreground">{description}</p>
      )}
      <div className="mt-5 flex flex-col gap-6">{children}</div>
    </section>
  )
}

function Row({
  label,
  hint,
  children,
}: {
  label: string
  hint?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <div className="grid grid-cols-[180px_1fr] gap-6">
      <div className="pt-1.5 text-sm font-medium">{label}</div>
      <div className="min-w-0">
        {children}
        {hint && (
          <p className="mt-2 text-xs leading-relaxed text-muted-foreground">
            {hint}
          </p>
        )}
      </div>
    </div>
  )
}

function ChoiceSelect({
  value,
  options,
  onChange,
}: {
  value: string
  options: string[]
  onChange: (v: string) => void
}) {
  return (
    <Select value={value} onValueChange={(v) => v && onChange(v as string)}>
      <SelectTrigger className="w-32">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((q) => (
          <SelectItem key={q} value={q}>
            {q}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function PathInput({
  value,
  placeholder,
  onChange,
  onBrowse,
  onBrowseFolder,
}: {
  value: string
  placeholder?: string
  onChange: (v: string) => void
  onBrowse: () => void
  onBrowseFolder?: () => void
}) {
  return (
    <div className="flex gap-2">
      <Input
        value={value}
        placeholder={placeholder}
        onChange={(e) => onChange(e.target.value)}
        spellCheck={false}
      />
      <Button variant="outline" onClick={onBrowse}>
        {onBrowseFolder ? (
          "File…"
        ) : (
          <>
            <FolderIcon /> Browse
          </>
        )}
      </Button>
      {onBrowseFolder && (
        <Button variant="outline" onClick={onBrowseFolder}>
          <FolderIcon /> Folder…
        </Button>
      )}
    </div>
  )
}

// The longest duration a DurationField accepts: a day.
const MAX_SECONDS = 24 * 3600

const UNITS = [
  { value: "s", label: "seconds", seconds: 1 },
  { value: "m", label: "minutes", seconds: 60 },
  { value: "h", label: "hours", seconds: 3600 },
]

/**
 * Edits a duration in nanoseconds, optionally as a random range: with Random
 * on, each use picks a time between min and max.
 */
function DurationRange({
  label,
  min,
  max,
  onChange,
}: {
  label: string
  min: number
  max: number
  onChange: (min: number, max: number) => void
}) {
  const random = max > min
  const secs = (ns: number) => Math.round(ns / NS_PER_SECOND)
  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-2">
        <DurationField
          label={random ? `${label} minimum` : label}
          seconds={secs(min)}
          onChange={(s) => {
            const lo = s * NS_PER_SECOND
            onChange(lo, random ? Math.max(max, lo + NS_PER_SECOND) : 0)
          }}
        />
        {random && (
          <>
            <span className="text-sm text-muted-foreground">to</span>
            <DurationField
              label={`${label} maximum`}
              seconds={secs(max)}
              onChange={(s) =>
                onChange(min, Math.max(s * NS_PER_SECOND, min + NS_PER_SECOND))
              }
            />
          </>
        )}
      </div>
      <label className="flex w-fit cursor-pointer items-center gap-2 text-xs text-muted-foreground">
        <Switch
          size="sm"
          checked={random}
          disabled={min <= 0}
          onCheckedChange={(on) => onChange(min, on ? min * 2 : 0)}
        />
        Random between two values
      </label>
    </div>
  )
}

/** Edits a duration in seconds as an amount plus a unit. */
function DurationField({
  label,
  seconds,
  onChange,
}: {
  label: string
  seconds: number
  onChange: (s: number) => void
}) {
  // Start on the largest unit that represents the saved value exactly.
  const [unit, setUnit] = useState(
    () =>
      [...UNITS]
        .reverse()
        .find((u) => seconds > 0 && seconds % u.seconds === 0) ?? UNITS[0]
  )
  const amount = Math.round(seconds / unit.seconds)

  return (
    <div className="flex items-center gap-2">
      <NumberStepper
        label={label}
        value={amount}
        min={0}
        max={MAX_SECONDS / unit.seconds}
        onChange={(v) => onChange(v * unit.seconds)}
      />
      <Select
        value={unit.value}
        items={UNITS}
        onValueChange={(v) => {
          const next = UNITS.find((u) => u.value === v)
          if (!next) return
          setUnit(next)
          // Keep the same length of time, converted to the new unit. A
          // value that doesn't divide evenly (90s in minutes) rounds to the
          // nearest whole amount, but never down to zero.
          if (seconds % next.seconds !== 0) {
            const n = Math.max(1, Math.round(seconds / next.seconds))
            onChange(n * next.seconds)
          }
        }}
      >
        <SelectTrigger className="w-28" aria-label={`${label} unit`}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {UNITS.map((u) => (
            <SelectItem key={u.value} value={u.value}>
              {u.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}
