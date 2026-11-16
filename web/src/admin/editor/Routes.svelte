<script lang="ts">
  // Маршруты: превью (как построит сервер при старте) и ручные маршруты для routes: explicit.
  import { ApiError } from '../../lib/api'
  import { ScenarioError } from '../adminApi'
  import { toYaml } from './model'
  import { useEditor } from './state.svelte'

  const ed = useEditor()
  const d = $derived(ed.draft)
  const explicit = $derived(d.flow.routes === 'explicit')
  const title = (id: string) => d.levels.find((l) => l.id === id)?.title || id

  let teams = $state(5)
  let preview = $state<string[][] | null>(null)
  let err = $state('')

  // Превью пересчитывается при изменениях (с задержкой).
  $effect(() => {
    const yaml = toYaml(d)
    const n = teams
    if (ed.flow !== 'linear' || !n || n < 1) return
    const t = setTimeout(async () => {
      try {
        preview = (await ed.api.previewRoutes(yaml, Math.min(n, 50))).routes ?? null
        err = ''
      } catch (e) {
        preview = null
        err = e instanceof ScenarioError ? 'Исправьте ошибки в сценарии — тогда покажем маршруты' : e instanceof ApiError ? e.message : 'Нет связи с сервером'
      }
    }, 500)
    return () => clearTimeout(t)
  })

  // --- ручные маршруты ---
  const list = $derived(d.flow.route_list ?? [])
  function addRoute() {
    const base = list.length ? [...list[list.length - 1]] : d.levels.map((l) => l.id)
    d.flow.route_list = [...list, base]
  }
  function removeRoute(i: number) {
    d.flow.route_list = list.filter((_, k) => k !== i)
  }
  function removeStep(r: number, i: number) {
    d.flow.route_list![r].splice(i, 1)
  }
  function addStep(r: number, id: string) {
    if (id) d.flow.route_list![r].push(id)
  }

  let drag: { r: number; i: number } | null = null
  function drop(r: number, i: number) {
    if (!drag || drag.r !== r) return
    const row = d.flow.route_list![r]
    const [x] = row.splice(drag.i, 1)
    row.splice(i, 0, x)
    drag = null
  }
</script>

<div class="routes">
  <h2>Маршруты</h2>
  {#if ed.flow !== 'linear'}
    <p class="muted">Маршруты задаются только при открытии уровней цепочкой. Сейчас: {ed.flow === 'storm' ? 'штурм' : 'граф'}.</p>
  {:else}
    {#if explicit}
      <section class="card" data-path="flow.route_list">
        <h3>Маршруты вручную</h3>
        <p class="muted small">Команды распределяются по маршрутам по кругу. Перетаскивайте уровни, чтобы поменять порядок.</p>
        {#each ed.at('flow.route_list') as m, k (k)}<div class="msg">{m}</div>{/each}
        {#each list as route, r (r)}
          {@const missing = d.levels.filter((l) => !route.includes(l.id))}
          <div class="route" data-path="flow.route_list[{r}]">
            <span class="muted small num">{r + 1}</span>
            <div class="steps">
              {#each route as id, i (id + i)}
                <span
                  class="step"
                  role="listitem"
                  draggable="true"
                  ondragstart={() => (drag = { r, i })}
                  ondragover={(e) => e.preventDefault()}
                  ondrop={() => drop(r, i)}
                >
                  {title(id)}
                  <button class="x" title="Убрать" onclick={() => removeStep(r, i)}>✕</button>
                </span>
              {/each}
              {#if missing.length}
                <select value="" onchange={(e) => (addStep(r, e.currentTarget.value), (e.currentTarget.value = ''))}>
                  <option value="">＋ уровень</option>
                  {#each missing as l (l.id)}<option value={l.id}>{l.title || l.id}</option>{/each}
                </select>
              {/if}
            </div>
            <button class="ghost small del" onclick={() => removeRoute(r)}>Удалить</button>
          </div>
        {/each}
        <button class="secondary small" onclick={addRoute}>＋ маршрут</button>
      </section>
    {/if}

    <section class="card">
      <div class="row">
        <h3 class="grow">Как пройдут команды</h3>
        <label for="teams" class="inline">Команд:</label>
        <input id="teams" type="number" min="1" max="50" bind:value={teams} style="width:5rem" />
      </div>
      {#if d.flow.routes === 'random'}<p class="muted small">Случайные маршруты — пример; в игре порядок будет другим.</p>{/if}
      {#if err}
        <p class="muted">{err}</p>
      {:else if preview}
        <table class="grid">
          <tbody>
            {#each preview as route, i (i)}
              <tr>
                <td class="muted nowrap">Команда {i + 1}</td>
                <td>
                  {#each route as id, k (k)}<span class="step ro">{title(id)}</span>{#if k < route.length - 1}<span class="arrow">→</span>{/if}{/each}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </section>
  {/if}
</div>

<style>
  .routes {
    max-width: 1100px;
  }
  .route {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
    padding: 0.5rem 0;
    border-bottom: 1px solid var(--line);
  }
  .num {
    width: 1.5rem;
    padding-top: 0.35rem;
  }
  .steps {
    flex: 1;
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
    align-items: center;
  }
  .step {
    display: inline-flex;
    align-items: center;
    gap: 0.2rem;
    padding: 0.2rem 0.5rem;
    border-radius: 999px;
    background: var(--accent-soft);
    color: var(--accent);
    font-size: 0.85rem;
    font-weight: 600;
    cursor: grab;
  }
  .step.ro {
    cursor: default;
  }
  .x {
    min-height: 0;
    padding: 0 0.15rem;
    background: none;
    color: inherit;
    font-size: 0.75rem;
    opacity: 0.6;
  }
  .arrow {
    color: var(--muted);
    margin: 0 0.25rem;
  }
  .del {
    color: var(--bad);
  }
  .nowrap {
    white-space: nowrap;
  }
  .inline {
    margin: 0;
  }
  select {
    font-size: 0.85rem;
    padding: 0.15rem 0.4rem;
  }
</style>
