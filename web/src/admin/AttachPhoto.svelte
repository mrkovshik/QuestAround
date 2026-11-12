<script lang="ts">
  // Кнопка «приложить фото» с превью; файл — через bind:file.
  let { file = $bindable(null) }: { file: File | null } = $props()
  let input: HTMLInputElement | undefined = $state()
  const url = $derived(file ? URL.createObjectURL(file) : null)
  $effect(() => {
    const u = url
    return () => {
      if (u) URL.revokeObjectURL(u)
    }
  })
  $effect(() => {
    if (!file && input) input.value = ''
  })
</script>

{#if url}
  <span class="att">
    <img src={url} alt="" />
    <button type="button" class="x" title="Убрать фото" onclick={() => (file = null)}>×</button>
  </span>
{:else}
  <button type="button" class="ghost clip" title="Приложить фото" onclick={() => input?.click()}>📷</button>
{/if}
<input bind:this={input} type="file" accept="image/*" hidden onchange={() => (file = input?.files?.[0] ?? null)} />

<style>
  .clip {
    padding: 0 0.6rem;
    font-size: 1.1rem;
  }
  .att {
    position: relative;
    display: inline-block;
    flex: none;
  }
  .att img {
    width: 42px;
    height: 42px;
    object-fit: cover;
    border-radius: 8px;
    display: block;
  }
  .x {
    position: absolute;
    top: -8px;
    right: -8px;
    min-height: 0;
    width: 20px;
    height: 20px;
    padding: 0;
    border-radius: 50%;
    background: var(--bad);
    color: #fff;
    font-size: 0.8rem;
    line-height: 20px;
  }
</style>
