// API ведущего. Зеркало DTO из internal/httpapi (admin.go, views.go).

import { ApiError, type ChatMsg, type Photo, type Player, type ScoreItem, type Sos, type Standing } from '../lib/api'

export type { ChatMsg, Photo, Sos }

export interface Asset {
  name: string
  key: string
  size: number
  at: string
  url: string
}

export interface ScenarioVersion {
  id: string
  slug: string
  version: number
  title?: string
  created_at: string
  source?: string
  host_notes?: string // заметки организаторам — только в админке
  level_notes?: { level_id: string; title: string; notes: string }[]
}

export interface TrashedRun {
  id: string
  title: string
  public_code: string
  created_at: string
  deleted_at: string
  purge_at: string
  teams: number
  started: boolean
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
  skipped?: boolean
  location?: boolean
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
  bonus_sec: number
  elapsed_sec: number
  levels_done: number
  last_activity?: string
  score: ScoreItem[]
  chat: ChatMsg[]
  current: LevelProgress[]
  levels: LevelProgress[]
  /** Коды переноса в VK-бот: id игрока → QA-XXXX-XXXX. */
  transfer_codes?: Record<string, string>
}

export interface Dashboard {
  seq: number
  run_status: 'pending' | 'running' | 'finished'
  levels_total: number
  registration: { open: boolean; max_teams: number }
  /** Витрина: город и видимость в «Доступных играх» VK-бота. */
  listing: { city: string; listed: boolean }
  leaderboard: { visibility: string; override: boolean | null; frozen: boolean }
  pending_moderation: number
  scoring_rule: string
  scoring_mode: 'time' | 'points' | 'mixed'
  host_contact: string
  sos: Sos[]
  results?: { at: string; message?: string; standings: Standing[] }
  teams: DashTeam[]
  withdrawn?: { id: string; name: string; players: Player[] }[]
  standings: Standing[]
  messages: { at: string; text: string; media_url?: string }[]
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
    if (body instanceof FormData) {
      payload = body
    } else if (raw !== undefined) {
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
  deleteRun = (id: string) => this.req<void>('DELETE', `/runs/${id}`)
  restoreRun = (id: string) => this.req<Run>('POST', `/runs/${id}/restore`)
  trash = () => this.req<TrashedRun[]>('GET', '/trash')
  purgeRun = (id: string) => this.req<void>('DELETE', `/trash/${id}`)
  run = (id: string) => this.req<Run>('GET', `/runs/${id}`)
  createRun = (scenario_version_id: string, title: string) => this.req<Run>('POST', '/runs', { scenario_version_id, title })
  scenarios = () => this.req<ScenarioVersion[]>('GET', '/scenarios')
  scenario = (id: string) => this.req<ScenarioVersion>('GET', `/scenarios/${id}`)
  uploadScenario = (yaml: string) => this.req<ScenarioVersion>('POST', '/scenarios', undefined, yaml)
  /** Проверка без сохранения (конструктор). */
  validateScenario = (yaml: string) =>
    this.req<{ ok: boolean; id: string; title: string; levels: number; next_version: number }>('POST', '/scenarios/validate', undefined, yaml)
  /** Маршруты команд, как их построит игра при старте (для random — пример). */
  previewRoutes = (yaml: string, teams: number) =>
    this.req<{ mode: string; routes?: string[][] }>('POST', `/scenarios/routes?teams=${teams}`, undefined, yaml)
  changeScenario = (run: string, scenario_version_id: string) => this.req<Run>('PUT', `/runs/${run}/scenario`, { scenario_version_id })

  dashboard = (run: string) => this.req<Dashboard>('GET', `/runs/${run}/dashboard`)
  start = (run: string) => this.req<Dashboard>('POST', `/runs/${run}/start`)
  finish = (run: string) => this.req<Dashboard>('POST', `/runs/${run}/finish`)
  approveResults = (run: string, message: string) => this.req<Dashboard>('POST', `/runs/${run}/results`, { message })
  setRegistration = (run: string, open: boolean, max_teams: number) =>
    this.req<Dashboard>('PUT', `/runs/${run}/registration`, { open, max_teams })
  setListing = (run: string, city: string, listed: boolean) =>
    this.req<Dashboard>('PUT', `/runs/${run}/listing`, { city, listed })
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
  teamChat = (run: string, team: string, text: string, media_key?: string) =>
    this.req<Dashboard>('POST', `/runs/${run}/teams/${team}/chat`, { text, media_key })
  resolveSOS = (run: string, seq: number, note: string) => this.req<Dashboard>('POST', `/runs/${run}/sos/${seq}/resolve`, { note })
  setHostContact = (run: string, text: string) => this.req<Dashboard>('PUT', `/runs/${run}/contact`, { text })
  broadcast = (run: string, text: string, media_key?: string) =>
    this.req<Dashboard>('POST', `/runs/${run}/broadcast`, { text, media_key })
  /** Фото к сообщению: сначала загрузить, потом отправить сообщение с ключом. */
  upload = (run: string, photo: Blob) => {
    const f = new FormData()
    f.append('photo', photo, 'photo.jpg')
    return this.req<{ key: string; url: string }>('POST', `/runs/${run}/uploads`, f)
  }
  photos = (run: string) => this.req<Photo[]>('GET', `/runs/${run}/photos`)
  photosZip = (run: string, all: boolean) =>
    `/admin/v1/runs/${run}/photos.zip?token=${encodeURIComponent(this.token)}${all ? '&all=1' : ''}`
  assets = () => this.req<Asset[]>('GET', '/assets')
  uploadAssets = (files: { name: string; blob: Blob }[]) => {
    const f = new FormData()
    files.forEach((x) => f.append('file', x.blob, x.name))
    return this.req<Asset[]>('POST', '/assets', f)
  }
  deleteAsset = (name: string) => this.req<void>('DELETE', `/assets/${encodeURIComponent(name)}`)
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
