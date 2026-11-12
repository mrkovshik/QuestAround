<script lang="ts">
  // Библиотека картинок для сценариев: в YAML на них ссылаются по имени файла.
  import { ApiError } from '../lib/api'
  import { compressImage } from '../lib/image'
  import Pic from '../components/Pic.svelte'
  import type { AdminApi, Asset } from './adminApi'

  let { api }: { api: AdminApi } = $props()

  let list = $state<Asset[]>([])
  let busy = $state(false)
  let msg = $state<{ kind: 'ok' | 'bad'; text: string } | null>(null)
  let over = $state(false)
  let input: HTMLInputElement | undefined = $state()

  async function load() {
    try {
      list = await api.assets()
    } catch (e) {
      msg = { kind: 'bad', text: (e as Error).message }
    }
  }
  $effect(() => {
    load()
  })

  // Большие фото с телефона ужимаем до 1600px: игрокам грузить по мобильной сети.
  async function prepare(f: File): Promise<Blob> {
    if (f.type === 'image/gif' || f.size < 600_000) return f
    try {
      return await compressImage(f)
    } catch {
      return f
    }
  }

  async function add(files: FileList | File[] | null | undefined) {
    const arr = [...(files ?? [])]
    if (!arr.length) return
    const replaced = arr.filter((f) => list.some((a) => a.name === f.name)).map((f) => f.name)
    if (replaced.length && !confirm(`Заменить картинки: ${replaced.join(', ')}? В идущих играх они поменяются сразу.`)) return
    busy = true
    msg = null
    try {
      const prepared = await Promise.all(arr.map(async (f) => ({ name: f.name, blob: await prepare(f) })))
      const saved = await api.uploadAssets(prepared)
      msg = { kind: 'ok', text: `Загружено: ${saved.map((a) => a.name).join(', ')}` }
      await load()
    } catch (e) {
      msg = { kind: 'bad', text: e instanceof ApiError ? e.message : 'Нет связи с сервером' }
    } finally {
      busy = false
      if (input) input.value = ''
    }
  }

  async function remove(a: Asset) {
    if (!confirm(`Убрать «${a.name}» из библиотеки? Если она есть в сценарии, у игроков картинка пропадёт.`)) return
    await api.deleteAsset(a.name).catch((e) => (msg = { kind: 'bad', text: (e as Error).message }))
    await load()
  }

  async function copy(name: string) {
    try {
      await navigator.clipboard.writeText(name)
      msg = { kind: 'ok', text: `Скопировано: ${name}` }
    } catch {
      /* ignore */
    }
  }

  const kb = (n: number) => (n > 1_000_000 ? `${(n / 1_000_000).toFixed(1)} МБ` : `${Math.round(n / 1000)} КБ`)
</script>

<section class="card">
  <div class="row wrap">
    <h2 class="grow">Картинки для заданий</h2>
    <button class="secondary" disabled={busy} onclick={() => input?.click()}>{busy ? 'Загружаем…' : 'Добавить картинки'}</button>
    <input bind:this={input} type="file" accept="image/jpeg,image/png,image/webp,image/gif" multiple hidden onchange={() => add(input?.files)} />
  </div>
  <p class="muted small">
    Загрузите картинки и укажите их в сценарии по имени файла — у уровня, подсказки или бонуса:
    <code>media: [park.jpg]</code>. Картинка подсказки не уходит игрокам, пока подсказка не открыта.
    Загрузка файла с тем же именем заменяет картинку, в том числе в идущей игре.
  </p>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="drop"
    class:over
    ondragover={(e) => {
      e.preventDefault()
      over = true
    }}
    ondragleave={() => (over = false)}
    ondrop={(e) => {
      e.preventDefault()
      over = false
      add(e.dataTransfer?.files)
    }}
  >
    {#if list.length === 0}
      <p class="muted center">Перетащите картинки сюда</p>
    {:else}
      <div class="grid">
        {#each list as a (a.name)}
          <figure>
            <Pic src={a.url} cls="thumb" alt={a.name} />
            <figcaption class="small">
              <button class="link mono" title="Скопировать имя" onclick={() => copy(a.name)}>{a.name}</button>
              <span class="muted">{kb(a.size)}</span>
              <button class="link del" title="Убрать из библиотеки" onclick={() => remove(a)}>×</button>
            </figcaption>
          </figure>
        {/each}
      </div>
    {/if}
  </div>
  {#if msg}<div class="notice {msg.kind}">{msg.text}</div>{/if}
</section>

<style>
  .drop {
    border: 2px dashed var(--line);
    border-radius: 12px;
    padding: 0.75rem;
    min-height: 80px;
  }
  .drop.over {
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(130px, 1fr));
    gap: 0.75rem;
  }
  figure {
    margin: 0;
  }
  figcaption {
    display: flex;
    gap: 0.3rem;
    align-items: baseline;
    margin-top: 0.25rem;
  }
  .link {
    background: none;
    border: 0;
    padding: 0;
    min-height: 0;
    color: inherit;
    cursor: pointer;
    text-align: left;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-weight: 400;
  }
  .link.mono {
    flex: 1;
    min-width: 0;
  }
  .del {
    color: var(--bad);
    font-size: 1.1rem;
  }
  .notice {
    margin-top: 0.75rem;
  }
</style>
