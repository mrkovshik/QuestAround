<script lang="ts">
  import { ApiError } from '../lib/api'
  import type { JournalEvent } from './adminApi'
  import type { RunLive } from './runLive.svelte'

  let { live, runId }: { live: RunLive; runId: string } = $props()

  let events = $state<JournalEvent[]>([])
  let error = $state<string | null>(null)
  let team = $state('')
  let hideTech = $state(true)

  const LABEL: Record<string, string> = {
    run_started: 'Старт игры',
    run_finished: 'Игра завершена',
    team_registered: 'Команда создана',
    player_joined: 'Игрок вступил',
    player_removed: 'Игрок удалён',
    captain_changed: 'Новый капитан',
    invite_rotated: 'Новое приглашение',
    team_renamed: 'Переименование',
    team_withdrawn: 'Команда снялась',
    registration_set: 'Настройки регистрации',
    team_start_scheduled: 'Старт по расписанию',
    route_assigned: 'Маршрут',
    team_started: 'Команда стартовала',
    level_opened: 'Уровень открыт',
    answer_submitted: 'Ответ',
    moderation_resolved: 'Модерация',
    hint_opened: 'Подсказка',
    bonus_completed: 'Бонус засчитан',
    level_completed: 'Уровень пройден',
    team_finished: 'Финиш команды',
    admin_override: 'Действие ведущего',
    leaderboard_visibility_set: 'Видимость таблицы',
    leaderboard_frozen: 'Таблица заморожена',
    broadcast: 'Сообщение всем',
  }
  const ACTION: Record<string, string> = {
    complete_level: 'засчитан уровень',
    open_hint: 'открыта подсказка',
    adjust_time: 'время',
    adjust_points: 'баллы',
    finish_team: 'остановлена команда',
  }
  const TECH = new Set(['route_assigned', 'team_start_scheduled', 'registration_set', 'invite_rotated'])

  async function load() {
    try {
      events = (await live.api.events(runId)).reverse()
      error = null
    } catch (e) {
      error = e instanceof ApiError ? e.message : 'Нет связи с сервером'
    }
  }

  $effect(() => {
    live.dash?.seq // перечитываем при изменениях
    load()
  })

  const teams = $derived(live.dash?.teams ?? [])
  const names = $derived(Object.fromEntries([...teams, ...(live.dash?.withdrawn ?? [])].map((t) => [t.id, t.name])))
  const players = $derived(
    Object.fromEntries([...teams, ...(live.dash?.withdrawn ?? [])].flatMap((t) => t.players.map((p) => [p.id, p.name]))),
  )
  const shown = $derived(events.filter((e) => (!team || e.team_id === team) && !(hideTech && TECH.has(e.type))))

  function describe(e: JournalEvent): string {
    const p = e.payload as Record<string, any>
    switch (e.type) {
      case 'answer_submitted': {
        const dist = typeof p.distance_m === 'number' ? `, до точки ${p.distance_m.toLocaleString('ru-RU')} м` : ''
        const what =
          p.kind === 'photo' ? 'фото' : p.kind === 'geo' ? `гео ±${Math.round(p.accuracy ?? 0)} м${dist}` : `«${p.value}»`
        const res: Record<string, string> = { correct: 'верно', wrong: 'неверно', duplicate: 'повтор', pending: 'на проверку', inaccurate: 'GPS неточный' }
        return `${what} → ${res[p.result] ?? p.result}${p.level_id ? ` (${p.level_id})` : ''}`
      }
      case 'hint_opened':
        return `${p.level_id} №${p.hint_idx + 1}, ${p.trigger === 'timer' ? 'по таймеру' : p.trigger === 'request' ? 'по запросу' : 'ведущий'}` +
          (p.penalty_time ? `, +${p.penalty_time / 6e10} мин` : '') + (p.penalty_points ? `, −${p.penalty_points} б.` : '')
      case 'admin_override':
        return `${ACTION[p.action] ?? p.action}${p.level_id ? ' ' + p.level_id : ''}${p.time ? ` ${p.time > 0 ? '+' : ''}${p.time / 6e10} мин` : ''}${p.points ? ` ${p.points > 0 ? '+' : ''}${p.points} б.` : ''} — ${p.reason}`
      case 'moderation_resolved':
        return `#${p.ref} ${p.approved ? 'принято' : 'отклонено'}${p.comment ? ': ' + p.comment : ''}`
      case 'level_opened':
      case 'level_completed':
        return p.level_id + (p.points ? ` (+${p.points})` : '')
      case 'bonus_completed':
        return `${p.bonus_id} (+${p.points})`
      case 'team_finished':
      case 'run_finished':
        return p.reason
      case 'player_joined':
        return (p.name ?? '') + (p.captain ? ' (капитан)' : '')
      case 'team_registered':
      case 'team_renamed':
        return p.name
      case 'broadcast':
        return p.text
      case 'leaderboard_visibility_set':
        return p.visible === null ? 'по сценарию' : p.visible ? 'показана' : 'скрыта'
      default:
        return ''
    }
  }

  const time = (iso: string) => new Date(iso).toLocaleTimeString('ru-RU')
</script>

<section class="card">
  <div class="row wrap head">
    <h2 class="grow">Журнал</h2>
    <select bind:value={team}>
      <option value="">Все команды</option>
      {#each teams as t (t.id)}<option value={t.id}>{t.name}</option>{/each}
    </select>
    <label class="row tech"><input type="checkbox" bind:checked={hideTech} /> скрыть служебные</label>
  </div>
  <p class="muted small">Всё, что происходило в игре, по порядку. Пригодится для спорных ситуаций.</p>
  {#if error}<div class="notice">{error}</div>{/if}
  <div class="scroll">
    <table class="grid">
      <thead><tr><th class="num">#</th><th>Время</th><th>Команда</th><th>Кто</th><th>Событие</th><th>Детали</th></tr></thead>
      <tbody>
        {#each shown as e (e.seq)}
          <tr>
            <td class="num muted">{e.seq}</td>
            <td class="mono">{time(e.at)}</td>
            <td>{e.team_id ? (names[e.team_id] ?? e.team_id) : '—'}</td>
            <td class="small">{e.player_id === 'host' ? 'ведущий' : e.player_id ? (players[e.player_id] ?? '') : ''}</td>
            <td>{LABEL[e.type] ?? e.type}</td>
            <td class="small">{describe(e)}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
</section>

<style>
  .head {
    margin-bottom: 0.25rem;
  }
  .tech {
    margin: 0;
    gap: 0.35rem;
    color: var(--muted);
  }
  .tech input {
    width: auto;
  }
  .scroll {
    max-height: 70vh;
    overflow: auto;
  }
</style>
