<script lang="ts">
  // Перенос в VK-бот: сайт недоступен («белые списки»), а VK работает.
  // Игрок отправляет код сообществу и продолжает тем же игроком.
  import { transferCode } from '../lib/transfer'

  let { token, vkUrl, open = false }: { token: string; vkUrl: string; open?: boolean } = $props()

  let manual = $state(false)
  const expanded = $derived(open || manual)
  let code = $state<string | null>(null)
  let copied = $state(false)

  $effect(() => {
    transferCode(token).then((c) => (code = c))
  })

  async function copyAndOpen() {
    if (code) {
      try {
        await navigator.clipboard.writeText(code)
        copied = true
      } catch {
        copied = false // не дали доступ к буферу — код виден на экране
      }
    }
    window.open(vkUrl, '_blank', 'noopener')
  }
</script>

<section class="card vk">
  {#if !expanded}
    <button class="ghost block" onclick={() => (manual = true)}>Сайт не открывается? Играть через VK</button>
  {:else}
    <h3>Играть через VK</h3>
    <p class="small muted">
      VK работает даже при «белых списках». Вы останетесь тем же игроком в своей команде — прогресс общий.
    </p>
    {#if code}
      <ol class="small">
        <li>Нажмите кнопку — код скопируется, откроется сообщество.</li>
        <li>Отправьте код сообщением. Дальше задания придут в VK.</li>
      </ol>
      <div class="code" aria-label="Код переноса">{code}</div>
      <button class="block" onclick={copyAndOpen}>{copied ? 'Код скопирован — открыть VK' : 'Скопировать код и открыть VK'}</button>
      <p class="small muted">Если кнопка не сработала — откройте VK, найдите сообщество игры и отправьте код вручную. Ведущий тоже может продиктовать ваш код.</p>
    {:else}
      <p class="small">Код не посчитать на этом телефоне — попросите ведущего продиктовать его и отправьте сообществу: <a href={vkUrl} target="_blank" rel="noopener">открыть VK</a>.</p>
    {/if}
  {/if}
</section>

<style>
  .vk h3 {
    margin: 0 0 0.25rem;
  }
  .vk ol {
    margin: 0.5rem 0;
    padding-left: 1.2rem;
  }
  .code {
    font-size: 1.6rem;
    font-weight: 800;
    letter-spacing: 0.06em;
    text-align: center;
    font-variant-numeric: tabular-nums;
    margin: 0.5rem 0 0.75rem;
    user-select: all;
  }
</style>
