<script lang="ts">
  import { ApiError } from '../lib/api'
  import { ScenarioConflict, type ScenarioVersion } from './adminApi'
  import type { RunLive } from './runLive.svelte'

  let { live, runId }: { live: RunLive; runId: string } = $props()

  const api = $derived(live.api)
  const run = $derived(live.run!)
  const pending = $derived(live.dash?.run_status === 'pending')
  const running = $derived(live.dash?.run_status === 'running')
  const editable = $derived(pending || running)
  let conflict = $state<string[]>([])

  let versions = $state<ScenarioVersion[]>([])
  let current = $state<ScenarioVersion | null>(null)
  let pick = $state('')
  let showSource = $state(false)

  async function load() {
    try {
      versions = await api.scenarios()
      current = await api.scenario(run.scenario_version_id)
    } catch {
      /* панель вспомогательная — молча */
    }
  }

  $effect(() => {
    run.scenario_version_id
    load()
  })

  // Сначала — версии того же сценария, новые сверху.
  // Во время игры — только версии того же сценария.
  const options = $derived(
    [...versions]
      .sort((a, b) => Number(b.slug === current?.slug) - Number(a.slug === current?.slug))
      .filter((v) => v.id !== current?.id && (pending || v.slug === current?.slug)),
  )
  $effect(() => {
    if (!options.some((o) => o.id === pick)) pick = options[0]?.id ?? ''
  })

  async function change() {
    const v = versions.find((x) => x.id === pick)
    if (!v) return
    conflict = []
    const other = v.slug !== current?.slug ? `\n\nВнимание: это другой сценарий (${v.slug}), не новая версия текущего.` : ''
    const live_ = running ? '\n\nИгра идёт: изменения сразу увидят все команды (телефоны обновятся сами).' : ''
    if (!confirm(`Заменить сценарий на «${v.slug}», версия ${v.version}? Команды и игроки сохранятся.${other}${live_}`)) return
    try {
      const r = await api.changeScenario(runId, v.id)
      live.run = r
      live.note('ok', 'Сценарий заменён')
      live.refresh()
    } catch (e) {
      if (e instanceof ScenarioConflict) conflict = e.details.map((d) => d.message)
      live.note('bad', e instanceof ApiError ? e.message : 'Нет связи с сервером')
    }
  }

  const fmt = (iso: string) => new Date(iso).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
</script>

<div class="scn">
  <div class="row wrap small">
    <span class="muted">Сценарий:</span>
    {#if current}
      <strong>{current.title ?? current.slug}</strong>
      <span class="mono">{current.slug} · версия {current.version}</span>
    {/if}
    <button class="ghost small" onclick={() => (showSource = !showSource)}>{showSource ? 'Скрыть YAML' : 'Показать YAML'}</button>
    {#if editable && options.length}
      <span class="grow"></span>
      <select bind:value={pick}>
        {#each options as v (v.id)}
          <option value={v.id}>{v.slug} · версия {v.version} · {fmt(v.created_at)}</option>
        {/each}
      </select>
      <button class="secondary small" disabled={!pick} onclick={change}>Заменить</button>
    {/if}
  </div>
  {#if pending}
    <p class="muted small hint">
      До старта сценарий можно заменить на любой: загрузите исправленный YAML на странице игр, затем выберите новую версию здесь. Код
      игры и зарегистрированные команды сохранятся.
    </p>
  {:else if running}
    <p class="muted small hint">
      Игра идёт: можно заменить на новую версию того же сценария, если менялось только содержимое — тексты, коды, координаты, радиусы,
      подсказки, баллы. Добавлять или убирать уровни, ответы, подсказки и бонусы нельзя.
    </p>
  {:else}
    <p class="muted small hint">Игра завершена.</p>
  {/if}
  {#if conflict.length}
    <div class="notice small conflict">
      <strong>Не заменено — изменилась структура:</strong>
      <ul>
        {#each conflict as c, i (i)}<li>{c}</li>{/each}
      </ul>
    </div>
  {/if}
  {#if showSource && current?.source}
    <pre class="mono src">{current.source}</pre>
  {/if}
</div>

<style>
  .scn {
    margin-top: 0.9rem;
    padding-top: 0.75rem;
    border-top: 1px solid var(--line);
  }
  .hint {
    margin: 0.35rem 0 0;
  }
  .conflict {
    margin-top: 0.5rem;
    background: var(--bad-soft);
  }
  .conflict ul {
    margin: 0.3rem 0 0;
    padding-left: 1.1rem;
  }
  .src {
    margin: 0.75rem 0 0;
    max-height: 50vh;
    overflow: auto;
    background: var(--bg);
    border-radius: 10px;
    padding: 0.75rem;
    font-size: 0.82rem;
    white-space: pre;
  }
</style>
