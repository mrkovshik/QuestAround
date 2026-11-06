<script lang="ts">
  import { ApiError } from '../lib/api'
  import QR from '../components/QR.svelte'
  import type { AdminApi } from './adminApi'
  import { RunLive } from './runLive.svelte'
  import { RUN_STATUS } from './time'
  import Dashboard from './Dashboard.svelte'
  import Registration from './Registration.svelte'
  import Moderation from './Moderation.svelte'
  import Standings from './Standings.svelte'
  import Journal from './Journal.svelte'
  import TeamDrawer from './TeamDrawer.svelte'
  import ScenarioPanel from './ScenarioPanel.svelte'
  import SosBanner from './SosBanner.svelte'
  import Comms from './Comms.svelte'

  let { api, runId, onUnauthorized }: { api: AdminApi; runId: string; onUnauthorized: () => void } = $props()

  // svelte-ignore state_referenced_locally
  const live = new RunLive(api, runId)
  $effect(() => live.start())

  let tab = $state<'dash' | 'reg' | 'mod' | 'comms' | 'table' | 'log'>('dash')
  let chatTeam = $state<string | null>(null)
  let teamId = $state<string | null>(null)
  let showQR = $state(false)
  let message = $state('')

  const d = $derived(live.dash)
  const run = $derived(live.run)
  const gameUrl = $derived(run ? `${location.origin}/g/${run.public_code}` : '')
  const team = $derived(d?.teams.find((t) => t.id === teamId) ?? null)

  // Непрочитанные от капитанов — для счётчика на вкладке (та же метка, что и во вкладке «Связь»).
  const chatUnread = $derived.by(() => {
    if (!d) return 0
    void d.seq
    let seen: Record<string, number> = {}
    try {
      seen = JSON.parse(localStorage.getItem(`qa:admin:seen:${runId}`) ?? '{}')
    } catch {
      /* ignore */
    }
    return d.teams.reduce((n, t) => n + t.chat.filter((m) => !m.from_host && m.seq > (seen[t.id] ?? 0)).length, 0)
  })

  function openChat(id: string) {
    chatTeam = id
    tab = 'comms'
  }

  $effect(() => {
    if (live.error && /авторизац/i.test(live.error)) onUnauthorized()
  })
  $effect(() => {
    if (run) document.title = `${run.title} — ведущий`
  })

  async function startRun() {
    if (!d) return
    const n = d.teams.length
    if (!confirm(`Запустить игру? Команд: ${n}. Регистрация новых команд закроется.`)) return
    await live.act(() => api.start(runId), 'Игра запущена')
  }

  async function finishRun() {
    if (!confirm('Завершить игру для всех команд? Это нельзя отменить.')) return
    await live.act(() => api.finish(runId), 'Игра завершена')
  }

  async function send(e: SubmitEvent) {
    e.preventDefault()
    if (await live.act(() => api.broadcast(runId, message.trim()), 'Сообщение отправлено')) message = ''
  }

  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text)
      live.note('ok', 'Скопировано')
    } catch {
      live.note('info', text)
    }
  }

  // Для подсказки в шапке: ошибки 401 — выйти.
  $effect(() => {
    api.run(runId).catch((e) => e instanceof ApiError && e.status === 401 && onUnauthorized())
  })
</script>

<div class="wrap">
  {#if !run || !d}
    {#if live.error}<div class="notice">{live.error}</div>{:else}<div class="spinner"></div>{/if}
  {:else}
    <section class="head card">
      <div class="row wrap">
        <div class="grow">
          <h1>{run.title}</h1>
          <div class="row wrap small">
            <span class="badge {RUN_STATUS[d.run_status][1]}">{RUN_STATUS[d.run_status][0]}</span>
            <span class="muted">Код игры</span>
            <strong class="mono code">{run.public_code}</strong>
            <button class="ghost small" onclick={() => copy(gameUrl)}>Скопировать ссылку</button>
            <button class="ghost small" onclick={() => (showQR = !showQR)}>{showQR ? 'Скрыть QR' : 'QR для афиши'}</button>
            <a class="small" href={gameUrl} target="_blank" rel="noopener">Открыть как игрок ↗</a>
            <span class="muted" title={live.streaming ? 'Обновления приходят сразу' : 'Нет потока обновлений — опрос раз в 30 с'}>
              {live.streaming ? '● онлайн' : '○ переподключение'}
            </span>
          </div>
        </div>
        {#if d.run_status === 'pending'}
          <button onclick={startRun} disabled={d.teams.length === 0}>Старт игры</button>
        {:else if d.run_status === 'running'}
          <button class="danger" onclick={finishRun}>Завершить игру</button>
        {/if}
      </div>
      {#if showQR}
        <div class="qrbox">
          <QR text={gameUrl} />
          <div class="muted small center mono">{gameUrl}</div>
        </div>
      {/if}
      <ScenarioPanel {live} {runId} />
      {#if d.run_status !== 'finished'}
        <form class="row msgform" onsubmit={send}>
          <input class="grow" bind:value={message} placeholder="Сообщение всем командам (появится у игроков сверху)" maxlength="500" />
          <button class="secondary" disabled={!message.trim()}>Отправить всем</button>
        </form>
      {/if}
    </section>

    <SosBanner {live} {runId} onOpen={openChat} />

    {#if d.run_status === 'finished' && !d.results && tab !== 'table'}
      <div class="card finbanner row wrap">
        <span class="grow"><strong>Игра завершена.</strong> Проверьте таблицу и утвердите итоги — команды получат поздравления.</span>
        <button onclick={() => (tab = 'table')}>К итогам</button>
      </div>
    {/if}

    <nav class="tabs">
      <button class:active={tab === 'dash'} onclick={() => (tab = 'dash')}>Команды</button>
      <button class:active={tab === 'reg'} onclick={() => (tab = 'reg')}>Регистрация</button>
      <button class:active={tab === 'mod'} onclick={() => (tab = 'mod')}>
        Модерация{#if live.queue.length}<span class="counter">{live.queue.length}</span>{/if}
      </button>
      <button class:active={tab === 'comms'} onclick={() => (tab = 'comms')}>
        Связь{#if chatUnread && tab !== 'comms'}<span class="counter">{chatUnread}</span>{/if}
      </button>
      <button class:active={tab === 'table'} onclick={() => (tab = 'table')}>Таблица</button>
      <button class:active={tab === 'log'} onclick={() => (tab = 'log')}>Журнал</button>
    </nav>

    {#if tab === 'dash'}
      <Dashboard {live} onOpen={(id) => (teamId = id)} />
    {:else if tab === 'reg'}
      <Registration {live} {runId} code={run.public_code} onOpen={(id) => (teamId = id)} />
    {:else if tab === 'mod'}
      <Moderation {live} {runId} />
    {:else if tab === 'comms'}
      <Comms {live} {runId} bind:teamId={chatTeam} />
    {:else if tab === 'table'}
      <Standings {live} {runId} />
    {:else}
      <Journal {live} {runId} />
    {/if}
  {/if}
</div>

{#if team}
  <TeamDrawer {live} {runId} {team} code={run?.public_code ?? ''} onClose={() => (teamId = null)} />
{/if}

<div class="notes">
  {#each live.notes as n (n.id)}
    <div class="note {n.kind}">{n.text}</div>
  {/each}
</div>

<style>
  .head h1 {
    margin-bottom: 0.35rem;
  }
  .code {
    letter-spacing: 0.08em;
    font-size: 1.05rem;
  }
  .finbanner {
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .qrbox {
    margin-top: 1rem;
    max-width: 320px;
  }
  .msgform {
    margin-top: 0.9rem;
  }
</style>
