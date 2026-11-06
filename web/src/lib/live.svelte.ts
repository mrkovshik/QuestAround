// Живое состояние игрока: загрузка /state, SSE-уведомления, офлайн-очередь,
// отправка ответов и всплывающие сообщения.

import { api, ApiError, isNetworkError, type AnswerBody, type AnswerResult, type PlayerState, type Result } from './api'
import { compressImage } from './image'
import { flush, newId, outbox, send, type Delivery, type OutboxItem } from './outbox'

export type ToastKind = 'ok' | 'bad' | 'info'
export interface Toast {
  id: number
  kind: ToastKind
  text: string
}

export const RESULT: Record<Result, [ToastKind, string]> = {
  correct: ['ok', 'Верно!'],
  wrong: ['bad', 'Неверно'],
  duplicate: ['info', 'Это уже засчитано'],
  pending: ['info', 'Отправлено на проверку ведущему'],
  inaccurate: ['bad', 'GPS пока неточный — подождите полминуты на открытом месте и попробуйте ещё раз'],
}

export type PhotoTarget = { level_id: string; bonus_id?: string; answer_idx?: number }

export class Live {
  state = $state<PlayerState | null>(null)
  error = $state<string | null>(null)
  online = $state(typeof navigator === 'undefined' ? true : navigator.onLine)
  streaming = $state(false)
  queued = $state(0)
  busy = $state(false)
  toasts = $state<Toast[]>([])
  /** Текущее время, тикает раз в секунду — для таймеров. */
  now = $state(Date.now())
  /** Токен больше не действует (игрока удалили) — надо войти заново. */
  unauthorized = $state(false)

  /** Момент, до которого игрок прочитал сообщения ведущего (мс). */
  lastRead = $state(0)

  private fetchedAt = 0
  private es: EventSource | null = null
  private refreshTimer: ReturnType<typeof setTimeout> | null = null
  private toastSeq = 0

  constructor(
    public code: string,
    public token: string,
  ) {
    try {
      this.lastRead = Number(localStorage.getItem(`qa:read:${code}`) ?? 0)
    } catch {
      /* без localStorage — всё будет «непрочитанным» до первого «Прочитано» */
    }
  }

  /** Непрочитанные сообщения ведущего: общие и в канал команды, старые сверху. */
  unread(): { at: string; text: string; personal: boolean }[] {
    const s = this.state
    if (!s) return []
    const all = [
      ...s.messages.map((m) => ({ at: m.at, text: m.text, personal: false })),
      ...s.chat.filter((m) => m.from_host).map((m) => ({ at: m.at, text: m.text, personal: true })),
    ]
    return all.filter((m) => Date.parse(m.at) > this.lastRead).sort((a, b) => Date.parse(a.at) - Date.parse(b.at))
  }

  markRead() {
    const items = this.unread()
    if (!items.length) return
    this.lastRead = Math.max(...items.map((m) => Date.parse(m.at)))
    try {
      localStorage.setItem(`qa:read:${this.code}`, String(this.lastRead))
    } catch {
      /* ignore */
    }
  }

  async sendChat(text: string): Promise<boolean> {
    try {
      await api.chat(this.token, text)
      this.scheduleRefresh()
      return true
    } catch (e) {
      if (isNetworkError(e)) this.toast('bad', 'Нет связи — сообщение не отправлено')
      else this.announceError(e as ApiError)
      return false
    }
  }

  /** Сигнал тревоги: координаты, если телефон быстро их отдаст, и текст. */
  async raiseSOS(text: string): Promise<boolean> {
    this.busy = true
    let coords: GeolocationCoordinates | null = null
    try {
      coords = await new Promise<GeolocationCoordinates | null>((resolve) => {
        if (!('geolocation' in navigator)) return resolve(null)
        navigator.geolocation.getCurrentPosition(
          (p) => resolve(p.coords),
          () => resolve(null),
          { enableHighAccuracy: true, timeout: 8000, maximumAge: 60_000 },
        )
      })
      await api.sos(this.token, {
        text,
        ...(coords ? { lat: coords.latitude, lng: coords.longitude, accuracy: coords.accuracy } : {}),
      })
      if (navigator.vibrate) navigator.vibrate([100, 60, 100])
      this.toast('ok', coords ? 'Сигнал и координаты отправлены ведущему' : 'Сигнал отправлен (без координат)', 5000)
      this.scheduleRefresh()
      return true
    } catch (e) {
      const contact = this.state?.host_contact
      this.toast('bad', `Не удалось отправить сигнал.${contact ? ' Звоните ведущему: ' + contact : ' Звоните ведущему.'}`, 12000)
      return false
    } finally {
      this.busy = false
    }
  }

  /** Запуск; возвращает функцию остановки. */
  start(): () => void {
    const tick = setInterval(() => (this.now = Date.now()), 1000)
    // Страховка, если SSE режет прокси: опрос раз в 30 секунд.
    const poll = setInterval(() => this.refresh(), 30_000)
    const onOnline = () => {
      this.online = true
      this.refresh()
      this.flushOutbox()
    }
    const onOffline = () => (this.online = false)
    const onVisible = () => {
      if (document.visibilityState === 'visible') {
        this.refresh()
        this.flushOutbox()
      }
    }
    addEventListener('online', onOnline)
    addEventListener('offline', onOffline)
    document.addEventListener('visibilitychange', onVisible)

    this.refresh()
    this.connect()
    this.flushOutbox()

    return () => {
      clearInterval(tick)
      clearInterval(poll)
      removeEventListener('online', onOnline)
      removeEventListener('offline', onOffline)
      document.removeEventListener('visibilitychange', onVisible)
      this.es?.close()
      if (this.refreshTimer) clearTimeout(this.refreshTimer)
    }
  }

  /** Секунды игры с учётом времени, прошедшего после загрузки состояния. */
  elapsed(): number {
    const s = this.state
    if (!s) return 0
    const running = s.team.status === 'playing'
    return s.elapsed_sec + (running ? Math.floor((this.now - this.fetchedAt) / 1000) : 0)
  }

  toast(kind: ToastKind, text: string, ms = 3500) {
    const id = ++this.toastSeq
    // Не больше двух сразу: новое вытесняет старое, задание не закрывается.
    this.toasts = [...this.toasts.slice(-1), { id, kind, text }]
    setTimeout(() => (this.toasts = this.toasts.filter((t) => t.id !== id)), ms)
  }

  async refresh() {
    try {
      const s = await api.state(this.token)
      this.fetchedAt = Date.now()
      this.state = s
      this.error = null
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        this.unauthorized = true
        return
      }
      if (!this.state) this.error = isNetworkError(e) ? 'Нет связи с сервером' : (e as Error).message
    }
  }

  private scheduleRefresh() {
    if (this.refreshTimer) return
    this.refreshTimer = setTimeout(() => {
      this.refreshTimer = null
      this.refresh()
    }, 150)
  }

  private connect() {
    this.es = new EventSource(`/api/v1/stream?token=${encodeURIComponent(this.token)}`)
    // При (пере)подключении перечитываем состояние: пока стрима не было,
    // могло измениться то, о чём уведомление не дошло.
    this.es.onopen = () => {
      this.streaming = true
      this.scheduleRefresh()
    }
    this.es.onerror = () => (this.streaming = false) // EventSource переподключится сам
    this.es.addEventListener('state_changed', (e) => {
      const seq = Number((e as MessageEvent).data)
      if (!this.state || seq !== this.state.seq) this.scheduleRefresh()
    })
  }

  private announce(r: AnswerResult) {
    const [kind, text] = RESULT[r.result]
    this.toast(kind, text, r.result === 'inaccurate' ? 6000 : 3500)
    if (navigator.vibrate && r.result === 'correct') navigator.vibrate(80)
  }

  announceError(e: ApiError) {
    this.toast('bad', e.message, 5000)
  }

  /** Отправить или поставить в очередь. Возвращает результат, если он есть сразу. */
  private async deliver(item: OutboxItem): Promise<AnswerResult | null> {
    this.busy = true
    try {
      const r = await send(item)
      this.announce(r)
      this.scheduleRefresh()
      return r
    } catch (e) {
      if (isNetworkError(e)) {
        await outbox.add(item)
        this.queued = (await outbox.all(this.token)).length
        this.toast('info', 'Нет связи — ответ сохранён и отправится сам, когда сеть появится', 5000)
      } else {
        this.announceError(e as ApiError)
      }
      return null
    } finally {
      this.busy = false
    }
  }

  submit(body: Omit<AnswerBody, 'client_event_id'>) {
    const id = newId()
    return this.deliver({ id, token: this.token, createdAt: Date.now(), answer: { ...body, client_event_id: id } })
  }

  async submitPhoto(file: File, target: PhotoTarget) {
    this.busy = true
    let blob: Blob
    try {
      blob = await compressImage(file)
    } catch {
      this.busy = false
      this.toast('bad', 'Не удалось прочитать фото — попробуйте другое')
      return null
    }
    return this.deliver({ id: newId(), token: this.token, createdAt: Date.now(), photo: { blob, ...target } })
  }

  /** Координаты с высокой точностью. */
  async locate(): Promise<GeolocationCoordinates | null> {
    if (!('geolocation' in navigator)) {
      this.toast('bad', 'Браузер не умеет определять местоположение')
      return null
    }
    this.busy = true
    try {
      const pos = await new Promise<GeolocationPosition>((resolve, reject) =>
        navigator.geolocation.getCurrentPosition(resolve, reject, { enableHighAccuracy: true, timeout: 20_000, maximumAge: 0 }),
      )
      return pos.coords
    } catch (e) {
      const err = e as GeolocationPositionError
      this.toast(
        'bad',
        err.code === 1
          ? 'Нет доступа к геолокации. Разрешите её для этого сайта в настройках браузера'
          : 'Не удалось определить местоположение — попробуйте ещё раз',
        6000,
      )
      return null
    } finally {
      this.busy = false
    }
  }

  async checkIn(levelId?: string) {
    const c = await this.locate()
    if (!c) return null
    return this.submit({ kind: 'geo', level_id: levelId, lat: c.latitude, lng: c.longitude, accuracy: c.accuracy })
  }

  async requestHint(levelId: string, idx: number) {
    try {
      await api.hint(this.token, levelId, idx)
      this.scheduleRefresh()
    } catch (e) {
      if (isNetworkError(e)) this.toast('bad', 'Нет связи — попробуйте ещё раз')
      else this.announceError(e as ApiError)
    }
  }

  async flushOutbox() {
    try {
      this.queued = await flush(this.token, (d: Delivery) => {
        if (d.result) this.announce(d.result)
        else if (d.error) this.announceError(d.error)
      })
      this.scheduleRefresh()
    } catch {
      // IndexedDB недоступна (приватный режим) — очереди нет
    }
  }

  /** Действия капитана с командой: ошибка — во всплывашку, успех — обновить. */
  async teamAction(fn: () => Promise<unknown>, okText?: string) {
    try {
      await fn()
      if (okText) this.toast('ok', okText)
      this.refresh()
      return true
    } catch (e) {
      if (isNetworkError(e)) this.toast('bad', 'Нет связи — попробуйте ещё раз')
      else this.announceError(e as ApiError)
      return false
    }
  }
}
