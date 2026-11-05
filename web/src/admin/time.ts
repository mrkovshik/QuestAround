import { clock } from '../lib/format'

export function hhmm(iso?: string): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })
}

export function since(iso: string | undefined, now: number): number {
  return iso ? Math.max(0, Math.floor((now - Date.parse(iso)) / 1000)) : 0
}

/** «3 мин назад», «только что». */
export function ago(iso: string | undefined, now: number): string {
  if (!iso) return '—'
  const s = since(iso, now)
  if (s < 60) return 'только что'
  const m = Math.floor(s / 60)
  if (m < 60) return `${m} мин назад`
  return `${Math.floor(m / 60)} ч ${m % 60} мин назад`
}

export function between(a?: string, b?: string): string {
  if (!a || !b) return '—'
  return clock((Date.parse(b) - Date.parse(a)) / 1000)
}

/** [подпись, класс бейджа] */
export const RUN_STATUS: Record<string, [string, string]> = {
  pending: ['Не начата', ''],
  running: ['Идёт', 'warn'],
  finished: ['Завершена', 'ok'],
}

export const TEAM_STATUS: Record<string, [string, string]> = {
  registered: ['Ждёт старта', ''],
  scheduled: ['По расписанию', ''],
  playing: ['Играет', 'warn'],
  finished: ['Финиш', 'ok'],
  withdrawn: ['Снята', 'bad'],
}

export const FINISH_REASON: Record<string, string> = {
  completed: 'прошли всё',
  time_limit: 'вышло время',
  admin: 'остановил ведущий',
}
