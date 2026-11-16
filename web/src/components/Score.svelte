<script lang="ts">
  import type { ScoreItem, ScoringMode } from '../lib/api'
  import { clock, minutes, points } from '../lib/format'

  let {
    items,
    total,
    penaltySec,
    bonusSec = 0,
    elapsedSec = 0,
    cleanSec = 0,
    mode = 'points',
    rule,
  }: {
    items: ScoreItem[]
    total: number
    penaltySec: number
    bonusSec?: number
    elapsedSec?: number
    cleanSec?: number
    mode?: ScoringMode
    rule: string
  } = $props()

  const timeMode = $derived(mode === 'time')
  // В зачёте по времени баллов нет — показываем только то, что двигает время.
  const shown = $derived(timeMode ? items.filter((it) => it.time_sec) : items)

  function delta(it: ScoreItem): string {
    const parts: string[] = []
    if (it.points && !timeMode) parts.push(`${it.points > 0 ? '+' : '−'}${Math.abs(it.points)}`)
    if (it.time_sec) parts.push(`${it.time_sec > 0 ? '+' : '−'}${minutes(Math.abs(it.time_sec))}`)
    return parts.join(', ')
  }
</script>

<section class="card">
  <h3>{timeMode ? 'Время команды' : 'Баллы команды'}</h3>
  <p class="muted small rule">{rule}</p>
  {#if shown.length === 0}
    <p class="muted small">
      {timeMode ? 'Штрафов и бонусов пока нет.' : 'Пока ничего — баллы появятся за пройденные уровни и бонусы.'}
    </p>
  {:else}
    <ul class="ledger">
      {#each shown as it, i (i)}
        <li class="row">
          <span class="grow">
            {it.title}
            {#if it.reason}<span class="muted small">— {it.reason}</span>{/if}
          </span>
          <span class="d" class:minus={(!timeMode && it.points < 0) || it.time_sec > 0}>{delta(it)}</span>
        </li>
      {/each}
    </ul>
  {/if}
  {#if timeMode}
    <div class="row total small">
      <span class="grow muted">Чистое время</span>
      <span class="mono">{clock(cleanSec)}</span>
    </div>
    {#if penaltySec}
      <div class="row total small">
        <span class="grow muted">Штрафы и поправки</span>
        <span class="mono">{penaltySec > 0 ? '+' : '−'}{minutes(Math.abs(penaltySec))}</span>
      </div>
    {/if}
    {#if bonusSec}
      <div class="row total small">
        <span class="grow muted">Бонусы</span>
        <span class="mono">−{minutes(bonusSec)}</span>
      </div>
    {/if}
    <div class="row total">
      <strong class="grow">Итоговое время</strong>
      <strong class="mono">{clock(elapsedSec)}</strong>
    </div>
  {:else}
    <div class="row total">
      <strong class="grow">Итого</strong>
      <strong>{points(total)}</strong>
    </div>
    {#if penaltySec || bonusSec}
      <div class="row total small">
        <span class="grow muted">Время: штрафы {bonusSec ? 'и бонусы ' : ''}(при равенстве баллов)</span>
        <span>{penaltySec ? `+${minutes(penaltySec)}` : ''}{penaltySec && bonusSec ? ' ' : ''}{bonusSec ? `−${minutes(bonusSec)}` : ''}</span>
      </div>
    {/if}
  {/if}
</section>

<style>
  .rule {
    margin-bottom: 0.6rem;
  }
  .ledger {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .ledger li {
    padding: 0.4rem 0;
    border-top: 1px solid var(--line);
    align-items: flex-start;
  }
  .d {
    font-weight: 700;
    color: var(--ok);
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
  }
  .d.minus {
    color: var(--bad);
  }
  .total {
    border-top: 2px solid var(--line);
    padding-top: 0.5rem;
    margin-top: 0.25rem;
  }
  .total.small {
    border-top: 0;
    padding-top: 0.2rem;
    margin-top: 0;
  }
  .total.small:first-of-type {
    border-top: 2px solid var(--line);
  }
</style>
