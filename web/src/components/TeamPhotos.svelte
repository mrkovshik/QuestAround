<script lang="ts">
  // Фото, которые прислала команда, со статусом проверки.
  import type { Photo } from '../lib/api'
  import Pic from './Pic.svelte'

  let { photos }: { photos: Photo[] } = $props()

  const label = { pending: ['на проверке', 'warn'], accepted: ['принято', 'ok'], rejected: ['отклонено', 'bad'] } as const
</script>

{#if photos.length}
  <section class="card">
    <h3>Наши фото · {photos.length}</h3>
    <div class="grid">
      {#each photos as p (p.seq)}
        <figure>
          <Pic src={p.url} cls="thumb" alt={p.title} />
          <figcaption class="small">
            <span class="badge {label[p.status][1]}">{label[p.status][0]}</span>
            <div class="muted">{p.title}</div>
            {#if p.comment}<div>«{p.comment}»</div>{/if}
          </figcaption>
        </figure>
      {/each}
    </div>
  </section>
{/if}

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(100px, 1fr));
    gap: 0.6rem;
  }
  figure {
    margin: 0;
  }
  figcaption {
    margin-top: 0.25rem;
    line-height: 1.25;
  }
</style>
