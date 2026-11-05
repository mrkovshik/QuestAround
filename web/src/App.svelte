<script lang="ts">
  import { router } from './lib/router.svelte'
  import { getToken } from './lib/session'
  import Home from './routes/Home.svelte'
  import Register from './routes/Register.svelte'
  import Game from './routes/Game.svelte'

  // Пересчитывается при смене маршрута и после входа/выхода.
  let authVersion = $state(0)
  const token = $derived.by(() => {
    authVersion
    const r = router.route
    return r.page === 'game' ? getToken(r.code) : null
  })
</script>

{#if router.route.page === 'home'}
  <Home />
{:else if token}
  {#key router.route.code + token}
    <Game code={router.route.code} {token} onLogout={() => authVersion++} />
  {/key}
{:else}
  <Register code={router.route.code} invite={router.route.invite} onAuthed={() => authVersion++} />
{/if}
