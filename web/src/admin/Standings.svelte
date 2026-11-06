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

  const MEDAL = ['', '🥇', '🥈', '🥉']
  const results = $derived(d.results)
  const finished = $derived(d.run_status === 'finished')
  // Живая таблица; после утверждения — утверждённая (поправки после него видны в живой до повторного утверждения).
  let showLive = $state(false)
  const rows = $derived(results && !showLive ? results.standings : d.standings)

  let message = $state('')
  let seenMsg = ''
  $effect(() => {
    const m = results?.message ?? ''
    if (m !== seenMsg) {
      seenMsg = m
      message = m
    }
  })

  async function approve() {
    const again = results ? ' Предыдущие итоги будут заменены.' : ''
    if (!confirm(`Утвердить итоги? Таблица станет окончательной, команды увидят свои места и поздравление.${again}`)) return
    await live.act(() => live.api.approveResults(runId, message), 'Итоги утверждены — команды получили поздравления')
    showLive = false
  }

  function set(v: boolean | null) {
    live.act(() => live.api.setLeaderboard(runId, v), v === null ? 'Как в сценарии' : v ? 'Таблица открыта игрокам' : 'Таблица скрыта')
  }
</script>

{#if finished}
  <section class="card stack approve" class:done={!!results}>
    <h2>Подведение итогов</h2>
    {#if results}
      <p>
        Итоги утверждены в {new Date(results.at).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })}. Команды видят свои места и
        поздравление. Если внесли поправки (баллы/время в карточке команды) — утвердите заново.
      </p>
    {:else}
      <p>
        Все команды на финише. Проверьте таблицу ниже, при необходимости поправьте баллы или время в карточке команды (вкладка «Команды»), затем
        утвердите итоги — места 1–3 отметятся медалями, каждая команда увидит поздравление со своим местом.
      </p>
    {/if}
    <div>
      <label for="cmsg">Сообщение командам (необязательно)</label>
      <textarea id="cmsg" rows="2" bind:value={message} placeholder="Например: награждение в 18:00 у фонтана, спасибо всем!"></textarea>
    </div>
    <button onclick={approve}>{results ? 'Утвердить заново' : 'Утвердить итоги и поздравить команды'}</button>
  </section>
{/if}

<section class="card">
  <div class="row wrap head">
    <h2 class="grow">{results && !showLive ? 'Итоги (утверждены)' : 'Таблица'}</h2>
    {#if results}
      <button class="ghost small" onclick={() => (showLive = !showLive)}>{showLive ? 'Показать утверждённые' : 'Показать живую'}</button>
    {/if}
    <span class="muted small">Игрокам:</span>
    <div class="seg">
      <button class:on={mode === 'scenario'} onclick={() => set(null)}>По сценарию</button>
      <button class:on={mode === 'show'} onclick={() => set(true)}>Показать</button>
      <button class:on={mode === 'hide'} onclick={() => set(false)}>Скрыть</button>
    </div>
  </div>
  <p class="muted small">
    По сценарию таблица {VIS[lb.visibility] ?? lb.visibility}. После утверждения итогов она видна всем. {d.scoring_rule}
    {#if lb.frozen}<strong class="warm">Заморожена: игроки видят снимок, вы — живую таблицу.</strong>{/if}
  </p>
  <table class="grid">
    <thead>
      <tr><th class="num">#</th><th>Команда</th><th class="num">Уровней</th><th class="num">Баллы</th><th class="num">Время</th><th></th></tr>
    </thead>
    <tbody>
      {#each rows as s (s.team_id)}
        <tr>
          <td class="num"><strong>{results && !showLive && MEDAL[s.rank] ? MEDAL[s.rank] : s.rank}</strong></td>
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
  .approve {
    border-color: var(--accent);
  }
  .approve.done {
    border-color: var(--ok);
  }
  .approve textarea {
    resize: vertical;
  }
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
