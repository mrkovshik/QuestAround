<script lang="ts">
  // Выбор геоточки на карте (OpenStreetMap): клик ставит точку, круг — радиус зачёта.
  // Остальные гео-уровни сценария показаны серыми — видно, как разнесены точки.
  import 'leaflet/dist/leaflet.css'
  import L from 'leaflet'
  import { onMount } from 'svelte'

  let {
    point,
    radius,
    others,
    onpick,
    onclose,
  }: {
    point?: number[]
    radius: number
    others: { lat: number; lng: number; title: string; radius: number }[]
    onpick: (p: [number, number]) => void
    onclose: () => void
  } = $props()

  // Горячий Ключ — точка по умолчанию, пока в сценарии нет ни одной.
  const FALLBACK: [number, number] = [44.6339, 39.1355]

  let el: HTMLDivElement
  // svelte-ignore state_referenced_locally — начальное значение, дальше точку двигает клик
  let picked = $state<[number, number] | null>(point?.length === 2 ? [point[0], point[1]] : null)

  onMount(() => {
    const map = L.map(el, { zoomControl: true })
    L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', {
      maxZoom: 19,
      attribution: '&copy; OpenStreetMap',
    }).addTo(map)

    const bounds: [number, number][] = []
    for (const o of others) {
      L.circle([o.lat, o.lng], { radius: o.radius || 25, color: '#888', weight: 1, fillOpacity: 0.15 })
        .bindTooltip(o.title)
        .addTo(map)
      bounds.push([o.lat, o.lng])
    }

    const accent = getComputedStyle(document.documentElement).getPropertyValue('--accent').trim() || '#1f6f5c'
    const circle = L.circle(picked ?? FALLBACK, { radius: radius || 25, color: accent, weight: 2, fillOpacity: 0.2 })
    const dot = L.circleMarker(picked ?? FALLBACK, { radius: 5, color: accent, fillOpacity: 1 })
    if (picked) {
      circle.addTo(map)
      dot.addTo(map)
      map.setView(picked, 17)
    } else if (bounds.length) {
      map.fitBounds(bounds, { padding: [40, 40], maxZoom: 17 })
    } else {
      map.setView(FALLBACK, 15)
    }

    map.on('click', (e: L.LeafletMouseEvent) => {
      picked = [round(e.latlng.lat), round(e.latlng.lng)]
      circle.setLatLng(picked).addTo(map)
      dot.setLatLng(picked).addTo(map)
    })
    return () => map.remove()
  })

  const round = (x: number) => Math.round(x * 1e6) / 1e6

  function onkey(e: KeyboardEvent) {
    if (e.key === 'Escape') onclose()
  }
</script>

<svelte:window onkeydown={onkey} />

<div class="back" role="presentation" onclick={onclose}></div>
<div class="modal" role="dialog" aria-label="Точка на карте">
  <div class="row bar">
    <strong>Кликните по карте, чтобы поставить точку</strong>
    <span class="grow"></span>
    {#if picked}<span class="mono small">{picked[0]}, {picked[1]}</span>{/if}
    <button class="ghost small" onclick={onclose}>Отмена</button>
    <button class="small" disabled={!picked} onclick={() => picked && onpick(picked)}>Готово</button>
  </div>
  <div class="map" bind:this={el}></div>
</div>

<style>
  .back {
    position: fixed;
    inset: 0;
    background: rgb(0 0 0 / 0.4);
    z-index: 100;
  }
  .modal {
    position: fixed;
    inset: 4vh 4vw;
    z-index: 101;
    display: flex;
    flex-direction: column;
    background: var(--card);
    border-radius: var(--radius);
    overflow: hidden;
  }
  .bar {
    padding: 0.6rem 0.9rem;
    border-bottom: 1px solid var(--line);
  }
  .map {
    flex: 1;
  }
</style>
