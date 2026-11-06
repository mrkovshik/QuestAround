<script lang="ts">
  import { ApiError } from '../lib/api'
  import { router } from '../lib/router.svelte'
  import { ScenarioError, type AdminApi, type Run, type ScenarioIssue, type ScenarioVersion } from './adminApi'
  import { RUN_STATUS } from './time'

  let { api, onUnauthorized }: { api: AdminApi; onUnauthorized: () => void } = $props()

  let runs = $state<Run[]>([])
  let scenarios = $state<ScenarioVersion[]>([])
  let error = $state<string | null>(null)

  let yaml = $state('')
  let issues = $state<ScenarioIssue[]>([])
  let uploadMsg = $state<string | null>(null)
  let uploading = $state(false)

  let versionId = $state('')
  let title = $state('')
  let creating = $state(false)

  async function load() {
    try {
      ;[runs, scenarios] = await Promise.all([api.runs(), api.scenarios()])
      if (!versionId && scenarios.length) versionId = scenarios[0].id
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) onUnauthorized()
      else error = 'Нет связи с сервером'
    }
  }

  $effect(() => {
    load()
  })

  async function pickFile(e: Event) {
    const f = (e.target as HTMLInputElement).files?.[0]
    if (f) yaml = await f.text()
  }

  async function upload() {
    uploading = true
    issues = []
    uploadMsg = null
    try {
      const sv = await api.uploadScenario(yaml)
      uploadMsg =
        `Загружено: «${sv.title}» — ${sv.slug}, версия ${sv.version}.` +
        (sv.version > 1 ? ' Чтобы применить её к уже созданной игре, откройте игру и нажмите «Заменить» у сценария.' : '')
      await load()
      versionId = sv.id
      title = title || sv.title || ''
    } catch (e) {
      if (e instanceof ScenarioError) issues = e.details
      else uploadMsg = e instanceof ApiError ? e.message : 'Нет связи с сервером'
    } finally {
      uploading = false
    }
  }

  async function create(e: SubmitEvent) {
    e.preventDefault()
    creating = true
    try {
      const r = await api.createRun(versionId, title.trim())
      router.go(`/admin/runs/${r.id}`)
    } catch (err) {
      error = err instanceof ApiError ? err.message : 'Нет связи с сервером'
    } finally {
      creating = false
    }
  }

  const fmt = (iso: string) => new Date(iso).toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
</script>

<div class="wrap">
  {#if error}<div class="notice">{error}</div>{/if}

  <section class="card">
    <h2>Игры</h2>
    {#if runs.length === 0}
      <p class="muted">Игр пока нет — загрузите сценарий и создайте игру ниже.</p>
    {:else}
      <table class="grid">
        <thead>
          <tr><th>Название</th><th>Код</th><th>Статус</th><th class="num">Команд</th><th>Создана</th></tr>
        </thead>
        <tbody>
          {#each runs as r (r.id)}
            <tr class="click" onclick={() => router.go(`/admin/runs/${r.id}`)}>
              <td><strong>{r.title}</strong></td>
              <td class="mono">{r.public_code}</td>
              <td>
                {#if r.status}<span class="badge {RUN_STATUS[r.status][1]}">{RUN_STATUS[r.status][0]}</span>{/if}
              </td>
              <td class="num">{r.teams}</td>
              <td>{fmt(r.created_at)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </section>

  <div class="cols">
    <section class="card stack">
      <h2>Загрузить сценарий</h2>
      <p class="muted small">YAML-файл сценария. Перед сохранением он проверяется — ошибки покажутся с номерами строк.</p>
      <input type="file" accept=".yaml,.yml,text/yaml" onchange={pickFile} />
      <textarea class="mono" rows="10" bind:value={yaml} placeholder="…или вставьте YAML сюда"></textarea>
      <button disabled={!yaml.trim() || uploading} onclick={upload}>{uploading ? 'Проверяем…' : 'Проверить и сохранить'}</button>
      {#if uploadMsg}<div class="notice">{uploadMsg}</div>{/if}
      {#if issues.length}
        <div class="issues">
          <strong>Ошибок: {issues.length}</strong>
          <ul>
            {#each issues as i, k (k)}
              <li>
                {#if i.line}<span class="mono line">строка {i.line}</span>{/if}
                {#if i.path}<code>{i.path}</code>{/if}
                {i.message}
              </li>
            {/each}
          </ul>
        </div>
      {/if}
    </section>

    <section class="card">
      <form class="stack" onsubmit={create}>
        <h2>Новая игра</h2>
        {#if scenarios.length === 0}
          <p class="muted">Сначала загрузите сценарий.</p>
        {:else}
          <div>
            <label for="sv">Сценарий</label>
            <select id="sv" bind:value={versionId} style="width:100%">
              {#each scenarios as sv (sv.id)}
                <option value={sv.id}>{sv.slug} · версия {sv.version} · {fmt(sv.created_at)}</option>
              {/each}
            </select>
          </div>
          <div>
            <label for="title">Название для игроков</label>
            <input id="title" bind:value={title} placeholder="Например, «Горячий Ключ: пилот, 29 ноября»" />
          </div>
          <button disabled={!versionId || creating}>Создать игру</button>
        {/if}
      </form>
    </section>
  </div>
</div>

<style>
  .issues {
    background: var(--bad-soft);
    border-radius: 10px;
    padding: 0.75rem 1rem;
  }
  .issues ul {
    margin: 0.5rem 0 0;
    padding-left: 1.1rem;
  }
  .issues li {
    margin-bottom: 0.3rem;
  }
  .line {
    font-weight: 700;
    margin-right: 0.35rem;
  }
  textarea {
    resize: vertical;
  }
</style>
