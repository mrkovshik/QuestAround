<script lang="ts">
  import { clock } from '../lib/format'
  import type { RunLive } from './runLive.svelte'

  let { live, runId }: { live: RunLive; runId: string } = $props()

  const d = $derived(live.dash!)
  const lb = $derived(d.leaderboard)
  const VIS: Record<string, string> = {
    always: 'видна всю игру',
    hidden: 'скрыта до конца игры',
    after_finish: 'видна командам после их финиша',
    host: 'скрыта, пока вы не включите',
  }
  const mode = $derived(lb.override === null ? 'scenario' : lb.override ? 'show' : 'hide')

  function set(v: boolean | null) {
    live.act(() => live.api.setLeaderboard(runId, v), v === null ? 'Как в сценарии' : v ? 'Таблица открыта игрокам' : 'Таблица скрыта')
  }
</script>

<section class="card">
  <div class="row wrap head">
    <h2 class="grow">Таблица</h2>
    <span class="muted small">Игрокам:</span>
    <div class="seg">
      <button class:on={mode === 'scenario'} onclick={() => set(null)}>По сценарию</button>
      <button class:on={mode === 'show'} onclick={() => set(true)}>Показать</button>
      <button class:on={mode === 'hide'} onclick={() => set(false)}>Скрыть</button>
    </div>
  </div>
  <p class="muted small">
    По сценарию таблица {VIS[lb.visibility] ?? lb.visibility}. После завершения игры она видна всем.
    {#if lb.frozen}<strong class="warm">Заморожена: игроки видят снимок, вы — живую таблицу.</strong>{/if}
  </p>
  <table class="grid">
    <thead>
      <tr><th class="num">#</th><th>Команда</th><th class="num">Уровней</th><th class="num">Баллы</th><th class="num">Время</th><th></th></tr>
    </thead>
    <tbody>
      {#each d.standings as s (s.team_id)}
        <tr>
          <td class="num"><strong>{s.rank}</strong></td>
          <td>{s.name}</td>
          <td class="num">{s.levels}</td>
          <td class="num"><strong>{s.points}</strong></td>
          <td class="num">{clock(s.elapsed_sec)}</td>
          <td>{#if s.finished}<span class="badge ok">финиш</span>{/if}</td>
        </tr>
      {/each}
    </tbody>
  </table>
</section>

<style>
  .head {
    margin-bottom: 0.5rem;
  }
  .seg {
    display: inline-flex;
    border: 1.5px solid var(--line);
    border-radius: 10px;
    overflow: hidden;
  }
  .seg button {
    border-radius: 0;
    background: transparent;
    color: var(--muted);
  }
  .seg button.on {
    background: var(--accent);
    color: var(--accent-ink);
  }
</style>
