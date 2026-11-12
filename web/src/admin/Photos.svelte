<script lang="ts">
  // Галерея всех фото команд: фильтры, статусы, просмотр крупно, архив.
  import Pic from '../components/Pic.svelte'
  import type { RunLive } from './runLive.svelte'
  import { hhmm } from './time'

  let { live, runId, onOpen }: { live: RunLive; runId: string; onOpen: (teamId: string) => void } = $props()

  let team = $state('')
  let level = $state('')
  let status = $state('')

  const teams = $derived([...new Map(live.photos.map((p) => [p.team_id!, p.team_name!])).entries()])
  const levels = $derived([...new Map(live.photos.map((p) => [p.level_id, p.title.split(' · ')[0]])).entries()])
  const shown = $derived(
    live.photos
      .filter((p) => (!team || p.team_id === team) && (!level || p.level_id === level) && (!status || p.status === status))
      .reverse(),
  )
  const rejected = $derived(live.photos.filter((p) => p.status === 'rejected').length)

  const label = { pending: ['на проверке', 'warn'], accepted: ['принято', 'ok'], rejected: ['отклонено', 'bad'] } as const
</script>

<section class="card">
  <div class="row wrap bar">
    <select bind:value={team}>
      <option value="">Все команды</option>
      {#each teams as [id, name] (id)}<option value={id}>{name}</option>{/each}
    </select>
    <select bind:value={level}>
      <option value="">Все уровни</option>
      {#each levels as [id, title] (id)}<option value={id}>{title}</option>{/each}
    </select>
    <select bind:value={status}>
      <option value="">Любой статус</option>
      <option value="accepted">Принятые</option>
      <option value="pending">На проверке</option>
      <option value="rejected">Отклонённые</option>
    </select>
    <span class="grow muted small">Показано {shown.length} из {live.photos.length}</span>
    {#if live.photos.length}
      <a class="button secondary" href={live.api.photosZip(runId, false)} download>Скачать все (zip)</a>
      {#if rejected}
        <a class="small" href={live.api.photosZip(runId, true)} download>…с отклонёнными</a>
      {/if}
    {/if}
  </div>

  {#if live.photos.length === 0}
    <p class="muted">Фото от команд пока нет.</p>
  {:else}
    <div class="grid">
      {#each shown as p (p.seq)}
        <figure>
          <Pic src={p.url} cls="thumb" alt={p.title} />
          <figcaption class="small">
            <div class="row">
              <button class="link grow" onclick={() => onOpen(p.team_id!)}>{p.team_name}</button>
              <span class="badge {label[p.status][1]}">{label[p.status][0]}</span>
            </div>
            <div class="muted">{hhmm(p.at)} · {p.player_name ?? ''}</div>
            <div>{p.title}</div>
            {#if p.comment}<div class="muted">«{p.comment}»</div>{/if}
          </figcaption>
        </figure>
      {/each}
    </div>
  {/if}
</section>

<style>
  .bar {
    margin-bottom: 1rem;
  }
  .button {
    display: inline-flex;
    align-items: center;
    padding: 0.45rem 0.9rem;
    border-radius: 10px;
    text-decoration: none;
    font-weight: 600;
    background: var(--accent-soft);
    color: var(--accent);
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
    gap: 1rem;
  }
  figure {
    margin: 0;
  }
  figcaption {
    margin-top: 0.35rem;
    line-height: 1.3;
  }
  .link {
    background: none;
    border: 0;
    padding: 0;
    min-height: 0;
    color: inherit;
    font-weight: 700;
    text-align: left;
    cursor: pointer;
  }
</style>
