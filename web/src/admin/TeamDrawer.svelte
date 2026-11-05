<script lang="ts">
  import { clock } from '../lib/format'
  import type { DashTeam } from './adminApi'
  import type { RunLive } from './runLive.svelte'
  import { between, hhmm, since, TEAM_STATUS, FINISH_REASON } from './time'

  let { live, runId, team, code, onClose }: { live: RunLive; runId: string; team: DashTeam; code: string; onClose: () => void } =
    $props()

  const api = $derived(live.api)
  const runStatus = $derived(live.dash?.run_status)
  const playing = $derived(team.status === 'playing')

  let answer = $state('')
  let reason = $state('')
  let timeDelta = $state('')
  let pointsDelta = $state('')
  let hintLevel = $state('')
  let hintIdx = $state(0)
  let newName = $state('')

  // Дашборд обновляется по SSE, и team приходит новым объектом —
  // формы сбрасываем только при переходе к другой команде.
  let shownId = ''
  $effect(() => {
    if (team.id !== shownId) {
      shownId = team.id
      answer = reason = timeDelta = pointsDelta = newName = ''
      hintIdx = 0
    }
    const withHints = team.current.filter((l) => l.hints_total > 0)
    if (!withHints.some((l) => l.level_id === hintLevel)) {
      hintLevel = withHints[0]?.level_id ?? ''
      hintIdx = 0
    }
  })

  const hintLevelInfo = $derived(team.current.find((l) => l.level_id === hintLevel))

  function needReason(): boolean {
    if (reason.trim()) return true
    live.note('bad', 'Укажите причину — она попадёт в журнал')
    return false
  }

  async function sendAnswer(e: SubmitEvent) {
    e.preventDefault()
    const r = await live.act(() => api.answerForTeam(runId, team.id, answer.trim()))
    if (r) {
      const text: Record<string, string> = { correct: 'Верно — засчитано', wrong: 'Неверный код', duplicate: 'Уже было засчитано' }
      live.note(r.result === 'correct' ? 'ok' : 'bad', text[r.result] ?? r.result)
      if (r.result !== 'wrong') answer = ''
    }
  }

  async function completeLevel(levelId: string, title: string) {
    if (!needReason()) return
    if (!confirm(`Засчитать уровень «${title}» команде «${team.name}»?`)) return
    await live.act(() => api.override(runId, team.id, { action: 'complete_level', level_id: levelId, reason: reason.trim() }), 'Уровень засчитан')
  }

  async function openHint() {
    if (!needReason()) return
    await live.act(
      () => api.override(runId, team.id, { action: 'open_hint', level_id: hintLevel, hint_idx: hintIdx, reason: reason.trim() }),
      'Подсказка открыта',
    )
  }

  async function adjustTime() {
    if (!needReason()) return
    const v = timeDelta.trim().replace(/\s/g, '')
    if (!/^[+-]?\d+(\.\d+)?(m|s|h)$|^[+-]?(\d+h)?(\d+m)?(\d+s)?$/.test(v) || v === '' || v === '+' || v === '-') {
      live.note('bad', 'Формат времени: 5m, -2m, 1m30s')
      return
    }
    const r = await live.act(() => api.override(runId, team.id, { action: 'adjust_time', time: v.replace(/^\+/, ''), reason: reason.trim() }), 'Время изменено')
    if (r) timeDelta = ''
  }

  async function adjustPoints() {
    if (!needReason()) return
    const n = Number(pointsDelta)
    if (!Number.isInteger(n) || n === 0) {
      live.note('bad', 'Баллы — целое число, например 5 или -3')
      return
    }
    const r = await live.act(() => api.override(runId, team.id, { action: 'adjust_points', points: n, reason: reason.trim() }), 'Баллы изменены')
    if (r) pointsDelta = ''
  }

  async function finishTeam() {
    if (!needReason()) return
    if (!confirm(`Остановить игру для команды «${team.name}»?`)) return
    await live.act(() => api.override(runId, team.id, { action: 'finish_team', reason: reason.trim() }), 'Команда остановлена')
  }

  async function rename(e: SubmitEvent) {
    e.preventDefault()
    if (await live.act(() => api.renameTeam(runId, team.id, newName.trim()), 'Переименовано')) newName = ''
  }

  const invite = $derived(team.invite_code ? `${location.origin}/g/${code}/join/${team.invite_code}` : '')

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
  }
</script>

<svelte:window onkeydown={onKey} />

<div class="backdrop" onclick={onClose} role="presentation"></div>
<aside class="drawer" aria-label="Команда {team.name}">
  <header class="row">
    <div class="grow">
      <h2>{team.name}</h2>
      <span class="badge {TEAM_STATUS[team.status][1]}">{TEAM_STATUS[team.status][0]}</span>
      {#if team.finish_reason}<span class="muted small">— {FINISH_REASON[team.finish_reason] ?? team.finish_reason}</span>{/if}
    </div>
    <button class="ghost" onclick={onClose} aria-label="Закрыть">✕</button>
  </header>

  <section class="facts">
    <div><span class="muted">Баллы</span> <strong>{team.points}</strong></div>
    <div><span class="muted">Уровней</span> <strong>{team.levels_done}</strong></div>
    <div><span class="muted">Время</span> <strong>{team.started_at ? clock(team.elapsed_sec) : '—'}</strong></div>
    <div><span class="muted">Штраф</span> <strong>{team.penalty_sec ? clock(Math.abs(team.penalty_sec)) : '—'}</strong>{team.penalty_sec < 0 ? ' (−)' : ''}</div>
    <div><span class="muted">Старт</span> <strong>{hhmm(team.started_at ?? team.scheduled_at)}</strong></div>
    <div><span class="muted">Финиш</span> <strong>{hhmm(team.finished_at)}</strong></div>
  </section>

  <section>
    <h3>Состав</h3>
    <ul class="plain">
      {#each team.players as p (p.id)}
        <li>{p.name} {#if p.captain}<span class="badge">капитан</span>{/if}</li>
      {:else}
        <li class="muted">Пока никого — отправьте ссылку-приглашение.</li>
      {/each}
    </ul>
    {#if team.contact}<p class="small">Контакт капитана: <strong>{team.contact}</strong></p>{/if}
    {#if invite && team.status !== 'finished'}
      <p class="small">Приглашение: <span class="mono">{team.invite_code}</span> · <span class="mono link">{invite}</span></p>
    {/if}
  </section>

  {#if team.levels.length}
    <section>
      <h3>Уровни</h3>
      <table class="grid">
        <thead><tr><th>Уровень</th><th>Открыт</th><th class="num">Время</th><th class="num">Подск.</th><th></th></tr></thead>
        <tbody>
          {#each team.levels as l (l.level_id)}
            <tr>
              <td>
                {l.title}
                {#if l.required > 1}<div class="muted small">секторы {l.solved}/{l.required}</div>{/if}
              </td>
              <td>{hhmm(l.opened_at)}</td>
              <td class="num">{l.completed_at ? between(l.opened_at, l.completed_at) : clock(since(l.opened_at, live.now))}</td>
              <td class="num">{l.hints_opened}/{l.hints_total}</td>
              <td class="num">
                {#if !l.completed_at && playing}
                  <button class="small secondary" onclick={() => completeLevel(l.level_id, l.title)}>Засчитать</button>
                {:else if l.completed_at}
                  <span class="badge ok">✓</span>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </section>
  {/if}

  {#if playing}
    <section class="stack">
      <h3>Ввести код за команду</h3>
      <p class="muted small">Если у команды нет связи: продиктовали код по телефону — вводите здесь.</p>
      <form class="row" onsubmit={sendAnswer}>
        <input class="grow" bind:value={answer} placeholder="Код" autocomplete="off" />
        <button disabled={!answer.trim()}>Отправить</button>
      </form>
    </section>
  {/if}

  {#if team.status !== 'withdrawn'}
    <section class="stack">
      <h3>Ручные действия</h3>
      <div>
        <label for="reason">Причина (обязательно, попадёт в журнал)</label>
        <input id="reason" bind:value={reason} placeholder="Например: сидели без связи 5 минут" />
      </div>

      {#if playing && team.current.some((l) => l.hints_total > 0)}
        <div class="row wrap">
          <select bind:value={hintLevel}>
            {#each team.current.filter((l) => l.hints_total > 0) as l (l.level_id)}<option value={l.level_id}>{l.title}</option>{/each}
          </select>
          <select bind:value={hintIdx}>
            {#each Array(hintLevelInfo?.hints_total ?? 0) as _, i (i)}<option value={i}>Подсказка {i + 1}</option>{/each}
          </select>
          <button class="secondary" disabled={!hintLevelInfo?.hints_total} onclick={openHint}>Открыть без штрафа</button>
        </div>
      {/if}

      {#if team.started_at}
        <div class="row wrap">
          <input class="short" bind:value={timeDelta} placeholder="+5m / -2m" />
          <button class="secondary" onclick={adjustTime} disabled={!timeDelta.trim()}>Изменить время</button>
          <input class="short" bind:value={pointsDelta} placeholder="+5 / -3" inputmode="numeric" />
          <button class="secondary" onclick={adjustPoints} disabled={!pointsDelta.trim()}>Изменить баллы</button>
        </div>
      {/if}

      <div class="row wrap">
        {#if (team.status === 'registered' || team.status === 'scheduled') && runStatus === 'running'}
          <button onclick={() => live.act(() => api.startTeam(runId, team.id), 'Команда стартовала')}>Стартовать сейчас</button>
        {/if}
        {#if playing}
          <button class="danger" onclick={finishTeam}>Остановить игру команды</button>
        {/if}
        {#if runStatus === 'pending'}
          <button
            class="danger"
            onclick={() => confirm(`Снять команду «${team.name}» с игры?`) && live.act(() => api.withdrawTeam(runId, team.id), 'Команда снята')}
            >Снять с игры</button
          >
        {/if}
      </div>

      <form class="row" onsubmit={rename}>
        <input class="grow" bind:value={newName} placeholder="Новое название команды" maxlength="40" />
        <button class="secondary" disabled={!newName.trim()}>Переименовать</button>
      </form>
    </section>
  {/if}
</aside>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgb(0 0 0 / 0.25);
    z-index: 20;
  }
  .drawer {
    position: fixed;
    top: 0;
    right: 0;
    bottom: 0;
    width: min(560px, 100vw);
    overflow-y: auto;
    background: var(--card);
    border-left: 1px solid var(--line);
    z-index: 21;
    padding: 1.1rem 1.25rem 2rem;
    box-shadow: -10px 0 30px rgb(0 0 0 / 0.15);
  }
  .drawer section {
    margin-top: 1.25rem;
  }
  .facts {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 0.5rem 1rem;
    background: var(--bg);
    border-radius: 10px;
    padding: 0.75rem;
  }
  .facts div {
    display: flex;
    flex-direction: column;
  }
  .facts .muted {
    font-size: 0.78rem;
  }
  .plain {
    list-style: none;
    padding: 0;
    margin: 0 0 0.5rem;
  }
  .plain li {
    padding: 0.2rem 0;
  }
  .short {
    width: 7.5rem;
  }
  .link {
    word-break: break-all;
  }
</style>
