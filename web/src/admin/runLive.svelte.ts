// Живое состояние игры для ведущего: дашборд + очередь модерации, SSE.

import { ApiError } from '../lib/api'
import type { AdminApi, Dashboard, ModerationItem, Run } from './adminApi'

export type Note = { id: number; kind: 'ok' | 'bad' | 'info'; text: string }

export class RunLive {
  run = $state<Run | null>(null)
  dash = $state<Dashboard | null>(null)
  queue = $state<ModerationItem[]>([])
  error = $state<string | null>(null)
  streaming = $state(false)
  now = $state(Date.now())
  notes = $state<Note[]>([])

  private es: EventSource | null = null
  private timer: ReturnType<typeof setTimeout> | null = null
  private noteSeq = 0

  constructor(
    public api: AdminApi,
    public runId: string,
  ) {}

  start(): () => void {
    const tick = setInterval(() => (this.now = Date.now()), 1000)
    const poll = setInterval(() => this.refresh(), 30_000)
    this.api.run(this.runId).then((r) => (this.run = r)).catch((e) => (this.error = (e as Error).message))
    this.refresh()
    this.es = new EventSource(`/admin/v1/runs/${this.runId}/stream?token=${encodeURIComponent(this.api.token)}`)
    this.es.onopen = () => (this.streaming = true)
    this.es.onerror = () => (this.streaming = false)
    this.es.addEventListener('state_changed', (e) => {
      if (Number((e as MessageEvent).data) !== this.dash?.seq) this.schedule()
    })
    return () => {
      clearInterval(tick)
      clearInterval(poll)
      this.es?.close()
      if (this.timer) clearTimeout(this.timer)
    }
  }

  private schedule() {
    if (this.timer) return
    this.timer = setTimeout(() => {
      this.timer = null
      this.refresh()
    }, 200)
  }

  async refresh() {
    try {
      const [d, q] = await Promise.all([this.api.dashboard(this.runId), this.api.moderation(this.runId)])
      this.dash = d
      this.queue = q
      this.error = null
    } catch (e) {
      this.error = e instanceof ApiError ? e.message : 'Нет связи с сервером'
    }
  }

  note(kind: Note['kind'], text: string) {
    const id = ++this.noteSeq
    this.notes = [...this.notes.slice(-2), { id, kind, text }]
    setTimeout(() => (this.notes = this.notes.filter((n) => n.id !== id)), 4000)
  }

  /** Выполнить действие; ошибку показать, состояние обновить. */
  async act<T>(fn: () => Promise<T>, ok?: string): Promise<T | null> {
    try {
      const r = await fn()
      if (ok) this.note('ok', ok)
      this.refresh()
      return r
    } catch (e) {
      this.note('bad', e instanceof ApiError ? e.message : 'Нет связи с сервером')
      return null
    }
  }
}
