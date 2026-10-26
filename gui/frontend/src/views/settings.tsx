import { useEffect, useState } from "react"
import { EyeIcon, EyeOffIcon, FolderIcon, Loader2Icon } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { LangPicker } from "@/components/lang-picker"
import { NumberStepper } from "@/components/number-stepper"
import { useBackend } from "@/hooks/use-backend"
import { api, errorMessage, NS_PER_SECOND, type Settings } from "@/lib/api"

const VIDEO_QUALITIES = ["1080p", "720p", "480p", "360p", "240p"]
const AUDIO_QUALITIES = ["192k", "128k", "96k"]

export function SettingsView() {
  const { settings, save, env } = useBackend()
  const [draft, setDraft] = useState<Settings | null>(settings)
  const [saving, setSaving] = useState(false)
  const [showCookie, setShowCookie] = useState(false)

  useEffect(() => {
    if (settings && !draft) setDraft(settings)
  }, [settings, draft])

  if (!draft) return null
  const o = draft.options
  const dirty = JSON.stringify(draft) !== JSON.stringify(settings)
  const set = (patch: Partial<Settings>) => setDraft({ ...draft, ...patch } as Settings)
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

  async function pickDir(title: string, current: string, apply: (p: string) => void) {
    const path = await api.PickDirectory(title, current)
    if (path) apply(path)
  }

  async function pickFile(title: string, current: string, pattern: string, apply: (p: string) => void) {
    const path = await api.PickFile(title, current, pattern)
    if (path) apply(path)
  }

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-end justify-between border-b px-8 pt-7 pb-5">
        <div>
          <h1 className="font-heading text-3xl leading-none font-bold tracking-tight">Settings</h1>
          <p className="mt-2 text-sm text-muted-foreground">Applied to every episode you queue from now on.</p>
        </div>
        <div className="flex gap-2">
          {dirty && (
            <Button variant="ghost" onClick={() => setDraft(settings)}>
              Discard
            </Button>
          )}
          <Button onClick={() => void submit()} disabled={!dirty || saving}>
            {saving && <Loader2Icon className="animate-spin" />}
            Save changes
          </Button>
        </div>
      </header>

      <ScrollArea className="min-h-0 flex-1">
        <div className="max-w-3xl px-8 pb-16">
          <Section title="Account" description="The downloader signs in with your browser's Crunchyroll session cookie.">
            <Row
              label="etp_rt cookie"
              hint={
                <>
                  Sign in on crunchyroll.com, open developer tools (Ctrl+Shift+I), then go to Application, Cookies and copy
                  the value of etp_rt.
                  {env?.loginError && <span className="mt-1 block text-destructive">{env.loginError}</span>}
                </>
              }
            >
              <div className="flex gap-2">
                <Input
                  type={showCookie ? "text" : "password"}
                  value={draft.etpRt}
                  onChange={(e) => set({ etpRt: e.target.value.trim() })}
                  placeholder="Paste the cookie value"
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
              </div>
            </Row>
          </Section>

          <Section title="Languages" description="The first language in each list becomes the default track in the file.">
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
            <Row label="Closed captions" hint="Captions transcribe a dub, and are added alongside subtitles.">
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
              <ChoiceSelect value={o.videoQuality} options={VIDEO_QUALITIES} onChange={(v) => setOpt({ videoQuality: v })} />
            </Row>
            <Row label="Audio">
              <ChoiceSelect value={o.audioQuality} options={AUDIO_QUALITIES} onChange={(v) => setOpt({ audioQuality: v })} />
            </Row>
          </Section>

          <Section title="Files">
            <Row label="Save to" hint="Each series gets its own folder inside this one.">
              <PathInput
                value={o.outputDir}
                onChange={(v) => setOpt({ outputDir: v })}
                onBrowse={() => void pickDir("Choose where to save episodes", o.outputDir, (p) => setOpt({ outputDir: p }))}
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
                  void pickFile("Choose a .wvd file", o.widevinePath, "*.wvd", (p) => setOpt({ widevinePath: p }))
                }
                onBrowseFolder={() =>
                  void pickDir("Choose the folder with your CDM files", o.widevinePath, (p) => setOpt({ widevinePath: p }))
                }
              />
            </Row>
            <Row label="FFmpeg" hint={env?.ffmpegPath ? `Using ${env.ffmpegPath}` : "Leave empty to find it on PATH."}>
              <PathInput
                value={draft.ffmpegPath}
                placeholder="Find automatically"
                onChange={(v) => set({ ffmpegPath: v })}
                onBrowse={() => void pickFile("Choose the ffmpeg executable", draft.ffmpegPath, "", (p) => set({ ffmpegPath: p }))}
              />
            </Row>
          </Section>

          <Section title="Rate limits" description="Crunchyroll limits how many streams an account can open, and how quickly.">
            <Row label="Simultaneous streams" hint="Keep this within your plan's stream limit.">
              <NumberStepper
                label="simultaneous streams"
                value={o.simultaneous}
                min={1}
                max={10}
                onChange={(v) => setOpt({ simultaneous: v })}
              />
            </Row>
            <Row label="Pause between episodes" hint="How long to wait before starting the next episode.">
              <NumberStepper
                label="pause between episodes"
                value={Math.round(o.downloadDelay / NS_PER_SECOND)}
                min={0}
                max={3600}
                suffix="seconds"
                onChange={(v) => setOpt({ downloadDelay: v * NS_PER_SECOND })}
              />
            </Row>
            <Row label="Log raw manifests" hint="Writes playback JSON and manifest XML to Logs, for bug reports.">
              <Switch checked={o.debugManifest} onCheckedChange={(v) => setOpt({ debugManifest: v })} />
            </Row>
          </Section>
        </div>
      </ScrollArea>
    </div>
  )
}

function Section({ title, description, children }: { title: string; description?: string; children: React.ReactNode }) {
  return (
    <section className="border-b py-8 last:border-b-0">
      <h2 className="font-heading text-lg font-semibold">{title}</h2>
      {description && <p className="mt-1 text-sm text-muted-foreground">{description}</p>}
      <div className="mt-5 flex flex-col gap-6">{children}</div>
    </section>
  )
}

function Row({ label, hint, children }: { label: string; hint?: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-[180px_1fr] gap-6">
      <div className="pt-1.5 text-sm font-medium">{label}</div>
      <div className="min-w-0">
        {children}
        {hint && <p className="mt-2 text-xs leading-relaxed text-muted-foreground">{hint}</p>}
      </div>
    </div>
  )
}

function ChoiceSelect({ value, options, onChange }: { value: string; options: string[]; onChange: (v: string) => void }) {
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
      <Input value={value} placeholder={placeholder} onChange={(e) => onChange(e.target.value)} spellCheck={false} />
      <Button variant="outline" onClick={onBrowse}>
        {onBrowseFolder ? "File…" : <><FolderIcon /> Browse</>}
      </Button>
      {onBrowseFolder && (
        <Button variant="outline" onClick={onBrowseFolder}>
          <FolderIcon /> Folder…
        </Button>
      )}
    </div>
  )
}
