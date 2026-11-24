<script lang="ts">
  import type { Level } from '../lib/api'
  import type { Live } from '../lib/live.svelte'
  import { countdown, minutes, points } from '../lib/format'
  import PhotoButton from './PhotoButton.svelte'
  import Pic from './Pic.svelte'

  let { live, level }: { live: Live; level: Level } = $props()

  const s = $derived(live.state!)
  const playing = $derived(s.team.status === 'playing')
  const active = $derived(playing && level.open)
  const canRequest = $derived(active && (s.hint_requests === 'anyone' || s.is_captain))
  const sectors = $derived(level.answers.length > 1)

  /** Секунд до момента iso; ≤ 0 — уже наступил (или не задан). */
  const left = (iso?: string) => (iso ? (Date.parse(iso) - live.now) / 1000 : 0)
  // Ближайшая подсказка по таймеру — для строки «До следующей подсказки…».
  const nextTimer = $derived(
    level.hints
      .filter((h) => !h.opened && !h.on_request && h.opens_at && left(h.opens_at) > 0)
      .sort((a, b) => Date.parse(a.opens_at!) - Date.parse(b.opens_at!))[0],
  )
  const locWait = $derived(left(level.location?.available_at))
  const rl = $derived(level.rate_limit)
  const blockedFor = $derived(left(rl?.blocked_until))
  const rlRule = $derived.by(() => {
    if (!rl) return ''
    const per = rl.per_sec >= 60 ? minutes(rl.per_sec) : `${rl.per_sec} сек`
    const then =
      rl.action === 'block'
        ? `ввод закрывается на ${minutes(rl.block_sec ?? rl.per_sec)}`
        : `штраф ${rl.penalty_sec ? '+' + minutes(rl.penalty_sec) : '−' + points(rl.penalty_points ?? 0)}`
    return `${rl.wrong_answers} неверных кодов за ${per} — ${then}`
  })
  const skipWait = $derived(left(level.skip?.available_at))

  let texts = $state<Record<string, string>>({})

  async function sendText(key: string, target: { answer_idx?: number; bonus_id?: string }) {
    const value = (texts[key] ?? '').trim()
    if (!value) return
    const r = await live.submit({ kind: 'text', level_id: level.id, value, ...target })
    if (r) texts[key] = ''
  }

  // Штраф слива/подсказки словами: «+15 мин к времени» / «−3 балла» / «без штрафа».
  const fineText = (p: { penalty_sec?: number; penalty_points?: number }) =>
    p.penalty_sec ? `+${minutes(p.penalty_sec)} к времени` : p.penalty_points ? `−${points(p.penalty_points)}` : 'без штрафа'
  const fineShort = (p: { penalty_sec?: number; penalty_points?: number }) =>
    p.penalty_sec ? `+${minutes(p.penalty_sec)}` : p.penalty_points ? `−${points(p.penalty_points)}` : 'без штрафа'

  async function slivLocation() {
    if (!level.location) return
    if (!confirm(`Слить локацию уровня «${level.title}»?\n\nПокажем, где искать: адрес или точку на карте. Штраф: ${fineText(level.location)}.`)) return
    await live.sliv('location', level.id)
  }

  async function slivLevel() {
    if (!level.skip) return
    const pts = level.points && s.scoring_mode !== 'time' ? ` Баллы за уровень (${level.points}) не начислятся.` : ''
    if (!confirm(`Слить уровень «${level.title}»?\n\nУровень засчитают без ответа, и откроется следующий.${pts} Штраф: ${fineText(level.skip)}.\n\nОтменить нельзя.`)) return
    await live.sliv('skip', level.id)
  }

  const mapLinks = (lat: number, lng: number) => ({
    y: `https://yandex.ru/maps/?pt=${lng},${lat}&z=17&l=map`,
    g: `https://www.google.com/maps/search/?api=1&query=${lat},${lng}`,
  })

  async function takeHint(idx: number, h: { penalty_sec?: number; penalty_points?: number }) {
    const fine = h.penalty_sec ? `+${minutes(h.penalty_sec)} к времени` : h.penalty_points ? `−${points(h.penalty_points)}` : 'без штрафа'
    if (!confirm(`Взять подсказку? Штраф: ${fine}.`)) return
    await live.requestHint(level.id, idx)
  }

  // Что даёт бонус: в зачёте по времени — минуты, иначе баллы (и минуты, если заданы).
  function bonusLabel(b: { points: number; time_sec?: number }): string {
    const parts: string[] = []
    if (b.points && live.state?.scoring_mode !== 'time') parts.push(`+${b.points}`)
    if (b.time_sec) parts.push(`−${minutes(b.time_sec)}`)
    return parts.join(', ')
  }
</script>

<article class="card level" class:solved={!level.open}>
  <header class="row">
    <h2 class="grow">{level.title || 'Задание'}</h2>
    {#if !level.open && level.skipped}
      <span class="badge warn">Слит</span>
    {:else if !level.open}
      <span class="badge ok">Пройдено</span>
    {:else if level.points && live.state?.scoring_mode !== 'time'}
      <span class="badge">{points(level.points)}</span>
    {/if}
  </header>

  {#if active && rl}
    {#if blockedFor > 0}
      <div class="rl blocked">⛔ Слишком много неверных кодов — ввод закрыт ещё {countdown(blockedFor)}</div>
    {:else}
      <div class="rl muted small">Защита от перебора: {rlRule}</div>
    {/if}
  {/if}

  {#if sectors && level.open}
    <div class="progress" aria-label="Найдено ответов">
      {#each level.answers as a (a.index)}
        <span class="dot" class:on={a.solved} class:wait={a.pending}></span>
      {/each}
      <span class="muted small">{level.solved} из {level.required}{level.required < level.answers.length ? ` (всего ${level.answers.length})` : ''}</span>
    </div>
  {/if}

  <p class="task">{level.task}</p>

  {#each level.media ?? [] as m (m)}
    <div class="media"><Pic src={m} /></div>
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
          <div class="hint">
            <strong>Подсказка {h.index + 1}.</strong> <span class="task">{h.text ?? ''}</span>
            {#each h.media ?? [] as m (m)}
              <div class="hmedia"><Pic src={m} /></div>
            {/each}
          </div>
        {:else if active && !h.on_request && h === nextTimer}
          <div class="timer">До следующей подсказки: <strong>{countdown(left(h.opens_at))}</strong></div>
        {:else if canRequest && h.on_request}
          {@const wait = left(h.available_at)}
          <button class="secondary block" disabled={wait > 0} onclick={() => takeHint(h.index, h)}>
            Взять подсказку {h.index + 1}
            {#if h.penalty_sec}<span class="fine">(+{minutes(h.penalty_sec)})</span>{/if}
            {#if h.penalty_points}<span class="fine">(−{points(h.penalty_points)})</span>{/if}
          </button>
          {#if wait > 0}
            <div class="muted small free">Можно будет взять через {countdown(wait)}</div>
          {/if}
          {#if h.opens_at}
            <div class="muted small free">или откроется сама, без штрафа, через {countdown(left(h.opens_at))}</div>
          {/if}
        {:else if active && h.on_request}
          {@const wait = left(h.available_at)}
          <div class="muted small">
            Подсказку {h.index + 1} может взять капитан{wait > 0 ? ` через ${countdown(wait)}` : ''}{#if h.opens_at}, или она откроется сама через {countdown(left(h.opens_at))}{/if}.
          </div>
        {/if}
      {/each}
    </div>
  {/if}

  {#if level.location?.revealed}
    <div class="hint loc">
      <strong>Локация.</strong>
      {#if level.location.text}<span class="task">{level.location.text}</span>{/if}
      {#if level.location.lat != null && level.location.lng != null}
        {@const m = mapLinks(level.location.lat, level.location.lng)}
        <div class="small maplinks">
          <span class="mono">{level.location.lat.toFixed(5)}, {level.location.lng.toFixed(5)}</span>
          <a href={m.y} target="_blank" rel="noopener">Яндекс Карты ↗</a>
          <a href={m.g} target="_blank" rel="noopener">Google Maps ↗</a>
        </div>
      {/if}
    </div>
  {/if}

  {#if active && ((level.location && !level.location.revealed) || level.skip)}
    <div class="sliv">
      {#if s.is_captain}
        {#if level.location && !level.location.revealed}
          <button class="ghost block" disabled={live.busy || locWait > 0} onclick={slivLocation}>
            Слить локацию <span class="fine">({fineShort(level.location)})</span>
          </button>
          <div class="muted small desc">
            Покажем, где искать: адрес или точку на карте.{#if locWait > 0}<br /><strong>Доступно через {countdown(locWait)}</strong>{/if}
          </div>
        {/if}
        {#if level.skip}
          <button class="ghost block warnbtn" disabled={live.busy || skipWait > 0} onclick={slivLevel}>
            Слить уровень <span class="fine">({fineShort(level.skip)})</span>
          </button>
          <div class="muted small desc">
            Уровень засчитают без ответа{level.points && s.scoring_mode !== 'time' ? ', баллы за него не начислятся' : ''}.{#if skipWait > 0}<br /><strong>Доступно через {countdown(skipWait)}</strong>{/if}
          </div>
        {/if}
      {:else}
        <div class="muted small">
          Капитан может
          {#if level.location && !level.location.revealed}слить локацию ({fineShort(level.location)}{locWait > 0 ? `, через ${countdown(locWait)}` : ''}){/if}{#if level.location && !level.location.revealed && level.skip}
            или{/if}
          {#if level.skip}слить уровень ({fineShort(level.skip)}{skipWait > 0 ? `, через ${countdown(skipWait)}` : ''}){/if}.
        </div>
      {/if}
    </div>
  {/if}

  {#if level.bonuses?.length}
    <div class="bonuses">
      <h3>Бонусы</h3>
      {#each level.bonuses as b (b.id)}
        <div class="bonus">
          <div class="row">
            <span class="grow">{b.title || 'Бонус'}</span>
            {#if b.pending}
              <span class="badge warn">на проверке</span>
            {:else if bonusLabel(b)}
              <span class="badge" class:ok={b.done}>{bonusLabel(b)}</span>
            {/if}
          </div>
          {#each b.media ?? [] as m (m)}
            <div class="hmedia"><Pic src={m} /></div>
          {/each}
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
    margin-bottom: 0.75rem;
  }
  .hmedia {
    margin-top: 0.5rem;
  }
  .choices {
    display: grid;
    gap: 0.5rem;
  }
  .rl {
    margin: 0.25rem 0 0.5rem;
  }
  .rl.blocked {
    padding: 0.5rem 0.75rem;
    border-radius: 10px;
    background: var(--bad-soft);
    color: var(--bad);
    font-weight: 600;
  }
  .free {
    margin-top: 0.25rem;
    text-align: center;
  }
  .timer {
    text-align: center;
    padding: 0.5rem;
    border-radius: 10px;
    background: var(--bg);
    font-variant-numeric: tabular-nums;
  }
  .timer strong {
    white-space: nowrap;
  }
  .sliv {
    margin-top: 0.75rem;
    padding-top: 0.75rem;
    border-top: 1px dashed var(--line);
    display: grid;
    gap: 0.25rem;
  }
  .sliv .desc {
    margin: 0 0 0.4rem;
    text-align: center;
  }
  .warnbtn {
    color: var(--bad);
  }
  .loc {
    margin-top: 0.75rem;
  }
  .maplinks {
    display: flex;
    flex-wrap: wrap;
    gap: 0.75rem;
    margin-top: 0.3rem;
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
