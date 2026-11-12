<script lang="ts">
  // Картинка, которая по нажатию открывается на весь экран (закрыть — нажатием или Esc).
  let { src, alt = '', cls = '' }: { src: string; alt?: string; cls?: string } = $props()
  let open = $state(false)

  $effect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && (open = false)
    addEventListener('keydown', onKey)
    return () => removeEventListener('keydown', onKey)
  })
</script>

<button type="button" class="pic {cls}" onclick={() => (open = true)} aria-label="Открыть крупно">
  <img {src} {alt} loading="lazy" />
</button>
{#if open}
  <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
  <div class="lightbox" onclick={() => (open = false)}>
    <img {src} {alt} />
    <a class="dl" href={src} target="_blank" rel="noopener" onclick={(e) => e.stopPropagation()}>Открыть файл ↗</a>
  </div>
{/if}

<style>
  .pic {
    display: block;
    padding: 0;
    border: 0;
    background: none;
    width: 100%;
    min-height: 0;
    cursor: zoom-in;
  }
  .pic img {
    display: block;
    width: 100%;
    border-radius: 10px;
  }
  .pic.thumb img {
    aspect-ratio: 1;
    object-fit: cover;
  }
  .pic.inline {
    width: auto;
    max-width: 100%;
  }
  .pic.inline img {
    max-height: 240px;
    width: auto;
    max-width: 100%;
  }
  .lightbox {
    position: fixed;
    inset: 0;
    z-index: 1000;
    background: rgba(0, 0, 0, 0.9);
    display: grid;
    place-items: center;
    cursor: zoom-out;
    padding: 1rem;
  }
  .lightbox img {
    max-width: 100%;
    max-height: calc(100vh - 4rem);
    object-fit: contain;
  }
  .dl {
    position: absolute;
    bottom: 1rem;
    color: #fff;
    font-size: 0.9rem;
  }
</style>
