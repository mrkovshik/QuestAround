<script lang="ts">
  import { Live } from '../lib/live.svelte'
  import { clearToken } from '../lib/session'
  import { clock, points } from '../lib/format'
  import Toasts from '../components/Toasts.svelte'
  import TeamPanel from '../components/TeamPanel.svelte'
  import Play from '../components/Play.svelte'
  import Leaderboard from '../components/Leaderboard.svelte'
  import Score from '../components/Score.svelte'
  import Congrats from '../components/Congrats.svelte'

  let { code, token, onLogout }: { code: string; token: string; onLogout: () => void } = $props()

  // Game пересоздаётся при смене игры или токена ({#key} в App) — начальных значений достаточно.
  // svelte-ignore state_referenced_locally
  const live = new Live(code, token)
  $effect(() => live.start())

  let tab = $state<'play' | 'team' | 'table'>('play')

  function logout() {
    clearToken(code)
    onLogout()
  }

  $effect(() => {
    if (live.unauthorized) logout()
  })

  const s = $derived(live.state)
  const status = $derived(s?.team.status)
  const waiting = $derived(status === 'registered' || status === 'scheduled')
  const remaining = $derived.by(() => {
    if (!s?.time_limit_at || status !== 'playing') return null
    return Math.floor((Date.parse(s.time_limit_at) - live.now) / 1000)
  })
  const startIn = $derived(s?.start_at ? Math.floor((Date.parse(s.start_at) - live.now) / 1000) : null)

  // Последнее сообщение ведущего, пока игрок его не закрыл.
  // svelte-ignore state_referenced_locally
  let dismissed = $state(Number(localStorage.getItem(`qa:msg:${code}`) ?? 0))
  const lastMsg = $derived(s?.messages.at(-1))
  const showMsg = $derived(!!lastMsg && Date.parse(lastMsg.at) > dismissed)
  function dismissMsg() {
    dismissed = Date.parse(lastMsg!.at)
    localStorage.setItem(`qa:msg:${code}`, String(dismissed))
  }

  $effect(() => {
    if (tab === 'table' && s && !s.leaderboard.visible) tab = 'play'
  })
</script>

<Toasts toasts={live.toasts} />

{#if !s}
  <main class="page">
    {#if live.error}
      <div class="card">
        <p>{live.error}</p>
        <button onclick={() => live.refresh()}>Повторить</button>
      </div>
    {:else}
      <div class="spinner"></div>
    {/if}
  </main>
{:else}
  <header class="bar">
    <div class="grow">
      <div class="team">{s.team.name}</div>
      <div class="muted small">{s.title}</div>
    </div>
    {#if status === 'playing' || status === 'finished'}
      <button class="stats" onclick={() => (tab = 'team')} title="Расшифровка баллов">
        <div class="big">{remaining !== null ? clock(remaining) : clock(live.elapsed())}</div>
        <div class="muted small">{points(s.points)} · {s.levels_done}/{s.levels_total}</div>
      </button>
    {/if}
  </header>

  {#if !live.online || live.queued > 0}
    <div class="net" class:off={!live.online}>
      {#if !live.online}Нет сети.{/if}
      {#if live.queued > 0}В очереди ответов: {live.queued} — отправятся сами.{/if}
    </div>
  {/if}

  <main class="page">
    {#if showMsg && lastMsg}
      <div class="card msg">
        <div class="row">
          <strong class="grow">Сообщение ведущего</strong>
          <button class="ghost small" onclick={dismissMsg}>Скрыть</button>
        </div>
        <p class="task">{lastMsg.text}</p>
      </div>
    {/if}

    {#if tab === 'team' || waiting}
      {#if waiting}
        <div class="card center wait">
          {#if status === 'scheduled' && startIn !== null && startIn > 0}
            <div class="muted">Ваш старт через</div>
            <div class="countdown">{clock(startIn)}</div>
          {:else if s.run_status === 'pending'}
            <h2>Ждём старта</h2>
            <p class="muted">Задание появится здесь, как только ведущий запустит игру. Держите страницу открытой.</p>
          {:else}
            <h2>Скоро старт</h2>
          {/if}
        </div>
      {/if}
      {#if !waiting}
        <Score items={s.score} total={s.points} penaltySec={s.penalty_sec} rule={s.scoring_rule} />
      {/if}
      <TeamPanel {live} {logout} />
    {:else if tab === 'table'}
      <Leaderboard {live} />
    {:else if status === 'finished'}
      {#if s.results}
        <Congrats results={s.results} team={s.team.name} />
        <button class="secondary block" onclick={() => (tab = 'table')}>Итоговая таблица</button>
      {:else}
        <div class="card center">
          <h2>Финиш!</h2>
          <p>{points(s.points)} · уровней: {s.levels_done} из {s.levels_total} · время {clock(live.elapsed())}</p>
          {#if s.run_status === 'finished'}
            <p class="muted small">Ведущий подводит итоги — результат появится здесь, держите страницу открытой.</p>
          {:else}
            <p class="muted small">Ждём, пока финишируют остальные команды. Итоги появятся здесь.</p>
          {/if}
          {#if s.leaderboard.visible}
            <button class="secondary" onclick={() => (tab = 'table')}>Предварительная таблица</button>
          {/if}
        </div>
      {/if}
      <Score items={s.score} total={s.points} penaltySec={s.penalty_sec} rule={s.scoring_rule} />
      <Play {live} />
    {:else if status === 'withdrawn'}
      <div class="card"><p>Команда снята с игры.</p></div>
    {:else}
      <Play {live} />
    {/if}
  </main>

  {#if !waiting}
    <nav class="tabs">
      <button class:active={tab === 'play'} onclick={() => (tab = 'play')}>Задание</button>
      <button class:active={tab === 'team'} onclick={() => (tab = 'team')}>Команда</button>
      {#if s.leaderboard.visible}
        <button class:active={tab === 'table'} onclick={() => (tab = 'table')}>Таблица</button>
      {/if}
    </nav>
  {/if}
{/if}

<style>
  .bar {
    position: sticky;
    top: 0;
    z-index: 5;
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.6rem 1rem;
    background: var(--card);
    border-bottom: 1px solid var(--line);
  }
  .team {
    font-weight: 700;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .stats {
    text-align: right;
    background: transparent;
    color: var(--text);
    padding: 0;
    min-height: 0;
    font-weight: 400;
  }
  .big {
    font-size: 1.3rem;
    font-weight: 700;
    font-variant-numeric: tabular-nums;
  }
  .net {
    background: var(--warn-soft);
    padding: 0.4rem 1rem;
    font-size: 0.875rem;
    text-align: center;
  }
  .net.off {
    background: var(--bad-soft);
  }
  .msg {
    border-color: var(--accent);
  }
  .wait {
    padding: 1.5rem 1rem;
  }
  .countdown {
    font-size: 2.6rem;
    font-weight: 800;
    font-variant-numeric: tabular-nums;
  }
  .tabs {
    position: fixed;
    bottom: 0;
    left: 0;
    right: 0;
    z-index: 5;
    display: flex;
    background: var(--card);
    border-top: 1px solid var(--line);
    padding-bottom: var(--safe-bottom);
  }
  .tabs button {
    flex: 1;
    border-radius: 0;
    background: transparent;
    color: var(--muted);
    min-height: 54px;
  }
  .tabs button.active {
    color: var(--accent);
    box-shadow: inset 0 3px 0 var(--accent);
  }
</style>
