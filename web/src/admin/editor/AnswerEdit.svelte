<script lang="ts">
  // Один ответ уровня или бонуса: поля зависят от типа.
  import Field from './Field.svelte'
  import Lines from './Lines.svelte'
  import { ANSWER_TYPES, parsePoint, retype, type Answer, type AnswerType } from './model'
  import { useEditor } from './state.svelte'

  let { answer, path }: { answer: Answer; path: string } = $props()
  const ed = useEditor()

  let showMap = $state(false)
  let coordText = $state('')
  let coordErr = $state(false)

  // Текст координат синхронизируем с point, пока поле не редактируется.
  let editing = false
  $effect(() => {
    const p = answer.point
    if (!editing) coordText = p?.length === 2 ? `${p[0]}, ${p[1]}` : ''
  })

  function setCoords(s: string) {
    coordText = s
    const p = parsePoint(s)
    coordErr = !!s.trim() && !p
    if (p) answer.point = p
    else if (!s.trim()) answer.point = []
  }

  // Другие геоточки сценария — для карты.
  const others = $derived(
    ed.draft.levels.flatMap((l) =>
      [...l.answers, ...(l.bonuses ?? []).map((b) => b.answer)]
        .filter((a) => a !== answer && a?.type === 'geo' && a.point?.length === 2)
        .map((a) => ({ lat: a.point![0], lng: a.point![1], title: l.title || l.id, radius: Number(a.radius_m) || 25 })),
    ),
  )

  const options = $derived((answer.options ?? []).map((o) => o.trim()).filter(Boolean))
</script>

<div class="answer">
  <Field label="Тип ответа" path="{path}.type">
    <select value={answer.type} onchange={(e) => retype(answer, e.currentTarget.value as AnswerType)}>
      {#each ANSWER_TYPES as [v, t] (v)}<option value={v}>{t}</option>{/each}
    </select>
  </Field>

  {#if answer.type === 'code'}
    <Field label="Правильные коды — по одному в строке" path="{path}.values" hint="Сравнение после нормализации (см. настройки игры): регистр и ё не важны.">
      <Lines bind:value={answer.values} rows={2} placeholder="1864" mono />
    </Field>
  {:else if answer.type === 'regex'}
    <Field label="Регулярное выражение" path="{path}.pattern" hint="Синтаксис Go (RE2). Пример: ^(19|20)\d{'{2}'}$ — год с 1900 по 2099.">
      <input class="mono" bind:value={answer.pattern} placeholder="^\d{'{4}'}$" />
    </Field>
  {:else if answer.type === 'choice'}
    <Field label="Варианты — по одному в строке" path="{path}.options">
      <Lines bind:value={answer.options} rows={3} />
    </Field>
    <Field label="Правильный вариант" path="{path}.correct">
      <select bind:value={answer.correct}>
        <option value="">— выберите —</option>
        {#each options as o (o)}<option value={o}>{o}</option>{/each}
      </select>
    </Field>
  {:else if answer.type === 'geo'}
    <div class="geo">
      <Field label="Координаты (широта, долгота)" path="{path}.point" hint="Можно вставить из Яндекс/Google Карт: 44.6338, 39.1352">
        <div class="row">
          <input
            class="mono"
            class:bad={coordErr}
            value={coordText}
            placeholder="44.6338, 39.1352"
            onfocus={() => (editing = true)}
            onblur={() => (editing = false)}
            oninput={(e) => setCoords(e.currentTarget.value)}
          />
          <button type="button" class="secondary small" onclick={() => (showMap = true)}>На карте</button>
        </div>
      </Field>
      <Field label="Радиус зачёта, м" path="{path}.radius_m">
        <input type="number" min="1" bind:value={answer.radius_m} />
      </Field>
      <Field label="Точность GPS не хуже, м" path="{path}.max_accuracy_m" hint="Пусто — не проверять">
        <input type="number" min="0" bind:value={answer.max_accuracy_m} />
      </Field>
    </div>
  {:else}
    <p class="muted small">Проверяет ведущий в разделе «Модерация». До решения у команды статус «на проверке».</p>
  {/if}
</div>

{#if showMap}
  {#await import('./GeoPicker.svelte') then { default: GeoPicker }}
    <GeoPicker
      point={answer.point}
      radius={Number(answer.radius_m) || 25}
      {others}
      onpick={(p) => ((answer.point = p), (showMap = false))}
      onclose={() => (showMap = false)}
    />
  {/await}
{/if}

<style>
  .answer {
    display: grid;
    gap: 0.6rem;
  }
  .geo {
    display: grid;
    grid-template-columns: 2fr 1fr 1fr;
    gap: 0.6rem;
  }
  input.bad {
    border-color: var(--bad);
  }
</style>
