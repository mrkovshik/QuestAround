<script lang="ts">
  // Поле формы: подпись, подсказка и ошибки сервера по пути (levels[0].task и т.п.).
  import type { Snippet } from 'svelte'
  import { useEditor } from './state.svelte'

  let {
    label,
    path,
    hint,
    wide = false,
    children,
  }: { label?: string; path: string; hint?: string; wide?: boolean; children: Snippet } = $props()

  const ed = useEditor()
  const errs = $derived(ed.at(path))
</script>

<div class="field" class:wide class:err={errs.length > 0} data-path={path}>
  {#if label}<div class="lbl">{label}</div>{/if}
  {@render children()}
  {#if hint}<div class="hint">{hint}</div>{/if}
  {#each errs as m, i (i)}<div class="msg">{m}</div>{/each}
</div>
