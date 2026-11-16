<script lang="ts">
  // Длительность в формате Go (15m, 1h30m). Число без единиц — минуты.
  import { fixDuration, humanDuration } from './model'

  let { value = $bindable(), placeholder = '15m' }: { value?: string; placeholder?: string } = $props()
  const human = $derived(humanDuration(value))
</script>

<div class="dur">
  <input
    class="mono"
    {placeholder}
    value={value ?? ''}
    oninput={(e) => (value = e.currentTarget.value)}
    onblur={() => (value = fixDuration(value) || undefined)}
  />
  {#if human}<span class="muted small">{human}</span>{/if}
</div>

<style>
  .dur {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .dur input {
    width: 7.5rem;
  }
  .dur span {
    white-space: nowrap;
  }
</style>
