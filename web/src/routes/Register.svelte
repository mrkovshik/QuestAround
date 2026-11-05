<script lang="ts">
  import { api, ApiError, type GameInfo } from '../lib/api'
  import { setToken } from '../lib/session'
  import { router } from '../lib/router.svelte'

  let { code, invite, onAuthed }: { code: string; invite?: string; onAuthed: () => void } = $props()

  let info = $state<GameInfo | null>(null)
  let loadError = $state<string | null>(null)
  let mode = $state<'choose' | 'create' | 'join'>('choose')
  let busy = $state(false)
  let error = $state<string | null>(null)

  let teamName = $state('')
  let name = $state('')
  let contact = $state('')
  let inviteCode = $state('')

  $effect(() => {
    if (invite) {
      mode = 'join'
      inviteCode = invite
    }
  })

  $effect(() => {
    api
      .gameInfo(code)
      .then((i) => (info = i))
      .catch((e) => (loadError = e instanceof ApiError && e.status === 404 ? 'Игра с таким кодом не найдена' : 'Не удалось загрузить игру — проверьте связь'))
  })

  const solo = $derived(info?.participants.mode === 'solo')
  const canCreate = $derived(!!info && info.registration.open)

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    busy = true
    error = null
    try {
      const cred =
        mode === 'create'
          ? await api.createTeam(code, solo ? name : teamName, name, contact)
          : await api.join(code, inviteCode.trim(), name)
      setToken(code, cred.token)
      // Ссылка-приглашение больше не нужна в адресной строке.
      router.go(`/g/${encodeURIComponent(code)}`, true)
      onAuthed()
    } catch (err) {
      error = err instanceof ApiError ? err.message : 'Нет связи с сервером — попробуйте ещё раз'
    } finally {
      busy = false
    }
  }
</script>

<main class="page">
  {#if loadError}
    <div class="card">
      <h2>Упс</h2>
      <p>{loadError}</p>
      <button class="secondary" onclick={() => router.go('/')}>Ввести другой код</button>
    </div>
  {:else if !info}
    <div class="spinner"></div>
  {:else}
    <header class="head">
      <h1>{info.title}</h1>
      {#if info.description}<p class="muted task">{info.description}</p>{/if}
      <div class="row wrap">
        {#if info.status === 'running'}<span class="badge warn">Игра идёт</span>{/if}
        {#if info.status === 'finished'}<span class="badge bad">Игра завершена</span>{/if}
        {#if !solo}<span class="badge">Команды {info.participants.min_size}–{info.participants.max_size} чел.</span>{/if}
        {#if info.registration.max_teams}
          <span class="badge">Команд: {info.registration.teams} из {info.registration.max_teams}</span>
        {/if}
      </div>
    </header>

    {#if mode === 'choose'}
      <div class="stack">
        {#if canCreate}
          <button class="block" onclick={() => (mode = 'create')}>{solo ? 'Участвовать' : 'Создать команду'}</button>
        {:else if info.status === 'pending'}
          <div class="notice">Регистрация новых команд закрыта.</div>
        {/if}
        {#if !solo && info.status !== 'finished'}
          <button class="block secondary" onclick={() => (mode = 'join')}>У меня есть приглашение в команду</button>
        {/if}
      </div>
    {:else}
      <form class="card stack" onsubmit={submit}>
        <h2>{mode === 'create' ? (solo ? 'Регистрация' : 'Новая команда') : 'Вступить в команду'}</h2>
        {#if mode === 'create' && !solo}
          <div>
            <label for="team">Название команды</label>
            <input id="team" bind:value={teamName} maxlength="40" required />
          </div>
        {/if}
        {#if mode === 'join'}
          <div>
            <label for="inv">Код приглашения</label>
            <input id="inv" bind:value={inviteCode} autocapitalize="characters" autocomplete="off" required />
          </div>
        {/if}
        <div>
          <label for="name">Ваше имя</label>
          <input id="name" bind:value={name} maxlength="40" autocomplete="given-name" required />
        </div>
        {#if mode === 'create'}
          <div>
            <label for="contact">Telegram или телефон — для связи с ведущим (необязательно)</label>
            <input id="contact" bind:value={contact} maxlength="60" />
          </div>
        {/if}
        {#if error}<div class="notice">{error}</div>{/if}
        <button class="block" disabled={busy}>{busy ? 'Секунду…' : mode === 'create' ? 'Создать' : 'Вступить'}</button>
        <button type="button" class="block ghost" onclick={() => ((mode = 'choose'), (error = null))}>Назад</button>
      </form>
    {/if}
  {/if}
</main>

<style>
  .head {
    margin: 1.5rem 0 1.25rem;
  }
  #inv {
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }
</style>
