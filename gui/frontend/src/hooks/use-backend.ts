import { createContext, useContext, useEffect, useMemo, useState } from "react"

import {
  api,
  onLog,
  onQueue,
  type Environment,
  type Job,
  type Language,
  type AccountStatus,
  type LogLine,
  type QueueState,
  type Settings,
} from "@/lib/api"

export type Backend = {
  ready: boolean
  env: Environment | null
  settings: Settings | null
  languages: Language[]
  languageName: (code: string) => string
  jobs: Job[]
  paused: boolean
  mixer: boolean
  accountStatus: AccountStatus[]
  waitUntil?: string
  logs: LogLine[]
  checking: boolean
  recheck: () => Promise<void>
  save: (s: Settings) => Promise<Environment>
  switchAccount: (id: string) => Promise<Environment>
}

export const BackendContext = createContext<Backend | null>(null)

export function useBackend() {
  const ctx = useContext(BackendContext)
  if (!ctx) throw new Error("useBackend outside BackendContext")
  return ctx
}

export function useBackendState(): Backend {
  const [env, setEnv] = useState<Environment | null>(null)
  const [settings, setSettings] = useState<Settings | null>(null)
  const [languages, setLanguages] = useState<Language[]>([])
  const [jobs, setJobs] = useState<Job[]>([])
  const [paused, setPaused] = useState(false)
  const [mixer, setMixer] = useState(false)
  const [accountStatus, setAccountStatus] = useState<AccountStatus[]>([])
  const [waitUntil, setWaitUntil] = useState<string | undefined>()
  const [logs, setLogs] = useState<LogLine[]>([])
  const [checking, setChecking] = useState(true)

  useEffect(() => {
    const applyQueue = (q: QueueState | null) => {
      setJobs(q?.jobs ?? [])
      setPaused(q?.paused ?? false)
      setMixer(q?.mixer ?? false)
      setAccountStatus(q?.accounts ?? [])
      setWaitUntil(q?.waitUntil ?? undefined)
    }
    const offQueue = onQueue(applyQueue)
    const offLog = onLog((line) =>
      setLogs((prev) => {
        const next = prev.length >= 2000 ? prev.slice(-1999) : prev.slice()
        next.push(line)
        return next
      })
    )
    void (async () => {
      const [s, langs, q, l] = await Promise.all([
        api.GetSettings(),
        api.Languages(),
        api.Queue(),
        api.Logs(),
      ])
      setSettings(s)
      setLanguages(langs ?? [])
      applyQueue(q as QueueState | null)
      setLogs(l ?? [])
      setEnv(await api.CheckEnvironment())
      setChecking(false)
    })()
    return () => {
      offQueue()
      offLog()
    }
  }, [])

  return useMemo<Backend>(() => {
    const names = new Map(languages.map((l) => [l.code, l.name]))
    return {
      ready: settings !== null,
      env,
      settings,
      languages,
      languageName: (code) => names.get(code) ?? code,
      jobs,
      paused,
      mixer,
      accountStatus,
      waitUntil,
      logs,
      checking,
      recheck: async () => {
        setChecking(true)
        try {
          setEnv(await api.CheckEnvironment())
        } finally {
          setChecking(false)
        }
      },
      save: async (s) => {
        const next = await api.SaveSettings(s)
        setSettings(s)
        setEnv(next)
        return next
      },
      switchAccount: async (id) => {
        setChecking(true)
        try {
          const next = await api.SwitchAccount(id)
          setSettings(await api.GetSettings())
          setEnv(next)
          return next
        } finally {
          setChecking(false)
        }
      },
    }
  }, [env, settings, languages, jobs, paused, mixer, accountStatus, waitUntil, logs, checking])
}
