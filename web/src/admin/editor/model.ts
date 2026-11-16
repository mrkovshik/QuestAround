// Модель конструктора: черновик повторяет структуру YAML-сценария
// (те же ключи, snake_case). Неизвестные конструктору поля проходят как есть,
// поэтому YAML → конструктор → YAML теряет только комментарии и форматирование.
// Проверяет сценарий сервер (тот же валидатор, что при загрузке файла).

import { Document, isScalar, parse, visit } from 'yaml'

export type AnswerType = 'code' | 'regex' | 'choice' | 'geo' | 'photo' | 'text'
export type ScoringMode = 'time' | 'points' | 'mixed'
export type FlowMode = 'linear' | 'storm' | 'graph'

export interface Answer {
  type: AnswerType
  values?: string[]
  pattern?: string
  options?: string[]
  correct?: string
  point?: number[]
  radius_m?: number | null
  max_accuracy_m?: number | null
  [k: string]: unknown
}

export interface Penalty {
  time?: string
  points?: number | null
}

export interface Hint {
  after?: string
  on_request?: boolean
  penalty?: Penalty
  text?: string
  media?: string[]
  [k: string]: unknown
}

export interface Bonus {
  id: string
  title?: string
  answer: Answer
  points?: number | null
  time?: string
  media?: string[]
  [k: string]: unknown
}

export interface Unlock {
  all_of?: string[]
  any_of?: string[]
  count?: number | null
  of?: string[]
}

export interface Level {
  id: string
  title?: string
  task?: string
  media?: string[]
  points?: number | null
  pin?: '' | 'first' | 'last'
  chain?: string
  require?: 'all' | 'any' | number | string
  answers: Answer[]
  hints?: Hint[]
  bonuses?: Bonus[]
  unlock?: Unlock
  [k: string]: unknown
}

export interface Draft {
  version: number
  id: string
  title: string
  description?: string
  participants: { mode?: 'team' | 'solo'; team_size?: { min?: number | null; max?: number | null }; hint_requests?: string; [k: string]: unknown }
  timing: { mode?: string; time_limit?: string; [k: string]: unknown }
  start: { mode?: string; interval?: string; order?: string; [k: string]: unknown }
  flow: { mode?: FlowMode; routes?: string; route_list?: string[][]; [k: string]: unknown }
  scoring: { mode?: ScoringMode; [k: string]: unknown }
  leaderboard: { visibility?: string; freeze_before_end?: string; columns?: string[]; [k: string]: unknown }
  defaults: {
    hint_after?: string[]
    rate_limit?: { wrong_answers?: number | null; per?: string }
    answer_normalize?: string[]
    [k: string]: unknown
  }
  levels: Level[]
  [k: string]: unknown
}

// Значения по умолчанию — как в internal/scenario/parse.go (applyDefaults).
export const DEFAULTS = {
  scoring: 'points' as ScoringMode,
  flow: 'linear' as FlowMode,
  routes: 'same',
  start: 'simultaneous',
  order: 'registration',
  visibility: 'always',
  columns: ['rank', 'team', 'points', 'levels', 'time'],
  normalize: ['trim', 'lower', 'yo_to_e'],
}

export const COLUMNS: [string, string][] = [
  ['rank', 'Место'],
  ['team', 'Команда'],
  ['points', 'Баллы'],
  ['levels', 'Уровни'],
  ['time', 'Время'],
]

export const NORMALIZE: [string, string][] = [
  ['trim', 'обрезать пробелы по краям'],
  ['lower', 'без учёта регистра'],
  ['yo_to_e', 'ё = е'],
  ['collapse_spaces', 'несколько пробелов = один'],
]

export const ANSWER_TYPES: [AnswerType, string][] = [
  ['code', 'Код / слово'],
  ['regex', 'Шаблон (regex)'],
  ['choice', 'Выбор варианта'],
  ['geo', 'Геоточка'],
  ['photo', 'Фото (модерация)'],
  ['text', 'Текст (модерация)'],
]

export const ID_RE = /^[a-z0-9][a-z0-9_-]*$/

/** Пустой сценарий для «Новый сценарий». */
export function emptyDraft(): Draft {
  return normalize({
    version: 1,
    id: '',
    title: '',
    participants: { mode: 'team', team_size: { min: 2, max: 5 } },
    timing: { time_limit: '3h' },
    start: { mode: 'simultaneous' },
    flow: { mode: 'linear', routes: 'same' },
    scoring: { mode: 'time' },
    leaderboard: { visibility: 'host' },
    defaults: { hint_after: ['15m', '30m'] },
    levels: [],
  } as unknown as Draft)
}

/** Гарантирует наличие вложенных объектов, чтобы формы могли к ним привязываться. */
export function normalize(d: Draft): Draft {
  d.version ??= 1
  d.id ??= ''
  d.title ??= ''
  for (const k of ['participants', 'timing', 'start', 'flow', 'scoring', 'leaderboard', 'defaults'] as const) {
    if (!d[k] || typeof d[k] !== 'object') (d as Record<string, unknown>)[k] = {}
  }
  d.participants.team_size ??= {}
  d.levels = Array.isArray(d.levels) ? d.levels : []
  for (const l of d.levels) normalizeLevel(l)
  return d
}

function normalizeLevel(l: Level) {
  l.id = l.id == null ? '' : String(l.id)
  l.answers = Array.isArray(l.answers) ? l.answers : []
  for (const b of l.bonuses ?? []) b.answer ??= { type: 'photo' }
  // after: 0 приходит из YAML числом — в форме длительности строкой
  for (const h of l.hints ?? []) if (typeof h.after === 'number') h.after = String(h.after)
}

/** YAML → черновик. Бросает Error с понятным текстом при синтаксической ошибке. */
export function fromYaml(src: string): Draft {
  const obj = parse(src, { prettyErrors: true })
  if (!obj || typeof obj !== 'object' || Array.isArray(obj)) throw new Error('ожидался YAML-объект сценария')
  return normalize(obj as Draft)
}

/** Черновик → YAML для сервера и экспорта. */
export function toYaml(d: Draft): string {
  const doc = new Document(clean(order(d)) ?? {})
  // Короткие списки и мапы из скаляров — в одну строку, как пишут руками:
  // point: [44.63, 39.13], values: ["1864"], team_size: {min: 2, max: 5}.
  const short = (x: unknown) => isScalar(x) && String(x.value).length <= 40 && !String(x.value).includes('\n')
  visit(doc, {
    Seq(_, n) {
      if (n.items.every(short)) n.flow = true
    },
    Map(_, n) {
      if (n.items.length <= 3 && n.items.every((x) => short(x.value))) n.flow = true
    },
  })
  return doc.toString({ lineWidth: 0, flowCollectionPadding: false })
}

// Порядок ключей верхнего уровня — как в документации.
const TOP = ['version', 'id', 'title', 'description', 'participants', 'timing', 'start', 'flow', 'scoring', 'leaderboard', 'defaults', 'levels']
const LEVEL = ['id', 'pin', 'chain', 'title', 'task', 'media', 'points', 'require', 'unlock', 'answers', 'hints', 'bonuses']

function order(d: Draft): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const k of TOP) if (k in d) out[k] = d[k]
  for (const k of Object.keys(d)) if (!(k in out)) out[k] = d[k]
  out.levels = d.levels.map((l) => {
    const o: Record<string, unknown> = {}
    for (const k of LEVEL) if (k in l) o[k] = l[k]
    for (const k of Object.keys(l)) if (!(k in o)) o[k] = l[k]
    return o
  })
  return out
}

// Длительности: «0» у них означает «не задано», кроме after у подсказки,
// где after: 0 — «автоматом не выдавать».
const DURATIONS = new Set(['after', 'time_limit', 'interval', 'freeze_before_end', 'per', 'time'])
const isZeroDuration = (v: string) => /^0+(\.0+)?[smh]?$/.test(v.trim())

// Списки строк, где пустые строки — это просто незаполненная строка в форме.
const STRING_LISTS = new Set(['values', 'options', 'media', 'hint_after', 'all_of', 'any_of', 'of'])

/**
 * Убирает пустое: '', null, undefined, false, 0 в необязательных числах, пустые объекты и списки.
 * Элементы массивов не выбрасываются (кроме пустых строк в списках строк) —
 * иначе пути из ошибок сервера (levels[2].answers[0]) разойдутся с формой.
 */
function clean(v: unknown, key = ''): unknown {
  if (v === null || v === undefined || v === '' || v === false) return undefined
  if (typeof v === 'number') return Number.isFinite(v) && (v !== 0 || key === 'version' || key === 'after') ? v : undefined
  if (typeof v === 'string') {
    if (DURATIONS.has(key) && isZeroDuration(v)) return key === 'after' ? 0 : undefined
    return v.trim() === '' ? undefined : v
  }
  if (Array.isArray(v)) {
    if (STRING_LISTS.has(key)) {
      const a = v.map((x) => (typeof x === 'string' ? x.trim() : x)).filter((x) => x !== '' && x != null)
      return a.length ? a : undefined
    }
    if (key === 'point') return v.length ? v : undefined
    if (key === 'route_list') {
      const a = v.map((r) => (Array.isArray(r) ? r.filter(Boolean) : r))
      return a.length ? a : undefined
    }
    const a = v.map((x) => clean(x) ?? (typeof x === 'object' ? {} : x))
    if (key === 'levels' || key === 'answers') return a // пустой список — ошибка, которую покажет сервер
    return a.length ? a : undefined
  }
  if (typeof v === 'object') {
    const o: Record<string, unknown> = {}
    for (const [k, x] of Object.entries(v as Record<string, unknown>)) {
      const c = clean(x, k)
      if (c !== undefined) o[k] = c
    }
    // у уровня и ответа обязательные ключи оставляем даже пустыми
    if ('answers' in (v as object) && !('answers' in o)) o.answers = []
    return Object.keys(o).length ? o : undefined
  }
  return v
}

// --- уровни ---

export function uniqueId(base: string, taken: Iterable<string>): string {
  const set = new Set(taken)
  if (base && !set.has(base)) return base
  const stem = base || 'level'
  for (let i = 2; ; i++) if (!set.has(`${stem}-${i}`)) return `${stem}-${i}`
}

export function newLevel(d: Draft): Level {
  const n = d.levels.length + 1
  return {
    id: uniqueId(`l${n}`, d.levels.map((l) => l.id)),
    title: `Уровень ${n}`,
    task: '',
    answers: [{ type: 'code', values: [''] }],
  }
}

export function newAnswer(type: AnswerType): Answer {
  switch (type) {
    case 'code':
      return { type, values: [''] }
    case 'regex':
      return { type, pattern: '' }
    case 'choice':
      return { type, options: ['', ''], correct: '' }
    case 'geo':
      return { type, point: [], radius_m: 30, max_accuracy_m: 50 }
    default:
      return { type }
  }
}

/** Смена типа ответа: убираем поля старого типа (сервер ругается на лишние). */
export function retype(a: Answer, type: AnswerType) {
  for (const k of ['values', 'pattern', 'options', 'correct', 'point', 'radius_m', 'max_accuracy_m']) delete a[k]
  Object.assign(a, newAnswer(type))
}

/** Копия уровня; snap — уже снятый снимок ($state.snapshot), не прокси. */
export function cloneLevel(d: Draft, snap: Level): Level {
  const c = structuredClone(snap)
  c.id = uniqueId(snap.id + '-copy', d.levels.map((x) => x.id))
  c.title = (snap.title ?? '') + ' (копия)'
  if (c.pin) delete c.pin
  const bonusIds = d.levels.flatMap((x) => (x.bonuses ?? []).map((b) => b.id))
  for (const b of c.bonuses ?? []) {
    b.id = uniqueId(b.id + '-copy', bonusIds)
    bonusIds.push(b.id)
  }
  return c
}

/** Переименование уровня: обновляем ссылки в unlock и route_list. */
export function renameLevel(d: Draft, from: string, to: string) {
  if (!from || from === to) return
  const sub = (a?: string[]) => a?.map((x) => (x === from ? to : x))
  for (const l of d.levels) {
    if (!l.unlock) continue
    l.unlock.all_of = sub(l.unlock.all_of)
    l.unlock.any_of = sub(l.unlock.any_of)
    l.unlock.of = sub(l.unlock.of)
  }
  if (d.flow.route_list) d.flow.route_list = d.flow.route_list.map((r) => sub(r) ?? [])
}

/** Удаление уровня: чистим ссылки на него. */
export function removeLevelRefs(d: Draft, id: string) {
  const drop = (a?: string[]) => a?.filter((x) => x !== id)
  for (const l of d.levels) {
    if (!l.unlock) continue
    l.unlock.all_of = drop(l.unlock.all_of)
    l.unlock.any_of = drop(l.unlock.any_of)
    l.unlock.of = drop(l.unlock.of)
  }
  if (d.flow.route_list) d.flow.route_list = d.flow.route_list.map((r) => drop(r) ?? [])
}

// --- длительности и прочее ---

/** «15» → «15m»: число без единиц считаем минутами. */
export function fixDuration(v: string | undefined): string | undefined {
  const s = (v ?? '').trim().replace(/\s+/g, '')
  if (/^\d+$/.test(s)) return isZeroDuration(s) ? '0' : s + 'm'
  return s.replace(/мин$/, 'm').replace(/ч$/, 'h').replace(/с$/, 's')
}

/** Человекочитаемо: 1h30m → «1 ч 30 мин». Пусто, если формат не распознан. */
export function humanDuration(v: string | undefined): string {
  const m = (v ?? '').match(/^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$/)
  if (!m || !v) return ''
  return [m[1] && `${m[1]} ч`, m[2] && `${m[2]} мин`, m[3] && `${m[3]} с`].filter(Boolean).join(' ')
}

const TR: Record<string, string> = {
  а: 'a', б: 'b', в: 'v', г: 'g', д: 'd', е: 'e', ё: 'e', ж: 'zh', з: 'z', и: 'i', й: 'y', к: 'k', л: 'l', м: 'm',
  н: 'n', о: 'o', п: 'p', р: 'r', с: 's', т: 't', у: 'u', ф: 'f', х: 'h', ц: 'c', ч: 'ch', ш: 'sh', щ: 'sch',
  ъ: '', ы: 'y', ь: '', э: 'e', ю: 'yu', я: 'ya', č: 'c', ć: 'c', š: 's', ž: 'z', đ: 'dj',
}

/** Название → id: латиница, цифры, дефис. */
export function slugify(s: string): string {
  return s
    .toLowerCase()
    .split('')
    .map((c) => TR[c] ?? c)
    .join('')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 40)
}

/** «44.6338, 39.1352» (как копирует Google/Яндекс Карты) → [lat, lng]. */
export function parsePoint(s: string): [number, number] | null {
  const m = s.trim().match(/^(-?\d+(?:[.,]\d+)?)\s*[,; ]\s*(-?\d+(?:[.,]\d+)?)$/)
  if (!m) return null
  const lat = Number(m[1].replace(',', '.'))
  const lng = Number(m[2].replace(',', '.'))
  return Math.abs(lat) <= 90 && Math.abs(lng) <= 180 ? [lat, lng] : null
}
