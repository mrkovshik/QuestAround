// API ведущего. Зеркало DTO из internal/httpapi (admin.go, views.go).

import { ApiError, type Player, type Standing } from '../lib/api'

export interface ScenarioVersion {
  id: string
  slug: string
  version: number
  title?: string
  created_at: string
  source?: string
}

export interface Run {
  id: string
  title: string
  public_code: string
  scenario_version_id: string
  created_at: string
  status?: 'pending' | 'running' | 'finished'
  teams: number
}

export interface LevelProgress {
  level_id: string
  title: string
  opened_at: string
  completed_at?: string
  solved: number
  required: number
  hints_opened: number
  hints_total: number
}

export interface DashTeam {
  id: string
  name: string
  status: 'registered' | 'scheduled' | 'playing' | 'finished' | 'withdrawn'
  invite_code?: string
  players: Player[]
  captain_id?: string
  contact?: string
  slot: number
  scheduled_at?: string
  started_at?: string
  finished_at?: string
  finish_reason?: string
  route?: string[]
  points: number
  penalty_sec: number
  elapsed_sec: number
  levels_done: number
  last_activity?: string
  current: LevelProgress[]
  levels: LevelProgress[]
}

export interface Dashboard {
  seq: number
  run_status: 'pending' | 'running' | 'finished'
  levels_total: number
  registration: { open: boolean; max_teams: number }
  leaderboard: { visibility: string; override: boolean | null; frozen: boolean }
  pending_moderation: number
  teams: DashTeam[]
  withdrawn?: { id: string; name: string; players: Player[] }[]
  standings: Standing[]
  messages: { at: string; text: string }[]
}

export interface ModerationItem {
  seq: number
  at: string
  team_id: string
  team_name: string
  player_name?: string
  level_id: string
  level_title: string
  target: string
  kind: 'photo' | 'text'
  value?: string
  media_url?: string
}

export interface JournalEvent {
  seq: number
  type: string
  at: string
  team_id?: string
  player_id?: string
  client_event_id?: string
  payload: Record<string, unknown>
}

export interface ScenarioIssue {
  line?: number
  path?: string
  message: string
}

/** Ошибка загрузки сценария — со списком проблем по строкам. */
export class ScenarioError extends ApiError {
  constructor(public details: ScenarioIssue[]) {
    super(422, 'invalid_scenario', 'В сценарии есть ошибки')
  }
}

/** Во время игры новая версия меняет структуру — с перечнем отличий. */
export class ScenarioConflict extends ApiError {
  constructor(
    message: string,
    public details: ScenarioIssue[],
  ) {
    super(409, 'scenario_incompatible', message)
  }
}

export type OverrideAction = 'complete_level' | 'open_hint' | 'adjust_time' | 'adjust_points' | 'finish_team'

export class AdminApi {
  constructor(public token: string) {}

  private async req<T>(method: string, path: string, body?: unknown, raw?: string): Promise<T> {
    const headers: Record<string, string> = { Authorization: `Bearer ${this.token}` }
    let payload: BodyInit | undefined
    if (raw !== undefined) {
      headers['Content-Type'] = 'application/yaml'
      payload = raw
    } else if (body !== undefined) {
      headers['Content-Type'] = 'application/json'
      payload = JSON.stringify(body)
    }
    const res = await fetch('/admin/v1' + path, { method, headers, body: payload })
    if (res.status === 204) return undefined as T
    const data = await res.json().catch(() => ({}))
    if (!res.ok) {
      if (data.error === 'invalid_scenario') throw new ScenarioError(data.details ?? [])
      if (data.error === 'scenario_incompatible') throw new ScenarioConflict(data.message, data.details ?? [])
      throw new ApiError(res.status, data.error ?? 'http_' + res.status, data.message ?? `Ошибка ${res.status}`)
    }
    return data as T
  }

  runs = () => this.req<Run[]>('GET', '/runs')
  run = (id: string) => this.req<Run>('GET', `/runs/${id}`)
  createRun = (scenario_version_id: string, title: string) => this.req<Run>('POST', '/runs', { scenario_version_id, title })
  scenarios = () => this.req<ScenarioVersion[]>('GET', '/scenarios')
  scenario = (id: string) => this.req<ScenarioVersion>('GET', `/scenarios/${id}`)
  uploadScenario = (yaml: string) => this.req<ScenarioVersion>('POST', '/scenarios', undefined, yaml)
  changeScenario = (run: string, scenario_version_id: string) => this.req<Run>('PUT', `/runs/${run}/scenario`, { scenario_version_id })

  dashboard = (run: string) => this.req<Dashboard>('GET', `/runs/${run}/dashboard`)
  start = (run: string) => this.req<Dashboard>('POST', `/runs/${run}/start`)
  finish = (run: string) => this.req<Dashboard>('POST', `/runs/${run}/finish`)
  setRegistration = (run: string, open: boolean, max_teams: number) =>
    this.req<Dashboard>('PUT', `/runs/${run}/registration`, { open, max_teams })
  createTeam = (run: string, name: string, contact: string) =>
    this.req<{ team_id: string; invite_code: string }>('POST', `/runs/${run}/teams`, { name, contact })
  renameTeam = (run: string, team: string, name: string) => this.req<Dashboard>('PATCH', `/runs/${run}/teams/${team}`, { name })
  withdrawTeam = (run: string, team: string) => this.req<Dashboard>('POST', `/runs/${run}/teams/${team}/withdraw`)
  startTeam = (run: string, team: string) => this.req<Dashboard>('POST', `/runs/${run}/teams/${team}/start`)
  override = (
    run: string,
    team: string,
    body: { action: OverrideAction; level_id?: string; hint_idx?: number; time?: string; points?: number; reason: string },
  ) => this.req<Dashboard>('POST', `/runs/${run}/teams/${team}/override`, body)
  answerForTeam = (run: string, team: string, value: string) =>
    this.req<{ result: string }>('POST', `/runs/${run}/teams/${team}/answers`, { kind: 'code', value })
  moderation = (run: string) => this.req<ModerationItem[]>('GET', `/runs/${run}/moderation`)
  moderate = (run: string, seq: number, approved: boolean, comment = '') =>
    this.req<ModerationItem[]>('POST', `/runs/${run}/moderation/${seq}`, { approved, comment })
  setLeaderboard = (run: string, visible: boolean | null) => this.req<Dashboard>('PUT', `/runs/${run}/leaderboard`, { visible })
  broadcast = (run: string, text: string) => this.req<Dashboard>('POST', `/runs/${run}/broadcast`, { text })
  events = (run: string) => this.req<JournalEvent[]>('GET', `/runs/${run}/events`)

  /** Фото с модерации: <img> не умеет заголовок Authorization — берём blob. */
  async mediaURL(path: string): Promise<string> {
    const res = await fetch(path, { headers: { Authorization: `Bearer ${this.token}` } })
    if (!res.ok) throw new ApiError(res.status, 'media', 'Не удалось загрузить фото')
    return URL.createObjectURL(await res.blob())
  }
}

const KEY = 'qa:admin'
export const adminToken = {
  get: () => localStorage.getItem(KEY),
  set: (t: string) => localStorage.setItem(KEY, t),
  clear: () => localStorage.removeItem(KEY),
}
