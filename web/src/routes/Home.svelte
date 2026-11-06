<script lang="ts">
  import { router } from '../lib/router.svelte'

  let code = $state('')

  function go(e: SubmitEvent) {
    e.preventDefault()
    const c = code.trim().toUpperCase()
    if (c) router.go(`/g/${encodeURIComponent(c)}`)
  }
</script>

<main class="page">
  <div class="hero">
    <img src="/icon.svg" alt="" width="72" height="72" />
    <h1>QuestAround</h1>
    <p class="muted">Городские игры и квесты. Введите код игры от организатора или откройте ссылку-приглашение.</p>
  </div>
  <form class="card stack" onsubmit={go}>
    <div>
      <label for="code">Код игры</label>
      <input id="code" bind:value={code} autocomplete="off" autocapitalize="characters" placeholder="Например, 6U9VKF" maxlength="12" />
    </div>
    <button class="block" disabled={!code.trim()}>Продолжить</button>
  </form>
</main>

<style>
  .hero {
    text-align: center;
    margin: 2.5rem 0 1.5rem;
  }
  .hero img {
    border-radius: 18px;
    margin-bottom: 0.75rem;
  }
  input {
    text-transform: uppercase;
    letter-spacing: 0.1em;
    font-weight: 600;
  }
</style>
