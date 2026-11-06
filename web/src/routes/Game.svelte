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
  import Comms from '../components/Comms.svelte'

  let { code, token, onLogout }: { code: string; token: string; onLogout: () => void } = $props()

  // Game пересоздаётся при смене игры или токена ({#key} в App) — начальных значений достаточно.
  // svelte-ignore state_referenced_locally
  const live = new Live(code, token)
  $effect(() => live.start())

  let tab = $state<'play' | 'team' | 'comms' | 'table'>('play')

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

  // Непрочитанные сообщения ведущего (общие и в канал команды) — лентой, пока не отмечены.
  const unread = $derived(live.state ? live.unread() : [])
  const hhmm = (iso: string) => new Date(iso).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })

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
    {#if unread.length && tab !== 'comms'}
      <div class="card msg">
        <div class="row">
          <strong class="grow">{unread.length === 1 ? 'Сообщение ведущего' : `Новые сообщения ведущего: ${unread.length}`}</strong>
          <button class="ghost small" onclick={() => live.markRead()}>Прочитано</button>
        </div>
        {#each unread as m, i (i)}
          <div class="um">
            <span class="small muted">{hhmm(m.at)}{m.personal ? ' · вашей команде' : ''}</span>
            <p class="task">{m.text}</p>
          </div>
        {/each}
        {#if unread.some((m) => m.personal)}
          <button class="ghost small" onclick={() => (tab = 'comms')}>Ответить</button>
        {/if}
      </div>
    {/if}

    {#if tab === 'comms'}
      <Comms {live} />
    {:else if tab === 'team' || waiting}
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

  <nav class="tabs">
    <button class:active={tab === 'play'} onclick={() => (tab = 'play')}>{waiting ? 'Старт' : 'Задание'}</button>
    {#if !waiting}
      <button class:active={tab === 'team'} onclick={() => (tab = 'team')}>Команда</button>
    {/if}
    <button class:active={tab === 'comms'} onclick={() => (tab = 'comms')}>
      Связь{#if unread.length}<span class="dot">{unread.length}</span>{/if}
    </button>
    {#if s.leaderboard.visible}
      <button class:active={tab === 'table'} onclick={() => (tab = 'table')}>Таблица</button>
    {/if}
  </nav>
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
  .um p {
    margin: 0.1rem 0 0.5rem;
  }
  .dot {
    display: inline-block;
    min-width: 1.3em;
    margin-left: 0.3em;
    padding: 0 0.3em;
    border-radius: 999px;
    background: var(--bad);
    color: #fff;
    font-size: 0.75rem;
    line-height: 1.3em;
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
