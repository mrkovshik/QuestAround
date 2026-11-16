<script lang="ts">
  // Картинки уровня / подсказки / бонуса: из библиотеки, загрузить новую или ссылка.
  import { ApiError } from '../../lib/api'
  import { compressImage } from '../../lib/image'
  import { useEditor } from './state.svelte'

  let { value = $bindable() }: { value?: string[] } = $props()
  const ed = useEditor()

  let open = $state(false)
  let url = $state('')
  let busy = $state(false)
  let err = $state('')
  let file: HTMLInputElement | undefined = $state()

  const list = $derived((value ?? []).filter(Boolean))

  function add(name: string) {
    if (!name || list.includes(name)) return
    value = [...list, name]
  }
  function remove(i: number) {
    value = list.filter((_, k) => k !== i)
  }
  function move(i: number, d: number) {
    const a = [...list]
    const j = i + d
    if (j < 0 || j >= a.length) return
    ;[a[i], a[j]] = [a[j], a[i]]
    value = a
  }

  async function upload(files: FileList | null) {
    const arr = [...(files ?? [])]
    if (!arr.length) return
    const replaced = arr.filter((f) => ed.assets.some((a) => a.name === f.name)).map((f) => f.name)
    if (replaced.length && !confirm(`В библиотеке уже есть ${replaced.join(', ')}. Заменить? В идущих играх картинка поменяется сразу.`)) return
    busy = true
    err = ''
    try {
      const prepared = await Promise.all(
        arr.map(async (f) => ({ name: f.name, blob: f.type === 'image/gif' || f.size < 600_000 ? f : await compressImage(f).catch(() => f) })),
      )
      const saved = await ed.api.uploadAssets(prepared)
      await ed.loadAssets()
      saved.forEach((a) => add(a.name))
      open = false
    } catch (e) {
      err = e instanceof ApiError ? e.message : 'Нет связи с сервером'
    } finally {
      busy = false
      if (file) file.value = ''
    }
  }

  function addUrl() {
    const u = url.trim()
    if (!/^https?:\/\//.test(u)) {
      err = 'Ссылка должна начинаться с https://'
      return
    }
    add(u)
    url = ''
    open = false
  }
</script>

<div class="media">
  {#each list as m, i (m)}
    <div class="thumb" title={m}>
      {#if ed.assetURL(m)}
        <img src={ed.assetURL(m)} alt={m} loading="lazy" />
      {:else}
        <div class="missing">нет в библиотеке</div>
      {/if}
      <div class="name mono">{m}</div>
      <div class="tools">
        {#if i > 0}<button type="button" class="ghost small" title="Раньше" onclick={() => move(i, -1)}>←</button>{/if}
        {#if i < list.length - 1}<button type="button" class="ghost small" title="Позже" onclick={() => move(i, 1)}>→</button>{/if}
        <button type="button" class="ghost small del" title="Убрать" onclick={() => remove(i)}>✕</button>
      </div>
    </div>
  {/each}
  <button type="button" class="add" onclick={() => (open = !open)}>＋ картинка</button>
</div>

{#if open}
  <div class="picker">
    <div class="row wrap">
      <button type="button" class="secondary small" disabled={busy} onclick={() => file?.click()}>{busy ? 'Загружаем…' : 'Загрузить с компьютера'}</button>
      <input bind:this={file} type="file" accept="image/*" multiple hidden onchange={(e) => upload(e.currentTarget.files)} />
      <span class="grow"></span>
      <input class="url" placeholder="или ссылка https://…" bind:value={url} onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), addUrl())} />
      <button type="button" class="ghost small" disabled={!url.trim()} onclick={addUrl}>Добавить</button>
    </div>
    {#if err}<div class="msg">{err}</div>{/if}
    {#if ed.assets.length}
      <div class="lib">
        {#each ed.assets as a (a.name)}
          <button type="button" class="libitem" class:picked={list.includes(a.name)} title={a.name} onclick={() => (add(a.name), (open = false))}>
            <img src={a.url} alt={a.name} loading="lazy" />
            <span class="mono">{a.name}</span>
          </button>
        {/each}
      </div>
    {:else}
      <p class="muted small">Библиотека пуста — загрузите картинку.</p>
    {/if}
    <p class="muted small">Внешние ссылки из РФ могут не открываться — надёжнее библиотека.</p>
  </div>
{/if}

<style>
  .media {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    align-items: stretch;
  }
  .thumb {
    width: 120px;
    border: 1px solid var(--line);
    border-radius: 10px;
    overflow: hidden;
    background: var(--bg);
    font-size: 0.75rem;
  }
  .thumb img,
  .missing {
    display: block;
    width: 100%;
    height: 80px;
    object-fit: cover;
  }
  .missing {
    display: grid;
    place-items: center;
    color: var(--bad);
    background: var(--bad-soft);
  }
  .name {
    padding: 0.2rem 0.4rem 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .tools {
    display: flex;
    justify-content: flex-end;
  }
  .tools button {
    min-height: 24px;
    padding: 0 0.4rem;
  }
  .del {
    color: var(--bad);
  }
  .add {
    width: 120px;
    min-height: 80px;
    background: transparent;
    border: 1.5px dashed var(--line);
    color: var(--muted);
  }
  .picker {
    margin-top: 0.5rem;
    padding: 0.75rem;
    border: 1px solid var(--line);
    border-radius: 10px;
    background: var(--bg);
  }
  .url {
    max-width: 280px;
  }
  .lib {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(96px, 1fr));
    gap: 0.4rem;
    margin-top: 0.6rem;
    max-height: 260px;
    overflow: auto;
  }
  .libitem {
    display: flex;
    flex-direction: column;
    padding: 0;
    min-height: 0;
    background: var(--card);
    border: 2px solid transparent;
    color: var(--text);
    font-weight: 400;
    font-size: 0.7rem;
    overflow: hidden;
  }
  .libitem.picked {
    border-color: var(--accent);
  }
  .libitem img {
    width: 100%;
    height: 64px;
    object-fit: cover;
  }
  .libitem span {
    padding: 0.15rem 0.3rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .msg {
    color: var(--bad);
    font-size: 0.85rem;
    margin-top: 0.4rem;
  }
</style>
