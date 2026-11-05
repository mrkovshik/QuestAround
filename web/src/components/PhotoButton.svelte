<script lang="ts">
  import type { Live, PhotoTarget } from '../lib/live.svelte'

  let { live, target, label, secondary = false }: { live: Live; target: PhotoTarget; label: string; secondary?: boolean } = $props()

  let input: HTMLInputElement | undefined = $state()
  let preview = $state<string | null>(null)
  let sending = $state(false)

  async function picked() {
    const file = input?.files?.[0]
    if (!file) return
    preview = URL.createObjectURL(file)
    sending = true
    try {
      await live.submitPhoto(file, target)
    } finally {
      sending = false
      if (preview) URL.revokeObjectURL(preview)
      preview = null
      if (input) input.value = ''
    }
  }
</script>

<div class="photo">
  {#if preview}
    <img src={preview} alt="" />
  {/if}
  <button type="button" class="block" class:secondary disabled={sending || live.busy} onclick={() => input?.click()}>
    {sending ? 'Отправляем…' : label}
  </button>
  <input bind:this={input} type="file" accept="image/*" capture="environment" hidden onchange={picked} />
</div>

<style>
  .photo img {
    width: 100%;
    max-height: 220px;
    object-fit: cover;
    border-radius: 10px;
    margin-bottom: 0.5rem;
    opacity: 0.7;
  }
</style>
