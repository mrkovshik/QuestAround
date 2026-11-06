<script lang="ts">
  import { api, ApiError, type Standing } from '../lib/api'
  import type { Live } from '../lib/live.svelte'
  import { clock } from '../lib/format'

  let { live }: { live: Live } = $props()

  let rows = $state<Standing[]>([])
  let frozen = $state(false)
  let final = $state(false)
  let rule = $state('')
  let error = $state<string | null>(null)

  // Перезагружаем таблицу при каждом изменении состояния игры.
  $effect(() => {
    live.state?.seq
    api
      .leaderboard(live.token)
      .then((r) => {
        rows = r.standings
        frozen = r.frozen
        final = r.final
        rule = r.rule
        error = null
      })
      .catch((e) => (error = e instanceof ApiError ? e.message : 'Нет связи'))
  })

  const myTeam = $derived(live.state?.team.id)
  const MEDAL = ['', '🥇', '🥈', '🥉']
</script>

<section class="card">
  <h2>{final ? 'Итоги' : 'Таблица'}</h2>
  {#if final}
    <p class="small muted">Итоги утверждены ведущим.</p>
  {:else if live.state?.run_status === 'finished'}
    <p class="notice small">Предварительно — ведущий ещё может внести поправки.</p>
  {/if}
  {#if frozen}<p class="notice small">Таблица заморожена до конца игры — интрига!</p>{/if}
  {#if error}
    <p class="muted">{error}</p>
  {:else}
    <table>
      <thead>
        <tr><th>#</th><th>Команда</th><th>Ур.</th><th>Баллы</th><th>Время</th></tr>
      </thead>
      <tbody>
        {#each rows as r (r.team_id)}
          <tr class:me={r.team_id === myTeam}>
            <td>{final && MEDAL[r.rank] ? MEDAL[r.rank] : r.rank}</td>
            <td class="name">{r.name}{#if r.finished}<span class="muted"> ✓</span>{/if}</td>
            <td>{r.levels}</td>
            <td>{r.points}</td>
            <td class="num">{clock(r.elapsed_sec)}</td>
          </tr>
        {/each}
      </tbody>
    </table>
    {#if rule}<p class="muted small rule">{rule}</p>{/if}
  {/if}
</section>

<style>
  table {
    width: 100%;
    border-collapse: collapse;
    font-variant-numeric: tabular-nums;
  }
  th,
  td {
    text-align: left;
    padding: 0.5rem 0.3rem;
    border-bottom: 1px solid var(--line);
  }
  th {
    font-size: 0.8rem;
    color: var(--muted);
    font-weight: 600;
  }
  .name {
    max-width: 10rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .num {
    text-align: right;
  }
  .rule {
    margin: 0.75rem 0 0;
  }
  tr.me td {
    background: var(--accent-soft);
    font-weight: 700;
  }
</style>
