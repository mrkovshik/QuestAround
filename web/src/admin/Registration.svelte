<script lang="ts">
  import type { RunLive } from './runLive.svelte'
  import { TEAM_STATUS } from './time'

  let { live, runId, code, onOpen }: { live: RunLive; runId: string; code: string; onOpen: (teamId: string) => void } = $props()

  const d = $derived(live.dash!)
  const api = $derived(live.api)
  const pending = $derived(d.run_status === 'pending')

  let open = $state(true)
  let maxTeams = $state(0)
  // Подхватываем значения с сервера, только когда они там изменились,
  // иначе обновление по SSE затирало бы то, что ведущий сейчас вводит.
  let seen = ''
  $effect(() => {
    const key = `${d.registration.open}/${d.registration.max_teams}`
    if (key !== seen) {
      seen = key
      open = d.registration.open
      maxTeams = d.registration.max_teams
    }
  })

  let name = $state('')
  let contact = $state('')
  let created = $state<{ name: string; url: string } | null>(null)

  async function save() {
    await live.act(() => api.setRegistration(runId, open, Math.max(0, Math.floor(maxTeams || 0))), 'Настройки регистрации сохранены')
  }

  async function addTeam(e: SubmitEvent) {
    e.preventDefault()
    const r = await live.act(() => api.createTeam(runId, name.trim(), contact.trim()))
    if (r) {
      created = { name: name.trim(), url: `${location.origin}/g/${code}/join/${r.invite_code}` }
      name = contact = ''
    }
  }

  const link = (invite?: string) => (invite ? `${location.origin}/g/${code}/join/${invite}` : '')
</script>

<div class="cols">
  <section class="card stack">
    <h2>Настройки</h2>
    {#if !pending}
      <p class="muted">Игра уже идёт — новые команды не регистрируются, но игроки могут вступать в существующие по приглашениям.</p>
    {:else}
      <label class="row check"><input type="checkbox" bind:checked={open} /> Регистрация открыта</label>
      <div>
        <label for="max">Максимум команд (0 — без ограничения)</label>
        <input id="max" type="number" min="0" bind:value={maxTeams} />
      </div>
      <button onclick={save}>Сохранить</button>
      <p class="muted small">Ведущему лимит не мешает: команды, добавленные здесь вручную, проходят всегда.</p>
    {/if}
  </section>

  {#if pending}
    <section class="card">
      <form class="stack" onsubmit={addTeam}>
        <h2>Добавить команду вручную</h2>
        <p class="muted small">Для корпоративов и тех, кто регистрируется у вас лично. Капитаном станет первый, кто вступит по ссылке.</p>
        <input bind:value={name} placeholder="Название команды" maxlength="40" required />
        <input bind:value={contact} placeholder="Контакт (необязательно)" maxlength="60" />
        <button disabled={!name.trim()}>Добавить</button>
      </form>
      {#if created}
        <div class="notice small created">
          Команда «{created.name}» создана. Ссылка для вступления:
          <div class="mono link">{created.url}</div>
        </div>
      {/if}
    </section>
  {/if}
</div>

<section class="card">
  <h2>Команды ({d.teams.length}{d.registration.max_teams ? ` из ${d.registration.max_teams}` : ''})</h2>
  {#if d.teams.length === 0}
    <p class="muted">Пока никого.</p>
  {:else}
    <table class="grid">
      <thead><tr><th>Команда</th><th>Статус</th><th>Состав</th><th>Контакт</th><th>Приглашение</th></tr></thead>
      <tbody>
        {#each d.teams as t (t.id)}
          <tr class="click" onclick={() => onOpen(t.id)}>
            <td><strong>{t.name}</strong></td>
            <td><span class="badge {TEAM_STATUS[t.status][1]}">{TEAM_STATUS[t.status][0]}</span></td>
            <td>{t.players.map((p) => p.name + (p.captain ? ' ★' : '')).join(', ') || '—'}</td>
            <td>{t.contact ?? '—'}</td>
            <td class="mono small" title={link(t.invite_code)}>{t.invite_code ?? '—'}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
  {#if d.withdrawn?.length}
    <p class="muted small">Снялись: {d.withdrawn.map((t) => t.name).join(', ')}</p>
  {/if}
</section>

<style>
  .check {
    color: var(--text);
    font-size: 1rem;
  }
  .check input {
    width: auto;
  }
  .created {
    margin-top: 0.75rem;
  }
  .link {
    word-break: break-all;
    margin-top: 0.3rem;
  }
</style>
