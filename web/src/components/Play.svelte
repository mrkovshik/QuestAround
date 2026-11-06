<script lang="ts">
  import type { Live } from '../lib/live.svelte'
  import LevelCard from './LevelCard.svelte'

  let { live }: { live: Live } = $props()

  const s = $derived(live.state!)
  const playing = $derived(s.team.status === 'playing')
  // После финиша незаконченные уровни не показываем — только пройденные.
  const open = $derived(playing ? s.levels.filter((l) => l.open) : [])
  const done = $derived(s.levels.filter((l) => !l.open).reverse())

  // Общее поле кода: ответ ищется по всем открытым уровням и бонусам.
  const needsCode = $derived(
    playing &&
      (open.some((l) => l.answers.some((a) => a.type === 'code' && !a.solved && !a.options)) ||
        s.levels.some((l) => l.bonuses?.some((b) => b.type === 'code' && !b.done))),
  )

  let code = $state('')
  let input: HTMLInputElement | undefined = $state()

  async function send(e: SubmitEvent) {
    e.preventDefault()
    const value = code.trim()
    if (!value || live.busy) return
    const r = await live.submit({ kind: 'code', value })
    if (r?.result === 'wrong') {
      input?.select()
    } else {
      code = ''
    }
  }
</script>

{#if playing && open.length === 0}
  <div class="card center muted">Ждём следующее задание…</div>
{/if}

{#each open as level (level.id)}
  <LevelCard {live} {level} />
{/each}

{#if done.length}
  <details class="done">
    <summary>Пройдено: {done.length}</summary>
    {#each done as level (level.id)}
      <LevelCard {live} {level} />
    {/each}
  </details>
{/if}

{#if needsCode}
  <form class="codebar" onsubmit={send}>
    <input
      bind:this={input}
      bind:value={code}
      placeholder="Введите код"
      autocomplete="off"
      autocorrect="off"
      autocapitalize="none"
      spellcheck="false"
      enterkeyhint="send"
    />
    <button disabled={!code.trim() || live.busy}>Отправить</button>
  </form>
{/if}

<style>
  .done summary {
    cursor: pointer;
    color: var(--muted);
    padding: 0.5rem 0.25rem;
    font-weight: 600;
  }
  .codebar {
    position: fixed;
    left: 0;
    right: 0;
    bottom: calc(54px + var(--safe-bottom));
    z-index: 4;
    display: flex;
    gap: 0.5rem;
    padding: 0.6rem 0.75rem;
    background: var(--card);
    border-top: 1px solid var(--line);
    max-width: 560px;
    margin: 0 auto;
  }
  .codebar input {
    flex: 1;
    min-width: 0;
  }
</style>
