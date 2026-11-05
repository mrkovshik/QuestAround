<script lang="ts">
  import { api } from '../lib/api'
  import type { Live } from '../lib/live.svelte'
  import QR from './QR.svelte'

  let { live, logout }: { live: Live; logout: () => void } = $props()

  const s = $derived(live.state!)
  const team = $derived(s.team)
  const captain = $derived(s.is_captain)
  const preStart = $derived(s.run_status === 'pending')
  const inviteUrl = $derived(team.invite_code ? `${location.origin}/g/${encodeURIComponent(live.code)}/join/${team.invite_code}` : '')

  let renaming = $state(false)
  let newName = $state('')
  let showQR = $state(false)

  async function share() {
    const text = `Вступай в команду «${team.name}»`
    if (navigator.share) {
      try {
        await navigator.share({ title: s.title, text, url: inviteUrl })
        return
      } catch {
        /* отменили — скопируем */
      }
    }
    try {
      await navigator.clipboard.writeText(inviteUrl)
      live.toast('ok', 'Ссылка скопирована')
    } catch {
      live.toast('info', inviteUrl, 8000)
    }
  }

  async function rename(e: SubmitEvent) {
    e.preventDefault()
    if (await live.teamAction(() => api.renameTeam(live.token, newName), 'Команда переименована')) renaming = false
  }
</script>

<section class="card">
  <div class="row">
    {#if renaming}
      <form class="row grow" onsubmit={rename}>
        <input class="grow" bind:value={newName} maxlength="40" />
        <button disabled={!newName.trim()}>OK</button>
        <button type="button" class="ghost" onclick={() => (renaming = false)}>Отмена</button>
      </form>
    {:else}
      <h2 class="grow">{team.name}</h2>
      {#if captain}
        <button class="ghost small" onclick={() => ((newName = team.name), (renaming = true))}>Переименовать</button>
      {/if}
    {/if}
  </div>

  <ul class="players">
    {#each team.players as p (p.id)}
      <li class="row">
        <span class="grow">
          {p.name}
          {#if p.id === s.player_id}<span class="muted small">(вы)</span>{/if}
        </span>
        {#if p.captain}<span class="badge">капитан</span>{/if}
        {#if captain && !p.captain}
          <button
            class="ghost small"
            onclick={() => confirm(`Сделать капитаном: ${p.name}?`) && live.teamAction(() => api.transferCaptain(live.token, p.id), 'Капитан передан')}
            >Капитан</button
          >
          <button
            class="ghost small del"
            onclick={() => confirm(`Убрать из команды: ${p.name}?`) && live.teamAction(() => api.removePlayer(live.token, p.id))}
            aria-label="Убрать">✕</button
          >
        {/if}
      </li>
    {/each}
  </ul>
</section>

{#if team.invite_code && team.status !== 'finished'}
  <section class="card stack">
    <h3>Пригласить в команду</h3>
    <p class="muted small">Код приглашения: <strong class="code">{team.invite_code}</strong></p>
    <div class="row">
      <button class="grow" onclick={share}>Поделиться ссылкой</button>
      <button class="secondary" onclick={() => (showQR = !showQR)}>{showQR ? 'Скрыть QR' : 'QR-код'}</button>
    </div>
    {#if showQR}
      <QR text={inviteUrl} />
    {/if}
    {#if captain}
      <button
        class="ghost small"
        onclick={() => confirm('Старая ссылка перестанет работать. Продолжить?') && live.teamAction(() => api.rotateInvite(live.token), 'Новая ссылка готова')}
        >Сменить ссылку-приглашение</button
      >
    {/if}
  </section>
{/if}

<section class="card stack">
  {#if captain && preStart}
    <button class="danger block" onclick={() => confirm('Снять команду с игры? Отменить нельзя.') && live.teamAction(() => api.withdraw(live.token))}>
      Снять команду с игры
    </button>
  {/if}
  <button class="ghost block" onclick={() => confirm('Выйти на этом устройстве? Чтобы вернуться, понадобится новое приглашение.') && logout()}>
    Выйти на этом устройстве
  </button>
</section>

<style>
  .players {
    list-style: none;
    padding: 0;
    margin: 0.5rem 0 0;
  }
  .players li {
    padding: 0.45rem 0;
    border-top: 1px solid var(--line);
    min-height: 44px;
  }
  .del {
    color: var(--bad);
  }
  .code {
    letter-spacing: 0.1em;
    color: var(--text);
  }
</style>
