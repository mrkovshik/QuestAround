<script lang="ts">
  // Расписание подсказок и сливов всех уровней в одной таблице. Время — от открытия уровня у команды;
  // его можно править прямо здесь.
  import Dur from './Dur.svelte'
  import { humanDuration, isZeroDur, penaltyShort, scheduledAfter, type Hint, type Level } from './model'
  import { useEditor } from './state.svelte'

  const ed = useEditor()
  const d = $derived(ed.draft)
  const hintAfter = $derived((d.defaults.hint_after ?? []).filter(Boolean))
  const maxHints = $derived(Math.max(0, ...d.levels.map((l) => l.hints?.length ?? 0)))
  const anyLocation = $derived(d.levels.some((l) => l.location))
  const anySkip = $derived(d.levels.some((l) => l.skip))

  // Когда подсказка откроется сама: своё время, из расписания или никогда.
  function autoAt(l: Level, h: Hint, i: number): string | undefined {
    if (h.after?.trim()) return isZeroDur(h.after) ? undefined : h.after
    return scheduledAfter(l, i, hintAfter)
  }

  function kind(l: Level, h: Hint, i: number): string {
    const at = autoAt(l, h, i)
    if (h.on_request) return at ? `запрос ${penaltyShort(h.penalty)} или сама` : `только запрос ${penaltyShort(h.penalty)}`
    if (!at) return 'не откроется'
    return h.after?.trim() ? 'сама' : 'сама, по расписанию'
  }

  function fmtMin(min: number): string {
    if (min <= 0) return 'сразу'
    const h = Math.floor(min / 60)
    const m = Math.round(min % 60)
    return [h && `${h} ч`, m && `${m} мин`].filter(Boolean).join(' ') || '< 1 мин'
  }

  const bad = (path: string) => ed.count(path) > 0
  const human = (v?: string) => (v && !isZeroDur(v) ? humanDuration(v) || v : '')

  // Сводная лента: что и когда становится доступно (для проверки, что всё не сваливается в одну минуту).
  const timeline = $derived.by(() => {
    const toMin = (v?: string) => {
      const m = v?.match(/^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$/)
      return m ? Number(m[1] ?? 0) * 60 + Number(m[2] ?? 0) + Number(m[3] ?? 0) / 60 : NaN
    }
    const rows: { min: number; text: string }[] = []
    d.levels.forEach((l, li) => {
      ;(l.hints ?? []).forEach((h, i) => {
        const at = autoAt(l, h, i)
        if (at) rows.push({ min: toMin(at), text: `${li + 1}. ${l.title || l.id} — подсказка ${i + 1}` })
      })
      if (l.location) rows.push({ min: toMin(l.location.available_after || '0m'), text: `${li + 1}. ${l.title || l.id} — слив локации` })
      if (l.skip) rows.push({ min: toMin(l.skip.available_after || '0m'), text: `${li + 1}. ${l.title || l.id} — слив уровня` })
    })
    return rows.filter((r) => Number.isFinite(r.min)).sort((a, b) => a.min - b.min)
  })
</script>

<div class="sched">
  <h2>Расписание подсказок и сливов</h2>
  <p class="muted small">
    Время — от открытия уровня у команды. Пустое поле у подсказки без «по запросу» — время из расписания в настройках игры
    {#if hintAfter.length}({hintAfter.map((x) => humanDuration(x) || x).join(', ')}){/if}; 0 — сама не откроется.
    У слива пусто — кнопка доступна сразу. Тип подсказки и штрафы меняются на странице уровня.
  </p>

  {#if !d.levels.length}
    <p class="muted">Уровней пока нет.</p>
  {:else if !maxHints && !anyLocation && !anySkip}
    <p class="muted">Ни у одного уровня нет подсказок и сливов.</p>
  {:else}
    <div class="scroll card">
      <table class="grid">
        <thead>
          <tr>
            <th>Уровень</th>
            {#each Array.from({ length: maxHints }, (_, i) => i) as i (i)}<th>Подсказка {i + 1}</th>{/each}
            {#if anyLocation}<th>Слив локации</th>{/if}
            {#if anySkip}<th>Слив уровня</th>{/if}
          </tr>
        </thead>
        <tbody>
          {#each d.levels as l, li (l)}
            {@const P = `levels[${li}]`}
            <tr>
              <td class="lvl">
                <button class="link" onclick={() => (ed.view = li)}>
                  <span class="muted">{li + 1}.</span>
                  {l.title || l.id}
                </button>
                {#if l.rate_limit}<div class="muted small">защита от перебора</div>{/if}
              </td>
              {#each Array.from({ length: maxHints }, (_, i) => i) as i (i)}
                {@const h = l.hints?.[i]}
                <td class:err={h && bad(`${P}.hints[${i}]`)} data-path="{P}.hints[{i}]">
                  {#if h}
                    <Dur bind:value={h.after} placeholder={h.on_request ? '0' : (scheduledAfter(l, i, hintAfter) ?? '—')} />
                    <div class="tag" class:warn={h.on_request} class:off={!h.on_request && !autoAt(l, h, i)}>{kind(l, h, i)}</div>
                  {:else}
                    <span class="muted">—</span>
                  {/if}
                </td>
              {/each}
              {#if anyLocation}
                <td class:err={l.location && bad(`${P}.location`)} data-path="{P}.location">
                  {#if l.location}
                    {@const loc = l.location}
                    <Dur bind:value={loc.available_after} placeholder="0" />
                    <div class="tag warn">{human(loc.available_after) ? '' : 'сразу · '}{penaltyShort(loc.penalty)}</div>
                  {:else}
                    <span class="muted">—</span>
                  {/if}
                </td>
              {/if}
              {#if anySkip}
                <td class:err={l.skip && bad(`${P}.skip`)} data-path="{P}.skip">
                  {#if l.skip}
                    {@const sk = l.skip}
                    <Dur bind:value={sk.available_after} placeholder="0" />
                    <div class="tag warn">{human(sk.available_after) ? '' : 'сразу · '}{penaltyShort(sk.penalty)}</div>
                  {:else}
                    <span class="muted">—</span>
                  {/if}
                </td>
              {/if}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if timeline.length}
      <section class="card">
        <h3>По времени от открытия уровня</h3>
        <ul class="tl">
          {#each timeline as r, k (k)}
            <li><span class="t mono">{fmtMin(r.min)}</span> {r.text}</li>
          {/each}
        </ul>
      </section>
    {/if}
  {/if}
</div>

<style>
  .sched {
    max-width: 1300px;
  }
  .scroll {
    overflow-x: auto;
    padding: 0;
  }
  table {
    min-width: 100%;
  }
  th {
    white-space: nowrap;
  }
  td {
    vertical-align: top;
    min-width: 150px;
  }
  td.lvl {
    min-width: 180px;
  }
  td.err {
    background: var(--bad-soft);
  }
  .link {
    background: none;
    color: var(--text);
    font-weight: 600;
    padding: 0;
    min-height: 0;
    text-align: left;
  }
  .link:hover {
    text-decoration: underline;
  }
  .tag {
    margin-top: 0.25rem;
    font-size: 0.75rem;
    color: var(--ok);
  }
  .tag.warn {
    color: var(--warn);
  }
  .tag.off {
    color: var(--bad);
  }
  .tl {
    list-style: none;
    margin: 0.5rem 0 0;
    padding: 0;
    display: grid;
    gap: 0.25rem;
  }
  .t {
    display: inline-block;
    min-width: 6rem;
    color: var(--muted);
  }
</style>
