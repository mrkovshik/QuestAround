<script lang="ts">
  import type { ScoreItem } from '../lib/api'
  import { minutes, points } from '../lib/format'

  let { items, total, penaltySec, rule }: { items: ScoreItem[]; total: number; penaltySec: number; rule: string } = $props()

  function delta(it: ScoreItem): string {
    const parts: string[] = []
    if (it.points) parts.push(`${it.points > 0 ? '+' : '−'}${Math.abs(it.points)}`)
    if (it.time_sec) parts.push(`${it.time_sec > 0 ? '+' : '−'}${minutes(Math.abs(it.time_sec))}`)
    return parts.join(', ')
  }
</script>

<section class="card">
  <h3>Баллы команды</h3>
  <p class="muted small rule">{rule}</p>
  {#if items.length === 0}
    <p class="muted small">Пока ничего — баллы появятся за пройденные уровни и бонусы.</p>
  {:else}
    <ul class="ledger">
      {#each items as it, i (i)}
        <li class="row">
          <span class="grow">
            {it.title}
            {#if it.reason}<span class="muted small">— {it.reason}</span>{/if}
          </span>
          <span class="d" class:minus={it.points < 0 || it.time_sec > 0}>{delta(it)}</span>
        </li>
      {/each}
    </ul>
  {/if}
  <div class="row total">
    <strong class="grow">Итого</strong>
    <strong>{points(total)}</strong>
  </div>
  {#if penaltySec}
    <div class="row total small">
      <span class="grow muted">Штрафное время (прибавляется к времени команды)</span>
      <span>{penaltySec > 0 ? '+' : '−'}{minutes(Math.abs(penaltySec))}</span>
    </div>
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
  }
</style>
