<script lang="ts">
  import { clock } from '../lib/format'
  import type { DashTeam } from './adminApi'
  import type { RunLive } from './runLive.svelte'
  import { ago, hhmm, since, TEAM_STATUS, FINISH_REASON } from './time'

  let { live, onOpen }: { live: RunLive; onOpen: (teamId: string) => void } = $props()

  const d = $derived(live.dash!)

  // Пороги «залипания»: на уровне дольше 25 минут, тишина дольше 10 минут.
  const STUCK_ON_LEVEL = 25 * 60
  const SILENT = 10 * 60

  function onLevel(t: DashTeam): number {
    const c = t.current[0]
    return c ? since(c.opened_at, live.now) : 0
  }

  function elapsed(t: DashTeam): number {
    // На сервере время посчитано на момент загрузки; для играющих — докручиваем.
    if (t.status !== 'playing' || !t.started_at) return t.elapsed_sec
    return since(t.started_at, live.now) + t.penalty_sec
  }

  let sort = $state<'slot' | 'progress' | 'stuck'>('slot')
  const teams = $derived.by(() => {
    const list = [...d.teams]
    if (sort === 'progress') list.sort((a, b) => b.levels_done - a.levels_done || b.points - a.points)
    if (sort === 'stuck') list.sort((a, b) => onLevel(b) - onLevel(a))
    return list
  })

  const counts = $derived({
    playing: d.teams.filter((t) => t.status === 'playing').length,
    finished: d.teams.filter((t) => t.status === 'finished').length,
    waiting: d.teams.filter((t) => t.status === 'registered' || t.status === 'scheduled').length,
  })
</script>

<section class="card">
  <div class="row wrap summary">
    <span class="grow">
      <strong>{d.teams.length}</strong> команд ·
      играют <strong>{counts.playing}</strong> · финишировали <strong>{counts.finished}</strong>
      {#if counts.waiting} · ждут старта <strong>{counts.waiting}</strong>{/if}
      · уровней в игре: {d.levels_total}
    </span>
    <label class="row sortby" for="sort">Порядок
      <select id="sort" bind:value={sort}>
        <option value="slot">как стартовали</option>
        <option value="progress">по прогрессу</option>
        <option value="stuck">кто дольше на уровне</option>
      </select>
    </label>
  </div>

  {#if d.teams.length === 0}
    <p class="muted">Команд пока нет. Ссылку на регистрацию — во вкладке «Регистрация» или QR для афиши в шапке.</p>
  {:else}
    <div class="scroll">
      <table class="grid">
        <thead>
          <tr>
            <th>Команда</th>
            <th>Статус</th>
            <th>Сейчас</th>
            <th class="num">На уровне</th>
            <th class="num">Прогресс</th>
            <th class="num">Подсказки</th>
            <th class="num">Баллы</th>
            <th class="num">Время</th>
            <th>Активность</th>
          </tr>
        </thead>
        <tbody>
          {#each teams as t (t.id)}
            {@const cur = t.current[0]}
            {@const lvl = onLevel(t)}
            {@const quiet = t.status === 'playing' && since(t.last_activity, live.now) > SILENT}
            <tr class="click" onclick={() => onOpen(t.id)}>
              <td>
                <strong>{t.name}</strong>
                <div class="muted small">{t.players.length} чел.{#if t.contact} · {t.contact}{/if}</div>
              </td>
              <td>
                <span class="badge {TEAM_STATUS[t.status][1]}">{TEAM_STATUS[t.status][0]}</span>
                {#if t.status === 'scheduled'}<div class="muted small">в {hhmm(t.scheduled_at)}</div>{/if}
                {#if t.finish_reason}<div class="muted small">{FINISH_REASON[t.finish_reason] ?? t.finish_reason}</div>{/if}
              </td>
              <td>
                {#if t.current.length > 1}
                  {t.current.length} уровней открыто
                {:else if cur}
                  {cur.title}
                {:else}
                  <span class="muted">—</span>
                {/if}
              </td>
              <td class="num" class:hot={lvl > STUCK_ON_LEVEL}>{cur ? clock(lvl) : '—'}</td>
              <td class="num">
                {t.levels_done}/{d.levels_total}
                {#if cur && cur.required > 1}<div class="muted small">секторы {cur.solved}/{cur.required}</div>{/if}
              </td>
              <td class="num">{t.levels.reduce((n, l) => n + l.hints_opened, 0)}</td>
              <td class="num"><strong>{t.points}</strong></td>
              <td class="num">
                {t.started_at ? clock(elapsed(t)) : '—'}
                {#if t.penalty_sec}<div class="small warm">{t.penalty_sec > 0 ? '+' : ''}{clock(Math.abs(t.penalty_sec))} штраф</div>{/if}
              </td>
              <td class:warm={quiet}>{t.status === 'playing' ? ago(t.last_activity, live.now) : '—'}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    <p class="muted small legend">
      Красным — команда на уровне дольше 25 минут, оранжевым — нет активности больше 10 минут. Клик по строке — карточка команды
      и ручные действия.
    </p>
  {/if}
</section>

<style>
  .summary {
    margin-bottom: 0.75rem;
  }
  .sortby {
    margin: 0;
    gap: 0.4rem;
    color: var(--muted);
  }
  .scroll {
    overflow-x: auto;
  }
  .legend {
    margin: 0.75rem 0 0;
  }
</style>
