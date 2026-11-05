<script lang="ts">
  import type { RunLive } from './runLive.svelte'
  import { ago } from './time'
  import AuthImage from './AuthImage.svelte'

  let { live, runId }: { live: RunLive; runId: string } = $props()

  const api = $derived(live.api)
  let comments = $state<Record<number, string>>({})
  let busy = $state<number | null>(null)

  async function decide(seq: number, approved: boolean) {
    busy = seq
    const q = await live.act(() => api.moderate(runId, seq, approved, comments[seq] ?? ''), approved ? 'Принято' : 'Отклонено')
    if (q) live.queue = q
    busy = null
  }

  // Горячие клавиши для первой заявки: A — принять, R — отклонить.
  function onKey(e: KeyboardEvent) {
    const t = e.target as HTMLElement
    if (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || e.metaKey || e.ctrlKey || e.altKey) return
    const first = live.queue[0]
    if (!first || busy) return
    if (e.key === 'a' || e.key === 'ф') decide(first.seq, true)
    if (e.key === 'r' || e.key === 'к') decide(first.seq, false)
  }
</script>

<svelte:window onkeydown={onKey} />

{#if live.queue.length === 0}
  <section class="card center muted">Очередь пуста — новые фото и ответы появятся здесь сами.</section>
{:else}
  <p class="muted small">Первая карточка: <kbd>A</kbd> — принять, <kbd>R</kbd> — отклонить.</p>
  <div class="mgrid">
    {#each live.queue as it, i (it.seq)}
      <article class="card item" class:first={i === 0}>
        {#if it.media_url}
          <AuthImage {api} src={it.media_url} alt="Фото команды {it.team_name}" />
        {:else}
          <blockquote>{it.value}</blockquote>
        {/if}
        <div class="meta">
          <strong>{it.team_name}</strong>
          <div class="small">{it.level_title} · {it.target}</div>
          <div class="muted small">{it.player_name ?? ''} · {ago(it.at, live.now)}</div>
        </div>
        <input class="small" bind:value={comments[it.seq]} placeholder="Комментарий (необязательно)" />
        <div class="row">
          <button class="grow" disabled={busy !== null} onclick={() => decide(it.seq, true)}>Принять</button>
          <button class="grow danger" disabled={busy !== null} onclick={() => decide(it.seq, false)}>Отклонить</button>
        </div>
      </article>
    {/each}
  </div>
{/if}

<style>
  .mgrid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
    gap: 1rem;
  }
  .item {
    display: grid;
    gap: 0.6rem;
    margin: 0;
  }
  .item.first {
    border-color: var(--accent);
    box-shadow: 0 0 0 2px var(--accent-soft);
  }
  blockquote {
    margin: 0;
    padding: 0.75rem;
    background: var(--bg);
    border-radius: 10px;
    white-space: pre-line;
  }
  kbd {
    font-family: inherit;
    border: 1px solid var(--line);
    border-bottom-width: 2px;
    border-radius: 4px;
    padding: 0 0.3rem;
  }
</style>
