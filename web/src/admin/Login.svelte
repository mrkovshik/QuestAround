<script lang="ts">
  import { ApiError } from '../lib/api'
  import { AdminApi, adminToken } from './adminApi'

  let { onLogin }: { onLogin: (token: string) => void } = $props()

  let token = $state('')
  let error = $state<string | null>(null)
  let busy = $state(false)

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    busy = true
    error = null
    try {
      await new AdminApi(token.trim()).runs()
      adminToken.set(token.trim())
      onLogin(token.trim())
    } catch (err) {
      error = err instanceof ApiError && err.status === 401 ? 'Неверный токен' : 'Нет связи с сервером'
    } finally {
      busy = false
    }
  }
</script>

<main class="page">
  <form class="card stack login" onsubmit={submit}>
    <h1>Вход для ведущего</h1>
    <div>
      <label for="tok">Токен ведущего (QUESTAROUND_ADMIN_TOKEN)</label>
      <input id="tok" type="password" bind:value={token} autocomplete="current-password" required />
    </div>
    {#if error}<div class="notice">{error}</div>{/if}
    <button class="block" disabled={busy || !token.trim()}>Войти</button>
  </form>
</main>

<style>
  .login {
    margin-top: 15vh;
  }
</style>
