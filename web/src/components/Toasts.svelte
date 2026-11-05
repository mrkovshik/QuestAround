<script lang="ts">
  import type { Toast } from '../lib/live.svelte'

  let { toasts }: { toasts: Toast[] } = $props()
</script>

<div class="toasts" aria-live="polite">
  {#each toasts as t (t.id)}
    <div class="toast {t.kind}">{t.text}</div>
  {/each}
</div>

<style>
  .toasts {
    position: fixed;
    top: calc(env(safe-area-inset-top, 0px) + 0.75rem);
    left: 0.75rem;
    right: 0.75rem;
    z-index: 20;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    pointer-events: none;
    max-width: 520px;
    margin: 0 auto;
  }
  .toast {
    padding: 0.8rem 1rem;
    border-radius: 12px;
    font-weight: 600;
    box-shadow: 0 6px 20px rgb(0 0 0 / 0.18);
    animation: in 0.18s ease-out;
    background: var(--card);
    color: var(--text);
    border: 1px solid var(--line);
  }
  .toast.ok {
    background: var(--ok);
    color: #fff;
    border-color: transparent;
  }
  .toast.bad {
    background: var(--bad);
    color: #fff;
    border-color: transparent;
  }
  @keyframes in {
    from {
      transform: translateY(-8px);
      opacity: 0;
    }
  }
</style>
