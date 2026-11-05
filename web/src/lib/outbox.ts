// Офлайн-очередь ответов в IndexedDB. Каждый ответ получает client_event_id —
// сервер дедуплицирует повторы, так что отправлять можно сколько угодно раз.

import { api, isNetworkError, type AnswerBody, type AnswerResult, ApiError } from './api'

export interface OutboxItem {
  id: string // = client_event_id
  token: string
  createdAt: number
  answer?: AnswerBody
  photo?: { blob: Blob; level_id: string; bonus_id?: string; answer_idx?: number }
}

const DB = 'questline'
const STORE = 'outbox'

function open(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB, 1)
    req.onupgradeneeded = () => req.result.createObjectStore(STORE, { keyPath: 'id' })
    req.onsuccess = () => resolve(req.result)
    req.onerror = () => reject(req.error)
  })
}

async function tx<T>(mode: IDBTransactionMode, fn: (s: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  const db = await open()
  return new Promise((resolve, reject) => {
    const t = db.transaction(STORE, mode)
    const req = fn(t.objectStore(STORE))
    t.oncomplete = () => resolve(req.result)
    t.onerror = () => reject(t.error)
  })
}

export const outbox = {
  add: (item: OutboxItem) => tx('readwrite', (s) => s.put(item)),
  remove: (id: string) => tx('readwrite', (s) => s.delete(id)),
  all: async (token: string) => {
    const items = (await tx('readonly', (s) => s.getAll())) as OutboxItem[]
    return items.filter((i) => i.token === token).sort((a, b) => a.createdAt - b.createdAt)
  },
}

export function newId(): string {
  if (crypto.randomUUID) return crypto.randomUUID()
  return Array.from(crypto.getRandomValues(new Uint8Array(16)), (b) => b.toString(16).padStart(2, '0')).join('')
}

export function send(item: OutboxItem): Promise<AnswerResult> {
  if (item.answer) return api.answer(item.token, item.answer)
  const p = item.photo!
  const form = new FormData()
  form.set('client_event_id', item.id)
  form.set('level_id', p.level_id)
  if (p.bonus_id) form.set('bonus_id', p.bonus_id)
  if (p.answer_idx !== undefined) form.set('answer_idx', String(p.answer_idx))
  form.set('photo', p.blob, 'photo.jpg')
  return api.photo(item.token, form)
}

export type Delivery = { item: OutboxItem; result?: AnswerResult; error?: ApiError }

/**
 * Отправляет накопленное по порядку. Останавливается на первой сетевой ошибке
 * (связи всё ещё нет). Ответ сервера — любой, в том числе ошибка, — финальный:
 * элемент удаляется из очереди.
 */
export async function flush(token: string, onDelivered: (d: Delivery) => void): Promise<number> {
  const items = await outbox.all(token)
  for (const item of items) {
    try {
      const result = await send(item)
      await outbox.remove(item.id)
      onDelivered({ item, result })
    } catch (e) {
      if (isNetworkError(e)) break
      await outbox.remove(item.id)
      onDelivered({ item, error: e as ApiError })
    }
  }
  return (await outbox.all(token)).length
}
