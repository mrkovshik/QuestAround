// Типы и вызовы API игрока. Зеркало DTO из internal/httpapi/views.go.

export interface AnswerSlot {
  index: number
  type: 'code' | 'geo' | 'photo' | 'text'
  options?: string[]
  solved: boolean
  pending: boolean
}

export interface Hint {
  index: number
  opened: boolean
  text?: string
  on_request: boolean
  penalty_sec?: number
  penalty_points?: number
  opens_at?: string
  available_at?: string
  media?: string[]
}

export interface Bonus {
  id: string
  title: string
  type: 'code' | 'geo' | 'photo' | 'text'
  points: number
  done: boolean
  pending: boolean
  media?: string[]
  time_sec?: number
}

export interface Penalty {
  penalty_sec?: number
  penalty_points?: number
  available_at?: string
}

export interface LevelLocation extends Penalty {
  revealed: boolean
  text?: string
  lat?: number
  lng?: number
}

export interface Level {
  id: string
  title: string
  task: string
  media?: string[]
  points: number
  open: boolean
  opened_at: string
  completed_at?: string
  required: number
  solved: number
  answers: AnswerSlot[]
  hints: Hint[]
  bonuses?: Bonus[]
  location?: LevelLocation
  skip?: Penalty
  skipped?: boolean
}

export interface Player {
  id: string
  name: string
  captain: boolean
}

export interface Team {
  id: string
  name: string
  status: 'registered' | 'scheduled' | 'playing' | 'finished' | 'withdrawn'
  invite_code?: string
  players: Player[]
  captain_id?: string
}

export interface Message {
  at: string
  text: string
  media_url?: string
}

export interface Photo {
  seq: number
  at: string
  team_id?: string
  team_name?: string
  player_name?: string
  level_id: string
  title: string
  status: 'pending' | 'accepted' | 'rejected'
  comment?: string
  url: string
}

export interface ScoreItem {
  at: string
  kind: 'level' | 'bonus' | 'hint' | 'adjust'
  title: string
  points: number
  time_sec: number
  reason?: string
}

export interface Results {
  approved: boolean
  at: string
  message?: string
  rank?: number
  teams: number
  points: number
  elapsed_sec: number
}

export interface ChatMsg {
  seq: number
  at: string
  from_host: boolean
  author: string
  text: string
  media_url?: string
}

export interface Sos {
  seq: number
  at: string
  team_id: string
  team_name: string
  player_name: string
  contact?: string
  lat?: number
  lng?: number
  accuracy?: number
  text?: string
  resolved_at?: string
  note?: string
  current_level?: string
}

export interface PlayerState {
  seq: number
  run_status: 'pending' | 'running' | 'finished'
  title: string
  team: Team
  player_id: string
  is_captain: boolean
  start_at?: string
  started_at?: string
  finished_at?: string
  time_limit_at?: string
  points: number
  penalty_sec: number
  bonus_sec: number
  elapsed_sec: number
  clean_sec: number
  levels_done: number
  levels_total: number
  levels: Level[]
  messages: Message[]
  hint_requests: 'captain' | 'anyone'
  score: ScoreItem[]
  scoring_rule: string
  scoring_mode: ScoringMode
  results?: Results
  chat: ChatMsg[]
  sos: Sos[]
  host_contact?: string
  photos: Photo[]
  leaderboard: { visible: boolean; frozen: boolean; final: boolean }
  /** Диалог с сообществом VK-бота — запасной канал, если сайт недоступен. */
  vk_url?: string
}

export interface GameInfo {
  title: string
  description?: string
  status: 'pending' | 'running' | 'finished'
  participants: { mode: 'team' | 'solo'; min_size: number; max_size: number }
  registration: { open: boolean; max_teams: number; teams: number }
}

export interface Credentials {
  token: string
  player_id: string
  team_id: string
  invite_code?: string
}

export type Result = 'correct' | 'wrong' | 'duplicate' | 'pending' | 'inaccurate'

export interface AnswerResult {
  seq: number
  result: Result
  level_id?: string
  answer_idx?: number
  bonus_id?: string
}

export interface Standing {
  rank: number
  team_id: string
  name: string
  status: string
  points: number
  levels: number
  elapsed_sec: number
  penalty_sec?: number
  bonus_sec?: number
  finished: boolean
}

export type ScoringMode = 'time' | 'points' | 'mixed'

export interface AnswerBody {
  client_event_id: string
  kind: 'code' | 'geo' | 'text'
  level_id?: string
  value?: string
  lat?: number
  lng?: number
  accuracy?: number
  answer_idx?: number
  bonus_id?: string
}

/** Ошибка API: есть HTTP-ответ. Сетевые ошибки — обычный TypeError от fetch. */
export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message)
  }
}

export function isNetworkError(e: unknown): boolean {
  return !(e instanceof ApiError)
}

async function request<T>(
  method: string,
  path: string,
  opts: { token?: string; body?: unknown; form?: FormData; timeoutMs?: number } = {},
): Promise<T> {
  const headers: Record<string, string> = {}
  if (opts.token) headers.Authorization = `Bearer ${opts.token}`
  let body: BodyInit | undefined
  if (opts.form) {
    body = opts.form
  } else if (opts.body !== undefined) {
    headers['Content-Type'] = 'application/json'
    body = JSON.stringify(opts.body)
  }
  // Таймаут: при «белых списках» запрос может не падать, а висеть минутами.
  const ctl = opts.timeoutMs ? new AbortController() : null
  const timer = ctl ? setTimeout(() => ctl.abort(), opts.timeoutMs) : undefined
  let res: Response
  let data: any
  try {
    res = await fetch(path, { method, headers, body, signal: ctl?.signal })
    if (res.status === 204) return undefined as T
    data = await res.json().catch(() => ({}))
  } finally {
    clearTimeout(timer)
  }
  if (!res.ok) {
    throw new ApiError(res.status, data.error ?? 'http_' + res.status, data.message ?? `Ошибка ${res.status}`)
  }
  return data as T
}

export const api = {
  gameInfo: (code: string) => request<GameInfo>('GET', `/api/v1/games/${encodeURIComponent(code)}`),
  createTeam: (code: string, team_name: string, captain_name: string, contact: string) =>
    request<Credentials>('POST', `/api/v1/games/${encodeURIComponent(code)}/teams`, { body: { team_name, captain_name, contact } }),
  join: (code: string, invite_code: string, name: string) =>
    request<Credentials>('POST', `/api/v1/games/${encodeURIComponent(code)}/join`, { body: { invite_code, name } }),

  state: (token: string) => request<PlayerState>('GET', '/api/v1/state', { token, timeoutMs: 15_000 }),
  answer: (token: string, body: AnswerBody) => request<AnswerResult>('POST', '/api/v1/answers', { token, body }),
  photo: (token: string, form: FormData) => request<AnswerResult>('POST', '/api/v1/photos', { token, form }),
  revealLocation: (token: string, levelId: string) =>
    request<void>('POST', `/api/v1/levels/${encodeURIComponent(levelId)}/location`, { token }),
  skipLevel: (token: string, levelId: string) =>
    request<void>('POST', `/api/v1/levels/${encodeURIComponent(levelId)}/skip`, { token }),
  hint: (token: string, levelId: string, idx: number) =>
    request<void>('POST', `/api/v1/levels/${encodeURIComponent(levelId)}/hints/${idx}`, { token }),
  leaderboard: (token: string) =>
    request<{ frozen: boolean; final: boolean; rule: string; mode: ScoringMode; standings: Standing[] }>('GET', '/api/v1/leaderboard', { token }),

  chat: (token: string, text: string, media_key?: string) =>
    request<void>('POST', '/api/v1/chat', { token, body: { text, media_key } }),
  upload: (token: string, form: FormData) => request<{ key: string; url: string }>('POST', '/api/v1/uploads', { token, form }),
  sos: (token: string, body: { lat?: number; lng?: number; accuracy?: number; text: string }) =>
    request<{ seq: number }>('POST', '/api/v1/sos', { token, body }),

  renameTeam: (token: string, name: string) => request<Team>('PATCH', '/api/v1/team', { token, body: { name } }),
  removePlayer: (token: string, id: string) => request<Team>('DELETE', `/api/v1/team/players/${encodeURIComponent(id)}`, { token }),
  rotateInvite: (token: string) => request<Team>('POST', '/api/v1/team/invite/rotate', { token }),
  transferCaptain: (token: string, player_id: string) => request<Team>('POST', '/api/v1/team/captain', { token, body: { player_id } }),
  withdraw: (token: string) => request<Team>('POST', '/api/v1/team/withdraw', { token }),
}
