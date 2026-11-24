<script lang="ts">
  // Защита уровня от перебора кодов: N неверных за время → блокировка ввода или штраф.
  import Dur from './Dur.svelte'
  import Field from './Field.svelte'
  import { humanDuration, type Level } from './model'
  import { useEditor } from './state.svelte'

  let { level, path }: { level: Level; path: string } = $props()
  const ed = useEditor()
  const P = $derived(`${path}.rate_limit`)

  const hasCodes = $derived(level.answers.some((a) => a.type === 'code' || a.type === 'regex' || a.type === 'choice'))
  const own = $derived(ed.issues.filter((x) => x.path === P).map((x) => x.message))

  function toggle(on: boolean) {
    if (on) level.rate_limit = { wrong_answers: 5, per: '1m', action: 'block', block_for: '5m' }
    else delete level.rate_limit
  }

  function setAction(a: 'block' | 'penalty') {
    const rl = level.rate_limit!
    rl.action = a
    if (a === 'block') {
      delete rl.penalty
      rl.block_for ??= '5m'
    } else {
      delete rl.block_for
      rl.penalty ??= ed.hasPoints && !ed.hasTime ? { points: 5 } : { time: '5m' }
    }
  }

  const summary = $derived.by(() => {
    const rl = level.rate_limit
    if (!rl?.wrong_answers || !rl.per) return ''
    const per = humanDuration(rl.per) || rl.per
    if (rl.action === 'penalty') {
      const pen = rl.penalty?.time ? `+${humanDuration(rl.penalty.time) || rl.penalty.time} к времени` : rl.penalty?.points ? `−${rl.penalty.points} баллов` : 'штраф'
      return `${rl.wrong_answers}-й неверный код за ${per} — ${pen}; счёт начинается заново, ввод не закрывается.`
    }
    const block = rl.block_for || rl.per
    return `${rl.wrong_answers}-й неверный код за ${per} — ввод кодов на уровне закрывается на ${humanDuration(block) || block}.`
  })
</script>

<section class="card" data-path={P}>
  <label class="chk head"><input type="checkbox" checked={!!level.rate_limit} onchange={(e) => toggle(e.currentTarget.checked)} /> <strong>Защита от перебора кодов</strong></label>
  {#each own as m, k (k)}<div class="msg">{m}</div>{/each}
  {#if !level.rate_limit}
    <p class="muted small">{hasCodes ? 'Ограничивает число неверных кодов на этом уровне.' : 'Работает только для ответов-кодов (код, шаблон, выбор).'}</p>
  {:else}
    {@const rl = level.rate_limit}
    <div class="grid3">
      <Field label="Неверных кодов" path="{P}.wrong_answers">
        <input type="number" min="1" bind:value={rl.wrong_answers} />
      </Field>
      <Field label="за" path="{P}.per">
        <Dur bind:value={rl.per} placeholder="1m" />
      </Field>
      <Field label="Что делать" path="{P}.action">
        <select value={rl.action ?? 'block'} onchange={(e) => setAction(e.currentTarget.value as 'block' | 'penalty')}>
          <option value="block">закрыть ввод на время</option>
          <option value="penalty">штраф</option>
        </select>
      </Field>
      {#if (rl.action ?? 'block') === 'block'}
        <Field label="Закрыть ввод на" path="{P}.block_for" hint="Пусто — на то же время, что окно подсчёта">
          <Dur bind:value={rl.block_for} placeholder={rl.per || '5m'} />
        </Field>
      {:else}
        {#if ed.hasTime}
          <Field label="Штраф временем" path="{P}.penalty.time">
            <Dur bind:value={() => rl.penalty?.time, (v) => (rl.penalty = { ...(rl.penalty ?? {}), time: v })} placeholder="5m" />
          </Field>
        {/if}
        {#if ed.hasPoints}
          <Field label="Штраф баллами" path="{P}.penalty.points" hint={ed.scoring === 'mixed' ? 'Укажите что-то одно: время или баллы' : undefined}>
            <input type="number" min="0" bind:value={() => rl.penalty?.points, (v) => (rl.penalty = { ...(rl.penalty ?? {}), points: v })} />
          </Field>
        {/if}
      {/if}
    </div>
    {#if summary}<p class="muted small sum">{summary} Коды, которые вводит ведущий за команду, не считаются.</p>{/if}
  {/if}
</section>

<style>
  .head {
    margin-bottom: 0.25rem;
  }
  .sum {
    margin: 0.5rem 0 0;
  }
</style>
