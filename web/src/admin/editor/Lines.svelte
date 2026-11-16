<script lang="ts">
  // Список строк — по одной в строке textarea.
  let {
    value = $bindable(),
    rows = 3,
    placeholder = '',
    mono = false,
  }: { value?: string[]; rows?: number; placeholder?: string; mono?: boolean } = $props()
</script>

<textarea
  class:mono
  rows={Math.max(rows, (value?.length ?? 0) + 1)}
  {placeholder}
  value={(value ?? []).join('\n')}
  oninput={(e) => (value = e.currentTarget.value.split('\n'))}
></textarea>
