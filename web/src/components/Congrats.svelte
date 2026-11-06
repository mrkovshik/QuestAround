<script lang="ts">
  import type { Results } from '../lib/api'
  import { clock, points } from '../lib/format'

  let { results, team }: { results: Results; team: string } = $props()

  const rank = $derived(results.rank ?? 0)
  const medal = $derived(['', '🥇', '🥈', '🥉'][rank] ?? '')
  const headline = $derived(
    rank === 1 ? 'Победа!' : rank === 2 || rank === 3 ? 'Вы в призёрах!' : 'Спасибо за игру!',
  )
</script>

<section class="card congrats" class:top={rank > 0 && rank <= 3}>
  {#if medal}<div class="medal" aria-hidden="true">{medal}</div>{/if}
  <h2>{headline}</h2>
  <p class="place">
    «{team}» — <strong>{rank} место</strong> из {results.teams}
  </p>
  <p class="muted">{points(results.points)} · время {clock(results.elapsed_sec)}</p>
  {#if results.message}
    <p class="msg task">{results.message}</p>
  {/if}
</section>

<style>
  .congrats {
    text-align: center;
    padding: 1.5rem 1rem;
  }
  .congrats.top {
    border-color: var(--accent);
    background: linear-gradient(180deg, var(--accent-soft), var(--card) 70%);
  }
  .medal {
    font-size: 4rem;
    line-height: 1;
    margin-bottom: 0.5rem;
    animation: pop 0.6s cubic-bezier(0.2, 1.6, 0.4, 1);
  }
  @keyframes pop {
    from {
      transform: scale(0.3);
      opacity: 0;
    }
  }
  .place {
    font-size: 1.15rem;
  }
  .msg {
    margin-top: 0.75rem;
    padding: 0.75rem;
    border-radius: 10px;
    background: var(--bg);
  }
</style>
