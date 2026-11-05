<script lang="ts">
  import type { AdminApi } from './adminApi'

  let { api, src, alt = '' }: { api: AdminApi; src: string; alt?: string } = $props()

  let url = $state<string | null>(null)
  let failed = $state(false)

  $effect(() => {
    let revoked = false
    let made: string | null = null
    failed = false
    api
      .mediaURL(src)
      .then((u) => {
        made = u
        if (revoked) URL.revokeObjectURL(u)
        else url = u
      })
      .catch(() => (failed = true))
    return () => {
      revoked = true
      if (made) URL.revokeObjectURL(made)
    }
  })
</script>

{#if url}
  <a href={url} target="_blank" rel="noopener"><img src={url} {alt} /></a>
{:else if failed}
  <div class="ph muted small">Не удалось загрузить фото</div>
{:else}
  <div class="ph"></div>
{/if}

<style>
  img,
  .ph {
    display: block;
    width: 100%;
    aspect-ratio: 4 / 3;
    object-fit: cover;
    border-radius: 10px;
    background: var(--bg);
  }
  .ph {
    display: grid;
    place-items: center;
  }
</style>
