<script lang="ts">
  // Активные сигналы тревоги: красная плашка над вкладками, звук и мигание заголовка.
  import type { RunLive } from './runLive.svelte'
  import { ago, hhmm } from './time'

  let { live, runId, onOpen }: { live: RunLive; runId: string; onOpen: (teamId: string) => void } = $props()

  const active = $derived((live.dash?.sos ?? []).filter((a) => !a.resolved_at))
  let notes = $state<Record<number, string>>({})

  // Звук: короткая сирена раз в 5 с, пока есть непринятые сигналы.
  // Браузер разрешит звук только после любого клика по странице — он почти всегда уже был.
  let ctx: AudioContext | null = null
  function beep() {
    try {
      ctx ??= new AudioContext()
      const o = ctx.createOscillator()
      const g = ctx.createGain()
      o.type = 'square'
      o.frequency.setValueAtTime(880, ctx.currentTime)
      o.frequency.setValueAtTime(660, ctx.currentTime + 0.25)
      o.frequency.setValueAtTime(880, ctx.currentTime + 0.5)
      g.gain.setValueAtTime(0.08, ctx.currentTime)
      g.gain.setValueAtTime(0, ctx.currentTime + 0.75)
      o.connect(g).connect(ctx.destination)
      o.start()
      o.stop(ctx.currentTime + 0.8)
    } catch {
      /* без звука */
    }
  }

  $effect(() => {
    if (!active.length) return
    beep()
    const base = document.title
    let flip = false
    const t = setInterval(() => {
      flip = !flip
      document.title = flip ? `🚨 ТРЕВОГА (${active.length})` : base
      if (!flip) beep()
    }, 2500)
    return () => {
      clearInterval(t)
      document.title = base
    }
  })

  const maps = (lat: number, lng: number) => ({
    y: `https://yandex.ru/maps/?pt=${lng},${lat}&z=17&l=map`,
    g: `https://www.google.com/maps?q=${lat},${lng}`,
  })

  async function resolve(seq: number) {
    await live.act(() => live.api.resolveSOS(runId, seq, (notes[seq] ?? '').trim()), 'Отмечено: принято')
  }
</script>

{#each active as a (a.seq)}
  <div class="card sos">
    <div class="row wrap">
      <strong class="grow">🚨 Тревога: {a.team_name}</strong>
      <span class="small">{hhmm(a.at)} · {ago(a.at, live.now)}</span>
    </div>
    <div class="small">
      Отправил(а): <strong>{a.player_name}</strong>
      {#if a.contact}· контакт капитана: <strong>{a.contact}</strong>{/if}
      {#if a.current_level}· уровень: {a.current_level}{/if}
    </div>
    {#if a.text}<p class="text">«{a.text}»</p>{/if}
    {#if a.lat != null && a.lng != null}
      {@const m = maps(a.lat, a.lng)}
      <div class="row wrap small">
        <span class="mono">{a.lat.toFixed(5)}, {a.lng.toFixed(5)}</span>
        {#if a.accuracy}<span>± {Math.round(a.accuracy)} м</span>{/if}
        <a href={m.y} target="_blank" rel="noopener">Яндекс Карты ↗</a>
        <a href={m.g} target="_blank" rel="noopener">Google Maps ↗</a>
      </div>
    {:else}
      <div class="small">Координаты не пришли — телефон не дал геолокацию.</div>
    {/if}
    <div class="row wrap act">
      <input class="grow" placeholder="Заметка (необязательно): «созвонились, едем»" bind:value={notes[a.seq]} maxlength="300" />
      <button class="secondary" onclick={() => onOpen(a.team_id)}>Чат команды</button>
      <button class="danger" onclick={() => resolve(a.seq)}>Принял</button>
    </div>
  </div>
{/each}

<style>
  .sos {
    border: 2px solid var(--bad);
    background: color-mix(in srgb, var(--bad) 10%, var(--card, #fff));
    animation: pulse 1.6s ease-in-out infinite;
  }
  @keyframes pulse {
    50% {
      box-shadow: 0 0 0 4px color-mix(in srgb, var(--bad) 30%, transparent);
    }
  }
  .text {
    margin: 0.4rem 0;
    font-size: 1.05rem;
  }
  .act {
    margin-top: 0.6rem;
  }
</style>
