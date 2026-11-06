<script lang="ts">
  import type { Live } from '../lib/live.svelte'

  let { live }: { live: Live } = $props()

  const s = $derived(live.state!)
  const mySos = $derived(s.sos[0]) // последний сигнал команды
  const sosActive = $derived(!!mySos && !mySos.resolved_at)
  const feed = $derived([...s.messages].reverse())

  let sosOpen = $state(false)
  let sosText = $state('')
  let chatText = $state('')
  let thread: HTMLElement | undefined = $state()

  const hhmm = (iso: string) => new Date(iso).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' })
  const telHref = $derived(s.host_contact && /^[+\d][\d\s()-]{5,}$/.test(s.host_contact) ? 'tel:' + s.host_contact.replace(/[^\d+]/g, '') : '')

  async function sendSOS() {
    if (await live.raiseSOS(sosText.trim())) {
      sosOpen = false
      sosText = ''
    }
  }

  async function sendChat(e: SubmitEvent) {
    e.preventDefault()
    if (await live.sendChat(chatText.trim())) chatText = ''
  }

  // Открыли вкладку — сообщения прочитаны; держим ленту чата внизу.
  $effect(() => {
    s.chat.length
    s.messages.length
    live.markRead()
    if (thread) thread.scrollTop = thread.scrollHeight
  })
</script>

<section class="card sos">
  {#if sosActive}
    <h3>Сигнал отправлен в {hhmm(mySos.at)}</h3>
    <p>Ведущий скоро свяжется с командой. Оставайтесь на месте, если это безопасно.</p>
  {:else if mySos?.resolved_at}
    <p class="small muted">
      Последний сигнал ({hhmm(mySos.at)}) ведущий принял в {hhmm(mySos.resolved_at)}{mySos.note ? `: «${mySos.note}»` : ''}.
    </p>
  {/if}
  {#if sosOpen}
    <div class="stack">
      <label for="sostext">Что случилось? (необязательно)</label>
      <textarea id="sostext" rows="2" bind:value={sosText} placeholder="Например: проколол колесо, нужна помощь"></textarea>
      <p class="small muted">Ведущему уйдут ваши координаты (если телефон их отдаст), имя и команда.</p>
      <button class="block alarm" disabled={live.busy} onclick={sendSOS}>{live.busy ? 'Отправляем…' : 'Отправить сигнал'}</button>
      <button class="block ghost" onclick={() => (sosOpen = false)}>Отмена</button>
    </div>
  {:else}
    <button class="block alarm" onclick={() => (sosOpen = true)}>Нужна помощь</button>
    <p class="small muted center hint">Если кто-то получил травму, потерялся или что-то сломалось.</p>
  {/if}
  {#if s.host_contact}
    <p class="contact">
      Телефон ведущего:
      {#if telHref}<a href={telHref}>{s.host_contact}</a>{:else}<strong>{s.host_contact}</strong>{/if}
    </p>
  {/if}
</section>

<section class="card">
  <h3>Связь с ведущим</h3>
  {#if s.chat.length === 0}
    <p class="muted small">Здесь — переписка вашей команды с ведущим.</p>
  {:else}
    <div class="thread" bind:this={thread}>
      {#each s.chat as m (m.seq)}
        <div class="msg" class:host={m.from_host}>
          <div class="meta small muted">{m.author} · {hhmm(m.at)}</div>
          <div class="bubble task">{m.text}</div>
        </div>
      {/each}
    </div>
  {/if}
  {#if s.is_captain}
    <form class="row send" onsubmit={sendChat}>
      <input class="grow" bind:value={chatText} placeholder="Сообщение ведущему" maxlength="1000" />
      <button disabled={!chatText.trim()}>Отправить</button>
    </form>
  {:else}
    <p class="muted small">Писать ведущему может капитан. Если срочно — кнопка «Нужна помощь».</p>
  {/if}
</section>

<section class="card">
  <h3>Объявления</h3>
  {#if feed.length === 0}
    <p class="muted small">Пока объявлений не было.</p>
  {:else}
    {#each feed as m, i (i)}
      <div class="ann">
        <div class="small muted">{hhmm(m.at)}</div>
        <div class="task">{m.text}</div>
      </div>
    {/each}
  {/if}
</section>

<style>
  .sos {
    border-color: var(--bad);
  }
  .alarm {
    background: var(--bad);
    color: #fff;
    font-size: 1.1rem;
    min-height: 54px;
  }
  .hint {
    margin: 0.5rem 0 0;
  }
  .contact {
    margin: 0.75rem 0 0;
    text-align: center;
  }
  .contact a {
    font-weight: 700;
    font-size: 1.1rem;
  }
  .thread {
    max-height: 45vh;
    overflow-y: auto;
    display: grid;
    gap: 0.5rem;
    margin-bottom: 0.75rem;
  }
  .msg {
    max-width: 85%;
  }
  .msg.host {
    justify-self: start;
  }
  .msg:not(.host) {
    justify-self: end;
    text-align: right;
  }
  .bubble {
    display: inline-block;
    text-align: left;
    padding: 0.5rem 0.75rem;
    border-radius: 12px;
    background: var(--bg);
  }
  .msg.host .bubble {
    background: var(--accent-soft);
  }
  .ann + .ann {
    border-top: 1px solid var(--line);
    margin-top: 0.5rem;
    padding-top: 0.5rem;
  }
</style>
