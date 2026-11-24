<script lang="ts">
  // Конструктор сценария. Черновик живёт в браузере (автосохранение в localStorage),
  // проверяет его сервер тем же валидатором, что и загрузку YAML; «Сохранить» создаёт новую версию.
  import './editor.css'
  import { untrack } from 'svelte'
  import { ApiError } from '../../lib/api'
  import { router } from '../../lib/router.svelte'
  import { ScenarioError, type AdminApi, type ScenarioIssue } from '../adminApi'
  import LevelForm from './LevelForm.svelte'
  import Routes from './Routes.svelte'
  import Schedule from './Schedule.svelte'
  import Settings from './Settings.svelte'
  import YamlView from './YamlView.svelte'
  import { emptyDraft, fromYaml, newLevel, toYaml, type Level } from './model'
  import { EditorState, provideEditor } from './state.svelte'

  let { api, id, onUnauthorized }: { api: AdminApi; id: string; onUnauthorized: () => void } = $props()

  // svelte-ignore state_referenced_locally — api не меняется за время жизни страницы
  const ed = provideEditor(new EditorState(api))

  let loading = $state(true)
  let fatal = $state('')
  let base = $state<{ id: string; slug: string; version: number } | null>(null)
  let savedYaml = $state('') // что лежит на сервере — для «есть несохранённые изменения»
  let restore = $state<{ at: string; yaml: string } | null>(null)
  let status = $state<'idle' | 'checking' | 'ok' | 'errors' | 'offline'>('idle')
  let nextVersion = $state(0)
  let saving = $state(false)
  let note = $state<{ kind: 'ok' | 'bad'; text: string } | null>(null)

  const yaml = $derived(toYaml(ed.draft))
  const dirty = $derived(!loading && yaml !== savedYaml)
  const storeKey = $derived(`qa:editor:${base?.slug ?? 'new'}`)

  async function load() {
    try {
      await ed.loadAssets()
      if (id === 'new') {
        ed.draft = emptyDraft()
        ed.draft.levels.push(newLevel(ed.draft))
        savedYaml = ''
      } else {
        const sv = await api.scenario(id)
        base = { id: sv.id, slug: sv.slug, version: sv.version }
        ed.draft = fromYaml(sv.source ?? '')
        savedYaml = toYaml(ed.draft)
        ed.view = ed.draft.levels.length ? 0 : 'settings'
      }
      const saved = localStorage.getItem(`qa:editor:${base?.slug ?? 'new'}`)
      if (saved) {
        const s = JSON.parse(saved) as { at: string; yaml: string; from?: string }
        if (s.yaml && s.yaml !== savedYaml) restore = s
      }
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) onUnauthorized()
      else fatal = e instanceof ApiError ? e.message : (e as Error).message
    } finally {
      loading = false
    }
  }
  // id читаем один раз: после первого сохранения адрес меняется на /admin/scenarios/<версия>,
  // но перезагружать конструктор не нужно.
  $effect(() => {
    untrack(load)
  })

  // Проверка на сервере и автосохранение черновика — с задержкой после правок.
  $effect(() => {
    const src = yaml
    if (loading) return
    const key = storeKey
    const isDirty = dirty
    status = 'checking'
    const t = setTimeout(async () => {
      try {
        if (isDirty && !restore) localStorage.setItem(key, JSON.stringify({ at: new Date().toISOString(), yaml: src }))
      } catch {
        /* приватный режим */
      }
      try {
        const r = await api.validateScenario(src)
        ed.issues = []
        nextVersion = r.next_version
        status = 'ok'
      } catch (e) {
        if (e instanceof ScenarioError) {
          ed.issues = e.details
          status = 'errors'
        } else if (e instanceof ApiError && e.status === 401) onUnauthorized()
        else status = 'offline'
      }
    }, 600)
    return () => clearTimeout(t)
  })

  function onbeforeunload(e: BeforeUnloadEvent) {
    if (dirty) e.preventDefault()
  }

  function doRestore() {
    if (!restore) return
    try {
      ed.draft = fromYaml(restore.yaml)
    } catch (e) {
      note = { kind: 'bad', text: (e as Error).message }
    }
    restore = null
  }
  function dropRestore() {
    localStorage.removeItem(storeKey)
    restore = null
  }

  async function save() {
    saving = true
    note = null
    try {
      const sv = await api.uploadScenario(yaml)
      localStorage.removeItem(storeKey)
      localStorage.removeItem('qa:editor:new')
      savedYaml = yaml
      note = { kind: 'ok', text: `Сохранено: ${sv.slug}, версия ${sv.version}` + (sv.version > 1 ? '. Чтобы применить к созданной игре — «Заменить» у сценария на странице игры.' : '') }
      base = { id: sv.id, slug: sv.slug, version: sv.version }
      router.go(`/admin/scenarios/${sv.id}`, true)
    } catch (e) {
      if (e instanceof ScenarioError) {
        ed.issues = e.details
        status = 'errors'
        note = { kind: 'bad', text: 'В сценарии есть ошибки — исправьте их перед сохранением' }
      } else note = { kind: 'bad', text: e instanceof ApiError ? e.message : 'Нет связи с сервером' }
    } finally {
      saving = false
    }
  }

  async function testRun() {
    if (!base) return
    try {
      const r = await api.createRun(base.id, `${ed.draft.title || base.slug} — тест`)
      window.open(`/admin/runs/${r.id}`, '_blank')
    } catch (e) {
      note = { kind: 'bad', text: e instanceof ApiError ? e.message : 'Нет связи с сервером' }
    }
  }

  function back() {
    if (dirty && !confirm('Есть несохранённые изменения. Черновик останется в этом браузере. Уйти?')) return
    router.go('/admin')
  }

  // --- список уровней ---
  function addLevel() {
    const d = ed.draft
    const l = newLevel(d)
    // перед финишем, если он закреплён последним
    const last = d.levels.findIndex((x) => x.pin === 'last')
    const at = last >= 0 ? last : d.levels.length
    d.levels.splice(at, 0, l)
    ed.view = at
  }

  let dragFrom = $state<number | null>(null)
  let dragOver = $state<number | null>(null)
  function drop(to: number) {
    const from = dragFrom
    dragFrom = dragOver = null
    if (from === null || from === to) return
    const levels = ed.draft.levels
    const cur = typeof ed.view === 'number' ? levels[ed.view] : null
    const [x] = levels.splice(from, 1)
    levels.splice(from < to ? to - 1 : to, 0, x)
    if (cur) ed.view = levels.indexOf(cur)
  }

  const ICON: Record<string, string> = { code: '#', regex: '.*', choice: '☰', geo: '◎', photo: '📷', text: '✎' }
  const icons = (l: Level) => [...new Set(l.answers.map((a) => ICON[a.type] ?? '?'))].join(' ')

  const globalIssues = $derived(ed.issues.filter((i) => ed.viewOf(i.path) === 'settings'))
  const issueLabel = (i: ScenarioIssue) => {
    const m = i.path?.match(/^levels\[(\d+)\]/)
    if (m) {
      const l = ed.draft.levels[Number(m[1])]
      return `${Number(m[1]) + 1}. ${l?.title || l?.id || ''}`
    }
    return i.path?.startsWith('flow.route_list') ? 'Маршруты' : 'Настройки'
  }
</script>

<svelte:window {onbeforeunload} />

<div class="editor">
  <header class="bar">
    <button class="ghost small" onclick={back}>← К играм</button>
    <div class="grow ttl">
      <strong>{ed.draft.title || 'Новый сценарий'}</strong>
      {#if base}<span class="muted small mono">{base.slug} · версия {base.version}</span>{:else}<span class="muted small">ещё не сохранён</span>{/if}
    </div>
    <span class="status small {status}">
      {#if status === 'checking'}проверяем…
      {:else if status === 'ok'}✓ ошибок нет
      {:else if status === 'errors'}ошибок: {ed.issues.length}
      {:else if status === 'offline'}нет связи с сервером{/if}
    </span>
    {#if base && !dirty}
      <button class="secondary small" onclick={testRun} title="Создать игру с этой версией и открыть в новой вкладке">Тестовая игра</button>
    {/if}
    <button disabled={!dirty || saving || status === 'errors' || status === 'checking'} onclick={save}>
      {saving ? 'Сохраняем…' : !dirty ? 'Сохранено' : nextVersion > 1 ? `Сохранить версию ${nextVersion}` : 'Сохранить'}
    </button>
  </header>

  {#if note}
    <div class="note {note.kind}">{note.text} <button class="ghost small" onclick={() => (note = null)}>✕</button></div>
  {/if}
  {#if restore}
    <div class="notice restore">
      В этом браузере есть несохранённый черновик от {new Date(restore.at).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })}.
      <button class="small" onclick={doRestore}>Восстановить</button>
      <button class="ghost small" onclick={dropRestore}>Отбросить</button>
    </div>
  {/if}

  {#if loading}
    <div class="spinner"></div>
  {:else if fatal}
    <div class="wrap"><div class="notice">{fatal}</div></div>
  {:else}
    <div class="body">
      <aside>
        <nav>
          <button class="nav" class:active={ed.view === 'settings'} onclick={() => (ed.view = 'settings')}>
            ⚙ Настройки игры
            {#if globalIssues.length}<span class="counter">{globalIssues.length}</span>{/if}
          </button>
          {#if ed.flow === 'linear'}
            <button class="nav" class:active={ed.view === 'routes'} onclick={() => (ed.view = 'routes')}>
              ⇄ Маршруты
              {#if ed.count('flow.route_list')}<span class="counter">{ed.count('flow.route_list')}</span>{/if}
            </button>
          {/if}
          <button class="nav" class:active={ed.view === 'schedule'} onclick={() => (ed.view = 'schedule')}>⏱ Подсказки и сливы</button>
          <button class="nav" class:active={ed.view === 'yaml'} onclick={() => (ed.view = 'yaml')}>{'</>'} YAML</button>
        </nav>

        <div class="lvhead row">
          <strong class="grow">Уровни · {ed.draft.levels.length}</strong>
          <button class="small" onclick={addLevel}>＋ уровень</button>
        </div>
        <ol class="levels" role="list">
          {#each ed.draft.levels as l, i (l)}
            {@const n = ed.count(`levels[${i}]`)}
            <li
              class:active={ed.view === i}
              class:over={dragOver === i && dragFrom !== i}
              draggable="true"
              ondragstart={(e) => ((dragFrom = i), e.dataTransfer?.setData('text/plain', String(i)))}
              ondragover={(e) => (e.preventDefault(), (dragOver = i))}
              ondragleave={() => dragOver === i && (dragOver = null)}
              ondrop={(e) => (e.preventDefault(), drop(i))}
              ondragend={() => (dragFrom = dragOver = null)}
            >
              <button class="lv" onclick={() => (ed.view = i)}>
                <span class="idx">{i + 1}</span>
                <span class="grow">
                  <span class="t">{l.title || 'Без названия'}</span>
                  <span class="meta mono">
                    {l.id}
                    {#if ed.flow === 'linear' && l.pin === 'first'}<span class="badge">старт</span>{/if}
                    {#if ed.flow === 'linear' && l.pin === 'last'}<span class="badge">финиш</span>{/if}
                    {#if ed.flow === 'linear' && l.chain}<span class="badge warn">⛓ {l.chain}</span>{/if}
                    {#if ed.flow === 'graph' && l.unlock}<span class="badge warn">условие</span>{/if}
                  </span>
                </span>
                <span class="ic" title="Типы ответов">{icons(l)}</span>
                {#if n}<span class="counter">{n}</span>{/if}
              </button>
            </li>
          {/each}
          <li
            class="tail"
            class:over={dragOver === ed.draft.levels.length}
            ondragover={(e) => (e.preventDefault(), (dragOver = ed.draft.levels.length))}
            ondrop={(e) => (e.preventDefault(), drop(ed.draft.levels.length))}
          ></li>
        </ol>
        {#if ed.draft.levels.length > 1}<p class="muted small tip">Перетаскивайте уровни, чтобы поменять порядок.</p>{/if}

        {#if ed.issues.length}
          <div class="issues">
            <strong>Ошибки · {ed.issues.length}</strong>
            <ul>
              {#each ed.issues as i, k (k)}
                <li><button class="link" onclick={() => ed.focus(i.path)}><span class="where">{issueLabel(i)}</span> {i.message}</button></li>
              {/each}
            </ul>
          </div>
        {/if}
      </aside>

      <main>
        {#if ed.view === 'settings'}
          <Settings existingSlug={base?.slug} />
        {:else if ed.view === 'routes'}
          <Routes />
        {:else if ed.view === 'schedule'}
          <Schedule />
        {:else if ed.view === 'yaml'}
          <YamlView />
        {:else if typeof ed.view === 'number' && ed.draft.levels[ed.view]}
          {#key ed.draft.levels[ed.view]}
            <LevelForm level={ed.draft.levels[ed.view]} index={ed.view} />
          {/key}
        {:else}
          <p class="muted">Добавьте первый уровень.</p>
        {/if}
      </main>
    </div>
  {/if}
</div>

<style>
  .bar {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.6rem 1.25rem;
    background: var(--card);
    border-bottom: 1px solid var(--line);
    position: sticky;
    top: 49px;
    z-index: 9;
  }
  .ttl {
    display: flex;
    gap: 0.6rem;
    align-items: baseline;
    min-width: 0;
  }
  .ttl strong {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .status {
    white-space: nowrap;
    color: var(--muted);
  }
  .status.ok {
    color: var(--ok);
  }
  .status.errors,
  .status.offline {
    color: var(--bad);
    font-weight: 600;
  }
  .note,
  .restore {
    margin: 0.75rem 1.25rem 0;
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .note {
    padding: 0.6rem 0.9rem;
    border-radius: 10px;
    font-weight: 600;
  }
  .note.ok {
    background: var(--ok-soft);
    color: var(--ok);
  }
  .note.bad {
    background: var(--bad-soft);
    color: var(--bad);
  }
  .body {
    display: grid;
    grid-template-columns: 300px minmax(0, 1fr);
    gap: 1.25rem;
    padding: 1rem 1.25rem 4rem;
    max-width: 1500px;
    margin: 0 auto;
  }
  aside {
    position: sticky;
    top: 110px;
    align-self: start;
    max-height: calc(100dvh - 120px);
    overflow: auto;
    padding-right: 0.25rem;
  }
  nav {
    display: grid;
    gap: 0.15rem;
    margin-bottom: 1rem;
  }
  .nav {
    display: flex;
    align-items: center;
    justify-content: space-between;
    background: transparent;
    color: var(--text);
    font-weight: 600;
    text-align: left;
  }
  .nav.active,
  li.active .lv {
    background: var(--accent-soft);
    color: var(--accent);
  }
  .lvhead {
    margin-bottom: 0.4rem;
  }
  .levels {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 0.15rem;
  }
  .levels li {
    border-top: 2px solid transparent;
  }
  .levels li.over {
    border-top-color: var(--accent);
  }
  .levels li.tail {
    min-height: 12px;
  }
  .lv {
    width: 100%;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    text-align: left;
    background: var(--card);
    border: 1px solid var(--line);
    color: var(--text);
    font-weight: 400;
    padding: 0.4rem 0.6rem;
    cursor: grab;
  }
  .idx {
    width: 1.4rem;
    color: var(--muted);
    font-variant-numeric: tabular-nums;
  }
  .t {
    display: block;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .meta {
    display: flex;
    gap: 0.3rem;
    align-items: center;
    font-size: 0.75rem;
    color: var(--muted);
    flex-wrap: wrap;
  }
  .meta .badge {
    font-family: system-ui, sans-serif;
    font-size: 0.68rem;
  }
  .ic {
    font-size: 0.8rem;
    color: var(--muted);
    white-space: nowrap;
  }
  .tip {
    margin-top: 0.4rem;
  }
  .issues {
    margin-top: 1rem;
    background: var(--bad-soft);
    border-radius: 10px;
    padding: 0.6rem 0.75rem;
  }
  .issues ul {
    list-style: none;
    margin: 0.4rem 0 0;
    padding: 0;
    display: grid;
    gap: 0.25rem;
  }
  .link {
    background: none;
    color: var(--text);
    font-weight: 400;
    text-align: left;
    padding: 0;
    min-height: 0;
    font-size: 0.85rem;
  }
  .link:hover {
    text-decoration: underline;
  }
  .where {
    font-weight: 700;
    color: var(--bad);
  }
</style>
