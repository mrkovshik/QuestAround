<script lang="ts">
  import './admin.css'
  import { router } from '../lib/router.svelte'
  import { AdminApi, adminToken } from './adminApi'
  import Login from './Login.svelte'
  import Runs from './Runs.svelte'
  import RunPage from './RunPage.svelte'

  let token = $state(adminToken.get())
  const api = $derived(token ? new AdminApi(token) : null)
  const runId = $derived(router.route.page === 'admin' ? router.route.runId : undefined)

  function logout() {
    adminToken.clear()
    token = null
  }

  $effect(() => {
    document.title = 'QuestAround — ведущий'
  })
</script>

<div class="admin">
  {#if !api}
    <Login onLogin={(t) => (token = t)} />
  {:else}
    <header class="topbar">
      <a class="brand" href="/admin" onclick={(e) => (e.preventDefault(), router.go('/admin'))}>QuestAround · ведущий</a>
      <span class="grow"></span>
      <button class="ghost small" onclick={logout}>Выйти</button>
    </header>
    {#if runId}
      {#key runId}
        <RunPage {api} {runId} onUnauthorized={logout} />
      {/key}
    {:else}
      <Runs {api} onUnauthorized={logout} />
    {/if}
  {/if}
</div>
