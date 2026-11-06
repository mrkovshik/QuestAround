<script lang="ts">
  import type { Level } from '../lib/api'
  import type { Live } from '../lib/live.svelte'
  import { clock, minutes, points } from '../lib/format'
  import PhotoButton from './PhotoButton.svelte'

  let { live, level }: { live: Live; level: Level } = $props()

  const s = $derived(live.state!)
  const playing = $derived(s.team.status === 'playing')
  const active = $derived(playing && level.open)
  const canRequest = $derived(active && (s.hint_requests === 'anyone' || s.is_captain))
  const sectors = $derived(level.answers.length > 1)

  let texts = $state<Record<string, string>>({})

  async function sendText(key: string, target: { answer_idx?: number; bonus_id?: string }) {
    const value = (texts[key] ?? '').trim()
    if (!value) return
    const r = await live.submit({ kind: 'text', level_id: level.id, value, ...target })
    if (r) texts[key] = ''
  }

  async function takeHint(idx: number, h: { penalty_sec?: number; penalty_points?: number }) {
    const fine = h.penalty_sec ? `+${minutes(h.penalty_sec)} к времени` : h.penalty_points ? `−${points(h.penalty_points)}` : 'без штрафа'
    if (!confirm(`Взять подсказку? Штраф: ${fine}.`)) return
    await live.requestHint(level.id, idx)
  }

  const isUrl = (m: string) => /^https?:\/\//.test(m)
</script>

<article class="card level" class:solved={!level.open}>
  <header class="row">
    <h2 class="grow">{level.title || 'Задание'}</h2>
    {#if !level.open}
      <span class="badge ok">Пройдено</span>
    {:else if level.points}
      <span class="badge">{points(level.points)}</span>
    {/if}
  </header>

  {#if sectors && level.open}
    <div class="progress" aria-label="Найдено ответов">
      {#each level.answers as a (a.index)}
        <span class="dot" class:on={a.solved} class:wait={a.pending}></span>
      {/each}
      <span class="muted small">{level.solved} из {level.required}{level.required < level.answers.length ? ` (всего ${level.answers.length})` : ''}</span>
    </div>
  {/if}

  <p class="task">{level.task}</p>

  {#each (level.media ?? []).filter(isUrl) as m (m)}
    <img class="media" src={m} alt="" loading="lazy" />
  {/each}

  {#if active}
    <div class="stack">
      {#each level.answers as a (a.index)}
        {#if !a.solved}
          {#if a.type === 'code' && a.options}
            <div class="choices">
              {#each a.options as opt (opt)}
                <button class="secondary" disabled={live.busy} onclick={() => live.submit({ kind: 'code', value: opt })}>{opt}</button>
              {/each}
            </div>
          {:else if a.type === 'geo'}
            <button class="block" disabled={live.busy} onclick={() => live.checkIn()}>Я на месте — отметиться</button>
          {:else if a.type === 'photo'}
            {#if a.pending}
              <div class="badge warn">Фото на проверке</div>
            {:else}
              <PhotoButton {live} label="Сфотографировать" target={{ level_id: level.id, answer_idx: a.index }} />
            {/if}
          {:else if a.type === 'text'}
            {#if a.pending}
              <div class="badge warn">Ответ на проверке</div>
            {:else}
              <div class="row">
                <textarea class="grow" rows="2" placeholder="Ваш ответ" bind:value={texts['a' + a.index]}></textarea>
                <button disabled={live.busy} onclick={() => sendText('a' + a.index, { answer_idx: a.index })}>OK</button>
              </div>
            {/if}
          {/if}
        {/if}
      {/each}
    </div>
  {/if}

  {#if level.hints.length}
    <div class="hints">
      {#each level.hints as h (h.index)}
        {#if h.opened}
          <div class="hint"><strong>Подсказка {h.index + 1}.</strong> <span class="task">{h.text}</span></div>
        {:else if active && !h.on_request && h.opens_at}
          <div class="muted small">Подсказка {h.index + 1} откроется через {clock((Date.parse(h.opens_at) - live.now) / 1000)}</div>
        {:else if canRequest && h.on_request}
          <button class="secondary block" onclick={() => takeHint(h.index, h)}>
            Взять подсказку {h.index + 1}
            {#if h.penalty_sec}<span class="fine">(+{minutes(h.penalty_sec)})</span>{/if}
            {#if h.penalty_points}<span class="fine">(−{points(h.penalty_points)})</span>{/if}
          </button>
        {:else if active && h.on_request}
          <div class="muted small">Подсказку {h.index + 1} может взять капитан.</div>
        {/if}
      {/each}
    </div>
  {/if}

  {#if level.bonuses?.length}
    <div class="bonuses">
      <h3>Бонусы</h3>
      {#each level.bonuses as b (b.id)}
        <div class="bonus">
          <div class="row">
            <span class="grow">{b.title || 'Бонус'}</span>
            {#if b.done}
              <span class="badge ok">+{b.points}</span>
            {:else if b.pending}
              <span class="badge warn">на проверке</span>
            {:else}
              <span class="badge">+{b.points}</span>
            {/if}
          </div>
          {#if playing && !b.done && !b.pending}
            {#if b.type === 'photo'}
              <PhotoButton {live} label="Отправить фото" secondary target={{ level_id: level.id, bonus_id: b.id }} />
            {:else if b.type === 'geo'}
              <button class="secondary block" disabled={live.busy} onclick={() => live.checkIn()}>Отметиться на месте</button>
            {:else if b.type === 'text'}
              <div class="row">
                <textarea class="grow" rows="2" placeholder="Ваш ответ" bind:value={texts['b' + b.id]}></textarea>
                <button disabled={live.busy} onclick={() => sendText('b' + b.id, { bonus_id: b.id })}>OK</button>
              </div>
            {:else}
              <div class="muted small">Код бонуса вводите в общее поле внизу.</div>
            {/if}
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</article>

<style>
  .level.solved {
    opacity: 0.85;
  }
  .progress {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    margin-bottom: 0.6rem;
  }
  .dot {
    width: 12px;
    height: 12px;
    border-radius: 50%;
    border: 2px solid var(--line);
  }
  .dot.on {
    background: var(--ok);
    border-color: var(--ok);
  }
  .dot.wait {
    border-color: var(--warn);
  }
  .media {
    width: 100%;
    border-radius: 10px;
    margin-bottom: 0.75rem;
  }
  .choices {
    display: grid;
    gap: 0.5rem;
  }
  .hints {
    margin-top: 0.9rem;
    display: grid;
    gap: 0.5rem;
  }
  .hint {
    background: var(--accent-soft);
    border-radius: 10px;
    padding: 0.6rem 0.8rem;
  }
  .fine {
    font-weight: 400;
    opacity: 0.8;
  }
  .bonuses {
    margin-top: 1rem;
    border-top: 1px dashed var(--line);
    padding-top: 0.75rem;
  }
  .bonus + .bonus {
    margin-top: 0.75rem;
  }
  .bonus .row {
    margin-bottom: 0.4rem;
  }
  textarea {
    resize: vertical;
  }
</style>
