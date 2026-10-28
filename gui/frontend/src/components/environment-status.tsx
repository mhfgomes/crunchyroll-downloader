import {
  CheckCircle2Icon,
  Loader2Icon,
  RefreshCwIcon,
  ShuffleIcon,
  XCircleIcon,
} from "lucide-react"

import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { useBackend } from "@/hooks/use-backend"
import { errorMessage } from "@/lib/api"

function Row({
  ok,
  label,
  detail,
}: {
  ok: boolean
  label: string
  detail?: string
}) {
  return (
    <div className="flex items-center gap-2 text-xs" title={detail}>
      {ok ? (
        <CheckCircle2Icon className="size-3.5 text-emerald-400" />
      ) : (
        <XCircleIcon className="size-3.5 text-destructive" />
      )}
      <span className={ok ? "text-muted-foreground" : "text-foreground"}>
        {label}
      </span>
    </div>
  )
}

export function EnvironmentStatus({
  onOpenSettings,
}: {
  onOpenSettings: () => void
}) {
  const { env, settings, checking, recheck, switchAccount, save, locked } =
    useBackend()
  const accounts = settings?.accounts ?? []
  const items = accounts.map((a) => ({
    value: a.id,
    label: a.name || "Unnamed account",
  }))

  async function pick(id: string) {
    try {
      const next = await switchAccount(id)
      if (next.loginError) toast.error(next.loginError)
    } catch (err) {
      toast.error(errorMessage(err))
    }
  }
  // The mixer needs two accounts with a cookie that are in it.
  const mixable = accounts.filter((a) => a.etpRt && !a.noMixer).length > 1

  async function toggleMixer(on: boolean) {
    if (!settings) return
    try {
      await save({ ...settings, mixer: on })
      toast.success(
        on ? "Mixer on: episodes are spread over your accounts" : "Mixer off"
      )
    } catch (err) {
      toast.error(errorMessage(err))
    }
  }
  const allGood = env && env.loggedIn && env.ffmpegPath && env.widevine

  return (
    <div className="rounded-lg border bg-card/50 p-3">
      <div className="mb-2 flex items-center justify-between">
        <span className="text-xs font-medium">Status</span>
        <Button
          variant="ghost"
          size="icon-xs"
          onClick={() => void recheck()}
          disabled={checking}
          aria-label="Check again"
        >
          {checking ? (
            <Loader2Icon className="animate-spin" />
          ) : (
            <RefreshCwIcon />
          )}
        </Button>
      </div>
      {accounts.length > 1 && (
        <Select
          value={settings?.activeAccount ?? ""}
          items={items}
          onValueChange={(v) => v && void pick(v as string)}
          disabled={checking || locked}
        >
          <SelectTrigger
            size="sm"
            className="mb-2 w-full"
            aria-label="Active account"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {items.map((i) => (
              <SelectItem key={i.value} value={i.value}>
                {i.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
      {mixable && (
        <label
          className="mb-2 flex cursor-pointer items-center gap-2 text-xs"
          title={
            locked
              ? "Pause the queue to change this"
              : "Spread episodes over every account in the mixer"
          }
        >
          <ShuffleIcon
            className={
              settings?.mixer
                ? "size-3.5 text-primary"
                : "size-3.5 text-muted-foreground"
            }
          />
          <span
            className={
              settings?.mixer ? "text-foreground" : "text-muted-foreground"
            }
          >
            Account mixer
          </span>
          <Switch
            size="sm"
            className="ml-auto"
            checked={!!settings?.mixer}
            disabled={locked}
            onCheckedChange={(v) => void toggleMixer(v)}
          />
        </label>
      )}
      {env ? (
        <div className="flex flex-col gap-1.5">
          <Row
            ok={env.loggedIn}
            label={
              env.loggedIn && env.account && accounts.length < 2
                ? `Signed in as ${env.account}`
                : "Signed in"
            }
            detail={env.loginError}
          />
          <Row
            ok={!!env.ffmpegPath}
            label="FFmpeg"
            detail={env.ffmpegPath || env.ffmpegError}
          />
          <Row ok={env.widevine} label="Widevine CDM" />
        </div>
      ) : (
        <div className="text-xs text-muted-foreground">Checking…</div>
      )}
      {env && !allGood && !checking && (
        <Button
          variant="link"
          size="xs"
          className="mt-1 h-auto px-0"
          onClick={onOpenSettings}
        >
          Fix in Settings
        </Button>
      )}
    </div>
  )
}
