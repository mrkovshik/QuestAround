// Общее состояние конструктора: черновик, ошибки сервера, библиотека картинок.

import { getContext, setContext } from 'svelte'
import type { AdminApi, Asset, ScenarioIssue } from '../adminApi'
import { DEFAULTS, emptyDraft, type Draft, type FlowMode, type ScoringMode } from './model'

export type View = 'settings' | 'routes' | 'schedule' | 'yaml' | number // число — индекс уровня

export class EditorState {
  draft = $state<Draft>(emptyDraft())
  issues = $state<ScenarioIssue[]>([])
  assets = $state<Asset[]>([])
  view = $state<View>('settings')

  constructor(public api: AdminApi) {}

  get scoring(): ScoringMode {
    return this.draft.scoring.mode ?? DEFAULTS.scoring
  }
  get flow(): FlowMode {
    return this.draft.flow.mode ?? DEFAULTS.flow
  }
  /** Есть ли у уровней баллы в текущем режиме зачёта. */
  get hasPoints() {
    return this.scoring !== 'time'
  }
  get hasTime() {
    return this.scoring !== 'points'
  }

  /** Ошибки ровно по этому пути и по элементам списка под ним (path[0], path[1]…). */
  at(path: string): string[] {
    return this.issues.filter((i) => i.path === path || i.path?.startsWith(path + '[')).map((i) => i.message)
  }

  /** Сколько ошибок внутри поддерева. */
  count(prefix: string): number {
    return this.issues.filter((i) => i.path === prefix || i.path?.startsWith(prefix + '.') || i.path?.startsWith(prefix + '[')).length
  }

  /** Куда вести по клику на ошибку. */
  viewOf(path?: string): View {
    const m = path?.match(/^levels\[(\d+)\]/)
    if (m) return Number(m[1])
    if (path?.startsWith('flow.route_list')) return 'routes'
    return 'settings'
  }

  async loadAssets() {
    try {
      this.assets = await this.api.assets()
    } catch {
      /* библиотека вспомогательная */
    }
  }

  assetURL(name: string): string | undefined {
    if (/^https?:\/\//.test(name)) return name
    return this.assets.find((a) => a.name === name)?.url
  }

  /** Перейти к полю с ошибкой: переключить вид и сфокусировать ближайший элемент с data-path. */
  focus(path?: string) {
    this.view = this.viewOf(path)
    if (!path) return
    requestAnimationFrame(() =>
      requestAnimationFrame(() => {
        let p = path
        while (p) {
          const el = document.querySelector<HTMLElement>(`[data-path="${CSS.escape(p)}"]`)
          if (el) {
            el.scrollIntoView({ block: 'center', behavior: 'smooth' })
            el.querySelector<HTMLElement>('input, textarea, select')?.focus({ preventScroll: true })
            el.classList.add('flash')
            setTimeout(() => el.classList.remove('flash'), 1200)
            return
          }
          p = p.replace(/(\.[^.[\]]+|\[\d+\])$/, '')
        }
      }),
    )
  }
}

const KEY = Symbol('editor')
export const provideEditor = (e: EditorState) => setContext(KEY, e)
export const useEditor = () => getContext<EditorState>(KEY)
