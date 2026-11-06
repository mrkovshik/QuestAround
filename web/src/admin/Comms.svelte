<script lang="ts">
  // Связь: каналы команд (капитан ↔ ведущий), история объявлений, телефон ведущего.
  import type { RunLive } from './runLive.svelte'
  import { hhmm } from './time'

  let { live, runId, teamId = $bindable() }: { live: RunLive; runId: string; teamId: string | null } = $props()

  const d = $derived(live.dash!)
  const KEY = $derived(`qa:admin:seen:${runId}`)

  // Последний просмотренный seq в каждом канале — чтобы считать непрочитанные.
  let seen = $state<Record<string, number>>({})
  $effect(() => {
    try {
      seen = JSON.parse(localStorage.getItem(KEY) ?? '{}')
    } catch {
      seen = {}
    }
  })

  const unreadOf = (t: { id: string; chat: { seq: number; from_host: boolean }[] }) =>
    t.chat.filter((m) => !m.from_host && m.seq > (seen[t.id] ?? 0)).length

  const teams = $derived(
    [...d.teams].sort((a, b) => {
      const la = a.chat.at(-1)?.seq ?? 0
      const lb = b.chat.at(-1)?.seq ?? 0
      return lb - la || a.name.localeCompare(b.name)
    }),
  )
  const cur = $derived(d.teams.find((t) => t.id === teamId) ?? null)

  // Открытый канал — прочитан.
  $effect(() => {
    if (!cur) return
    const last = cur.chat.at(-1)?.seq ?? 0
    if ((seen[cur.id] ?? 0) >= last) return
    seen = { ...seen, [cur.id]: last }
    try {
      localStorage.setItem(KEY, JSON.stringify(seen))
    } catch {
      /* ignore */
    }
  })

  let thread: HTMLElement | undefined = $state()
  $effect(() => {
    cur?.chat.length
    if (thread) thread.scrollTop = thread.scrollHeight
  })

  let text = $state('')
  async function send(e: SubmitEvent) {
    e.preventDefault()
    if (!cur) return
    if (await live.act(() => live.api.teamChat(runId, cur.id, text.trim()))) text = ''
  }

  let contact = $state('')
  let contactInit = false
  $effect(() => {
    if (!contactInit && d) {
      contact = d.host_contact
      contactInit = true
    }
  })
  async function saveContact(e: SubmitEvent) {
    e.preventDefault()
    await live.act(() => live.api.setHostContact(runId, contact.trim()), 'Контакт сохранён — игроки видят его во вкладке «Связь»')
  }

  const sosDone = $derived(d.sos.filter((a) => a.resolved_at))
</script>

<div class="grid">
  <section class="card list">
    <h3>Команды</h3>
    {#if teams.length === 0}<p class="muted small">Команд пока нет.</p>{/if}
    {#each teams as t (t.id)}
      {@const n = unreadOf(t)}
      <button class="item" class:active={t.id === teamId} onclick={() => (teamId = t.id)}>
        <span class="grow">{t.name}</span>
        {#if n}<span class="counter">{n}</span>{:else if t.chat.length}<span class="muted small">{t.chat.length}</span>{/if}
      </button>
    {/each}
  </section>

  <section class="card">
    {#if !cur}
      <p class="muted">Выберите команду слева. Писать ведущему может капитан; ответ увидит вся команда.</p>
    {:else}
      <h3>{cur.name}</h3>
      <div class="thread" bind:this={thread}>
        {#if cur.chat.length === 0}<p class="muted small">Сообщений пока нет.</p>{/if}
        {#each cur.chat as m (m.seq)}
          <div class="msg" class:host={m.from_host}>
            <div class="small muted">{m.author} · {hhmm(m.at)}</div>
            <div class="bubble">{m.text}</div>
          </div>
        {/each}
      </div>
      <form class="row" onsubmit={send}>
        <input class="grow" bind:value={text} placeholder="Ответ команде «{cur.name}»" maxlength="1000" />
        <button disabled={!text.trim()}>Отправить</button>
      </form>
    {/if}
  </section>

  <section class="card side">
    <h3>Телефон для экстренной связи</h3>
    <form class="stack" onsubmit={saveContact}>
      <input bind:value={contact} placeholder="+7 900 123-45-67, Алексей" maxlength="200" />
      <button class="secondary" disabled={contact.trim() === d.host_contact}>Сохранить</button>
    </form>
    <p class="small muted">Показывается игрокам рядом с кнопкой «Нужна помощь» — на случай, если пропал интернет.</p>

    <h3>Объявления</h3>
    {#if d.messages.length === 0}<p class="muted small">Не было.</p>{/if}
    {#each [...d.messages].reverse() as m, i (i)}
      <div class="ann small"><span class="muted">{hhmm(m.at)}</span> {m.text}</div>
    {/each}

    {#if sosDone.length}
      <h3>Принятые тревоги</h3>
      {#each sosDone as a (a.seq)}
        <div class="ann small">
          <span class="muted">{hhmm(a.at)}→{hhmm(a.resolved_at)}</span>
          {a.team_name}, {a.player_name}{a.text ? `: «${a.text}»` : ''}{a.note ? ` — ${a.note}` : ''}
        </div>
      {/each}
    {/if}
  </section>
</div>

<style>
  .grid {
    display: grid;
    grid-template-columns: 220px 1fr 300px;
    gap: 1rem;
    align-items: start;
  }
  @media (max-width: 1000px) {
    .grid {
      grid-template-columns: 1fr;
    }
  }
  .list {
    display: grid;
    gap: 0.25rem;
  }
  .item {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    text-align: left;
    background: transparent;
    color: inherit;
    border: 1px solid transparent;
    padding: 0.45rem 0.6rem;
  }
  .item.active {
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .thread {
    max-height: 55vh;
    min-height: 200px;
    overflow-y: auto;
    display: grid;
    gap: 0.5rem;
    align-content: start;
    margin-bottom: 0.75rem;
  }
  .msg {
    max-width: 75%;
  }
  .msg.host {
    justify-self: end;
    text-align: right;
  }
  .bubble {
    display: inline-block;
    text-align: left;
    padding: 0.45rem 0.7rem;
    border-radius: 10px;
    background: var(--bg);
    white-space: pre-wrap;
  }
  .msg.host .bubble {
    background: var(--accent-soft);
  }
  .side h3 + * {
    margin-top: 0.3rem;
  }
  .side h3:not(:first-child) {
    margin-top: 1.2rem;
  }
  .ann {
    padding: 0.3rem 0;
    border-bottom: 1px solid var(--line);
  }
</style>
