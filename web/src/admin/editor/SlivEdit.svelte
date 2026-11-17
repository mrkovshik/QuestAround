<script lang="ts">
  // Сливы уровня: локация (адрес/точка за штраф) и уровень целиком (засчитать без ответа).
  import Dur from './Dur.svelte'
  import Field from './Field.svelte'
  import { parsePoint, type Level, type Penalty } from './model'
  import { useEditor } from './state.svelte'

  let { level, path }: { level: Level; path: string } = $props()
  const ed = useEditor()

  // Точка geo-ответа: если своей нет, игроку покажется она.
  const geoPoint = $derived(level.answers.find((a) => a.type === 'geo' && a.point?.length === 2)?.point)

  const defaultPenalty = (): Penalty => (ed.hasPoints && !ed.hasTime ? { points: 5 } : { time: '15m' })

  function toggleLocation(on: boolean) {
    if (on) level.location = { text: '', penalty: defaultPenalty() }
    else delete level.location
  }
  function toggleSkip(on: boolean) {
    if (on) level.skip = { penalty: ed.hasPoints && !ed.hasTime ? { points: 10 } : { time: '30m' } }
    else delete level.skip
  }

  let showMap = $state(false)
  let coordText = $state('')
  let coordErr = $state(false)
  let editing = false
  $effect(() => {
    const p = level.location?.point
    if (!editing) coordText = p?.length === 2 ? `${p[0]}, ${p[1]}` : ''
  })
  function setCoords(s: string) {
    coordText = s
    const p = parsePoint(s)
    coordErr = !!s.trim() && !p
    if (!level.location) return
    if (p) level.location.point = p
    else if (!s.trim()) delete level.location.point
  }
</script>

{#snippet penalty(pen: Penalty | undefined, set: (p: Penalty) => void, p: string)}
  {#if ed.hasTime}
    <Field label="Штраф временем" path="{p}.penalty.time" hint="Пусто — без штрафа">
      <Dur bind:value={() => pen?.time, (v) => set({ ...(pen ?? {}), time: v })} placeholder="15m" />
    </Field>
  {/if}
  {#if ed.hasPoints}
    <Field label="Штраф баллами" path="{p}.penalty.points" hint={ed.scoring === 'mixed' ? 'Укажите что-то одно: время или баллы' : 'Пусто — без штрафа'}>
      <input type="number" min="0" bind:value={() => pen?.points, (v) => set({ ...(pen ?? {}), points: v })} />
    </Field>
  {/if}
{/snippet}

<section class="card">
  <h3>Сливы</h3>
  <p class="muted small">Кнопки у капитана под подсказками, с подтверждением и штрафом на кнопке.</p>

  <div class="item" data-path="{path}.location">
    <label class="chk">
      <input type="checkbox" checked={!!level.location} onchange={(e) => toggleLocation(e.currentTarget.checked)} />
      <strong>Слив локации</strong> — команда получает место: адрес и/или точку на карте
    </label>
    {#each ed.issues.filter((x) => x.path === `${path}.location`) as x, k (k)}<div class="msg">{x.message}</div>{/each}
    {#if level.location}
      {@const loc = level.location}
      <div class="grid2">
        <Field label="Что показать команде" path="{path}.location.text" wide hint="Адрес или описание места">
          <textarea rows="2" bind:value={loc.text} placeholder="ул. Красная, 5 — во дворе, у старого дуба"></textarea>
        </Field>
        <Field
          label="Точка на карте"
          path="{path}.location.point"
          hint={geoPoint ? `Пусто — покажем точку ответа уровня (${geoPoint[0]}, ${geoPoint[1]})` : 'Необязательно'}
        >
          <div class="row">
            <input
              class="mono"
              class:bad={coordErr}
              value={coordText}
              placeholder={geoPoint ? `${geoPoint[0]}, ${geoPoint[1]}` : '44.6338, 39.1352'}
              onfocus={() => (editing = true)}
              onblur={() => (editing = false)}
              oninput={(e) => setCoords(e.currentTarget.value)}
            />
            <button type="button" class="secondary small" onclick={() => (showMap = true)}>На карте</button>
          </div>
        </Field>
        {@render penalty(loc.penalty, (p) => (loc.penalty = p), `${path}.location`)}
      </div>
    {/if}
  </div>

  <div class="item" data-path="{path}.skip">
    <label class="chk">
      <input type="checkbox" checked={!!level.skip} onchange={(e) => toggleSkip(e.currentTarget.checked)} />
      <strong>Слив уровня</strong> — уровень засчитывается без ответа{ed.hasPoints ? ', баллы за него не начисляются' : ''}
    </label>
    {#if level.skip}
      {@const sk = level.skip}
      <div class="grid2">
        {@render penalty(sk.penalty, (p) => (sk.penalty = p), `${path}.skip`)}
      </div>
    {/if}
  </div>
</section>

{#if showMap && level.location}
  {#await import('./GeoPicker.svelte') then { default: GeoPicker }}
    <GeoPicker
      point={level.location.point?.length === 2 ? level.location.point : geoPoint}
      radius={25}
      others={[]}
      onpick={(p) => (level.location && (level.location.point = p), (showMap = false))}
      onclose={() => (showMap = false)}
    />
  {/await}
{/if}

<style>
  .item {
    border: 1px solid var(--line);
    border-radius: 10px;
    padding: 0.75rem;
    margin-bottom: 0.75rem;
    background: var(--bg);
  }
  .grid2 {
    margin-top: 0.5rem;
  }
</style>
