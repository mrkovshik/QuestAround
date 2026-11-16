<script lang="ts">
  // YAML сценария: просмотр, правка руками, импорт и экспорт файла.
  import { fromYaml, toYaml } from './model'
  import { useEditor } from './state.svelte'

  const ed = useEditor()
  const generated = $derived(toYaml(ed.draft))

  let text = $state<string | null>(null) // null — показываем сгенерированный
  let err = $state('')
  let file: HTMLInputElement | undefined = $state()

  function apply(src: string) {
    try {
      ed.draft = fromYaml(src)
      text = null
      err = ''
    } catch (e) {
      err = (e as Error).message
    }
  }

  async function open(files: FileList | null) {
    const f = files?.[0]
    if (!f) return
    if (!confirm(`Заменить текущий сценарий содержимым «${f.name}»?`)) return
    apply(await f.text())
    if (file) file.value = ''
  }

  function download() {
    const blob = new Blob([generated], { type: 'text/yaml' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `${ed.draft.id || 'scenario'}.yaml`
    a.click()
    URL.revokeObjectURL(a.href)
  }
</script>

<div class="yaml">
  <div class="row wrap">
    <h2 class="grow">YAML</h2>
    <button class="ghost small" onclick={() => file?.click()}>Открыть файл…</button>
    <input bind:this={file} type="file" accept=".yaml,.yml,text/yaml" hidden onchange={(e) => open(e.currentTarget.files)} />
    <button class="ghost small" onclick={download}>Скачать .yaml</button>
  </div>
  <p class="muted small">
    Всё, что вы делаете в конструкторе, — это этот файл. Можно править прямо здесь и нажать «Применить».
    Комментарии в YAML конструктор не сохраняет.
  </p>
  <textarea class="mono" spellcheck="false" value={text ?? generated} oninput={(e) => (text = e.currentTarget.value)}></textarea>
  {#if err}<div class="msg">{err}</div>{/if}
  {#if text !== null}
    <div class="row">
      <button onclick={() => apply(text!)}>Применить</button>
      <button class="ghost" onclick={() => ((text = null), (err = ''))}>Отменить правки</button>
    </div>
  {/if}
</div>

<style>
  .yaml {
    max-width: 1000px;
  }
  textarea {
    min-height: 65vh;
    font-size: 0.85rem;
    line-height: 1.5;
    resize: vertical;
    white-space: pre;
    tab-size: 2;
  }
  .msg {
    white-space: pre-wrap;
    font-family: ui-monospace, monospace;
  }
</style>
