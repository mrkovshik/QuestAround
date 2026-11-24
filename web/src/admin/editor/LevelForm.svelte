<script lang="ts">
  // Форма уровня: задание, ответы, подсказки, бонусы, условия маршрута.
  import AnswerEdit from './AnswerEdit.svelte'
  import Dur from './Dur.svelte'
  import Field from './Field.svelte'
  import MediaPicker from './MediaPicker.svelte'
  import RateLimitEdit from './RateLimitEdit.svelte'
  import SlivEdit from './SlivEdit.svelte'
  import { ID_RE, cloneLevel, humanDuration, scheduledAfter, newAnswer, removeLevelRefs, renameLevel, uniqueId, type Hint, type Level } from './model'
  import { useEditor } from './state.svelte'

  let { level, index }: { level: Level; index: number } = $props()
  const ed = useEditor()
  const P = $derived(`levels[${index}]`)
  const d = $derived(ed.draft)
  const ownErrs = $derived(ed.issues.filter((x) => x.path === P).map((x) => x.message))
  const others = $derived(d.levels.filter((l) => l !== level))
  const hintAfter = $derived((d.defaults.hint_after ?? []).filter(Boolean))

  // id: правим черновик, переименование ссылок — по blur.
  let idBefore = ''
  function idFocus() {
    idBefore = level.id
  }
  function idBlur() {
    const v = level.id.trim().toLowerCase()
    level.id = v
    if (v && v !== idBefore) renameLevel(d, idBefore, v)
  }

  function remove() {
    if (!confirm(`Удалить уровень «${level.title || level.id}»?`)) return
    removeLevelRefs(d, level.id)
    d.levels.splice(index, 1)
    ed.view = d.levels.length ? Math.min(index, d.levels.length - 1) : 'settings'
  }

  function duplicate() {
    d.levels.splice(index + 1, 0, cloneLevel(d, $state.snapshot(level) as Level))
    ed.view = index + 1
  }

  function addHint() {
    level.hints = [...(level.hints ?? []), { text: '' }]
  }

  const isZero = (v?: string) => !!v && /^0+[smh]?$/.test(v.trim())
  const hasAfter = (h: Hint) => !!h.after?.trim()

  // Время из расписания (defaults.hint_after) для i-й подсказки, если она его использует:
  // расписание раздаётся по порядку подсказкам без «по запросу» — как на сервере.
  const scheduled = (i: number) => scheduledAfter(level, i, hintAfter)

  function summary(h: Hint, sched?: string): string {
    const auto = hasAfter(h) && !isZero(h.after) ? h.after : !h.on_request && !hasAfter(h) ? sched : undefined
    if (h.on_request) {
      const btn = h.available_after && !isZero(h.available_after) ? ` (кнопка через ${humanDuration(h.available_after) || h.available_after})` : ''
      return (auto ? `по запросу${btn} или сама через ${humanDuration(auto) || auto}` : `только по запросу${btn}`)
    }
    if (isZero(h.after)) return 'не откроется'
    return auto ? `сама через ${humanDuration(auto) || auto}` : 'нет времени'
  }

  function afterHint(h: Hint, sched?: string): string {
    if (h.on_request)
      return '0 или пусто — только по запросу. Если указать время, подсказка откроется сама и без штрафа, когда команда не взяла её раньше.'
    if (isZero(h.after)) return '0 — не выдавать автоматически. Тогда включите «Выдавать по запросу», иначе подсказка не откроется никогда.'
    const zero = ' 0 — не выдавать автоматически (только вместе с «по запросу»).'
    return sched
      ? `Пусто — по расписанию из настроек игры: через ${humanDuration(sched) || sched}.${zero}`
      : `Укажите время или задайте расписание подсказок в настройках игры.${zero}`
  }

  function setRequest(h: Hint, on: boolean, sched?: string) {
    if (on) {
      h.on_request = true
      h.penalty ??= ed.hasPoints && !ed.hasTime ? { points: 5 } : { time: '10m' }
      // подсказка по расписанию не перестаёт открываться сама — фиксируем её время
      if (!hasAfter(h) && sched) h.after = sched
    } else {
      delete h.on_request
      delete h.penalty
      delete h.available_after
      if (isZero(h.after)) delete h.after
    }
  }

  function addBonus() {
    const ids = d.levels.flatMap((l) => (l.bonuses ?? []).map((b) => b.id))
    const b = {
      id: uniqueId(`${level.id}-bonus`, ids),
      title: '',
      answer: newAnswer('photo'),
      ...(ed.hasPoints && !ed.hasTime ? { points: 5 } : { time: '5m' }),
    }
    level.bonuses = [...(level.bonuses ?? []), b]
  }

  function moveItem<T>(arr: T[] | undefined, i: number, dir: number) {
    if (!arr) return
    const j = i + dir
    if (j < 0 || j >= arr.length) return
    ;[arr[i], arr[j]] = [arr[j], arr[i]]
  }

  // unlock (graph): одна форма из трёх.
  type UMode = 'none' | 'all_of' | 'any_of' | 'count'
  const umode = $derived<UMode>(
    !level.unlock ? 'none' : level.unlock.count || level.unlock.of ? 'count' : level.unlock.any_of ? 'any_of' : level.unlock.all_of ? 'all_of' : 'none',
  )
  const udeps = $derived(level.unlock?.all_of ?? level.unlock?.any_of ?? level.unlock?.of ?? [])
  function setUMode(m: UMode) {
    const deps = [...udeps]
    if (m === 'none') delete level.unlock
    else if (m === 'count') level.unlock = { count: Math.min(2, Math.max(1, deps.length)), of: deps }
    else level.unlock = { [m]: deps }
  }
  function toggleDep(id: string) {
    const deps = udeps.includes(id) ? udeps.filter((x) => x !== id) : [...udeps, id]
    if (umode === 'count') level.unlock!.of = deps
    else if (umode !== 'none') level.unlock = { [umode]: deps }
  }

  const reqValue = $derived(level.require == null || level.require === '' ? 'all' : String(level.require))
  function setRequire(v: string) {
    level.require = v === 'all' ? undefined : v === 'any' ? 'any' : Number(v)
  }

  const idBad = $derived(!!level.id && !ID_RE.test(level.id))
</script>

<div class="lvl">
  <div class="row head">
    <h2 class="grow">Уровень {index + 1}</h2>
    <button class="ghost small" onclick={duplicate}>Дублировать</button>
    <button class="ghost small del" onclick={remove}>Удалить</button>
  </div>
  {#each ownErrs as m, i (i)}<div class="msg block" data-path={P}>{m}</div>{/each}

  <section class="card grid2">
    <Field label="Название" path="{P}.title">
      <input bind:value={level.title} placeholder="Мост" />
    </Field>
    <Field label="id (латиница — для маршрутов и журнала)" path="{P}.id" hint={idBad ? 'Только строчные латинские буквы, цифры, - и _' : undefined}>
      <input class="mono" class:bad={idBad} bind:value={level.id} onfocus={idFocus} onblur={idBlur} />
    </Field>
    <Field label="Задание" path="{P}.task" wide>
      <textarea rows="5" bind:value={level.task} placeholder="Что нужно найти или сделать"></textarea>
    </Field>
    <Field label="Заметки для организаторов" path="{P}.host_notes" wide hint="Игроки не видят. Где спрятан конверт, кто стоит на точке, что проверить перед игрой">
      <textarea class="host-notes" rows="2" bind:value={level.host_notes}></textarea>
    </Field>
    <Field label="Картинки к заданию" path="{P}.media" wide>
      <MediaPicker bind:value={level.media} />
    </Field>
    {#if ed.hasPoints}
      <Field label="Баллы за уровень" path="{P}.points">
        <input type="number" min="0" bind:value={level.points} />
      </Field>
    {:else if level.points}
      <Field label="Баллы" path="{P}.points" hint="В зачёте по времени баллов нет">
        <button class="ghost small del" onclick={() => delete level.points}>Убрать баллы ({level.points})</button>
      </Field>
    {/if}
  </section>

  {#if ed.flow === 'linear'}
    <section class="card grid2">
      <h3 class="wide">Место в маршруте</h3>
      <Field label="Закрепить" path="{P}.pin" hint="Общий старт и общий финиш у всех команд">
        <select value={level.pin ?? ''} onchange={(e) => (level.pin = (e.currentTarget.value || undefined) as Level['pin'])}>
          <option value="">нет — порядок по правилу маршрутов</option>
          <option value="first">первым у всех (старт)</option>
          <option value="last">последним у всех (финиш)</option>
        </select>
      </Field>
      <Field label="Цепочка" path="{P}.chain" hint="Уровни с одним именем цепочки всегда идут подряд — если задание ссылается на предыдущую точку">
        <input class="mono" bind:value={level.chain} placeholder="например, riddle" list="chains" disabled={!!level.pin} />
        <datalist id="chains">
          {#each [...new Set(d.levels.map((l) => l.chain).filter(Boolean))] as c (c)}<option value={c}></option>{/each}
        </datalist>
      </Field>
    </section>
  {:else if ed.flow === 'graph'}
    <section class="card">
      <h3>Когда открывается</h3>
      <Field label="Условие" path="{P}.unlock">
        <select value={umode} onchange={(e) => setUMode(e.currentTarget.value as UMode)}>
          <option value="none">сразу (стартовый уровень)</option>
          <option value="all_of">после всех отмеченных</option>
          <option value="any_of">после любого из отмеченных</option>
          <option value="count">после N из отмеченных</option>
        </select>
      </Field>
      {#if umode !== 'none'}
        <div class="deps">
          {#each others as o (o)}
            <label class="chk"><input type="checkbox" checked={udeps.includes(o.id)} onchange={() => toggleDep(o.id)} /> {o.title || o.id}</label>
          {/each}
        </div>
        {#if umode === 'count' && level.unlock}
          <Field label="Сколько нужно пройти" path="{P}.unlock.count">
            <input type="number" min="1" max={udeps.length} bind:value={level.unlock.count} style="width:6rem" />
          </Field>
        {/if}
      {/if}
    </section>
  {/if}

  <section class="card">
    <div class="row"><h3 class="grow">Ответы</h3></div>
    {#each level.answers as a, i (a)}
      <div class="item">
        <div class="row itemhead">
          <span class="muted small">Ответ {i + 1}</span>
          <span class="grow"></span>
          {#if level.answers.length > 1}
            <button class="ghost small" onclick={() => moveItem(level.answers, i, -1)} disabled={i === 0}>↑</button>
            <button class="ghost small" onclick={() => moveItem(level.answers, i, 1)} disabled={i === level.answers.length - 1}>↓</button>
            <button class="ghost small del" onclick={() => level.answers.splice(i, 1)}>Удалить</button>
          {/if}
        </div>
        <AnswerEdit answer={a} path="{P}.answers[{i}]" />
      </div>
    {/each}
    <div class="row wrap">
      <button class="secondary small" onclick={() => level.answers.push(newAnswer('code'))}>＋ ответ</button>
      {#if level.answers.length > 1}
        <span class="grow"></span>
        <Field label="Для зачёта нужно" path="{P}.require">
          <select value={reqValue} onchange={(e) => setRequire(e.currentTarget.value)}>
            <option value="all">все ответы</option>
            <option value="any">любой один</option>
            {#each Array.from({ length: level.answers.length - 2 }, (_, k) => k + 2) as n (n)}<option value={String(n)}>{n} из {level.answers.length}</option>{/each}
          </select>
        </Field>
      {/if}
    </div>
  </section>

  <section class="card">
    <h3>Подсказки</h3>
    {#each level.hints ?? [] as h, i (h)}
      {@const hp = `${P}.hints[${i}]`}
      {@const sched = scheduled(i)}
      <div class="item" data-path={hp}>
        <div class="row itemhead">
          <strong class="small">Подсказка {i + 1}</strong>
          <span class="badge" class:warn={h.on_request}>{summary(h, sched)}</span>
          <span class="grow"></span>
          <button class="ghost small" onclick={() => moveItem(level.hints, i, -1)} disabled={i === 0}>↑</button>
          <button class="ghost small" onclick={() => moveItem(level.hints, i, 1)} disabled={i === (level.hints?.length ?? 0) - 1}>↓</button>
          <button class="ghost small del" onclick={() => level.hints?.splice(i, 1)}>Удалить</button>
        </div>
        {#each ed.issues.filter((x) => x.path === hp) as x, k (k)}<div class="msg">{x.message}</div>{/each}
        <div class="grid2">
          <div class="wide" data-path="{hp}.on_request">
            <label class="chk"><input type="checkbox" checked={!!h.on_request} onchange={(e) => setRequest(h, e.currentTarget.checked, sched)} /> Выдавать по запросу</label>
          </div>
          {#if h.on_request}
            {#if ed.hasTime}
              <Field label="Штраф временем" path="{hp}.penalty.time">
                <Dur bind:value={() => h.penalty?.time, (v) => (h.penalty = { ...(h.penalty ?? {}), time: v })} placeholder="10m" />
              </Field>
            {/if}
            {#if ed.hasPoints}
              <Field label="Штраф баллами" path="{hp}.penalty.points" hint={ed.scoring === 'mixed' ? 'Укажите что-то одно: время или баллы' : undefined}>
                <input type="number" min="0" bind:value={() => h.penalty?.points, (v) => (h.penalty = { ...(h.penalty ?? {}), points: v })} />
              </Field>
            {/if}
            <Field label="Кнопка появится через" path="{hp}.available_after" hint="Пусто — сразу. До этого игроки видят обратный отсчёт">
              <Dur bind:value={h.available_after} placeholder="0" />
            </Field>
          {/if}
          <Field label="Открыть автоматически через" path="{hp}.after" hint={afterHint(h, sched)}>
            <Dur bind:value={h.after} placeholder={h.on_request ? '0' : (sched ?? '15m')} />
          </Field>
          <Field label="Текст" path="{hp}.text" wide>
            <textarea rows="2" bind:value={h.text}></textarea>
          </Field>
          <Field label="Картинки (видны только после открытия)" path="{hp}.media" wide>
            <MediaPicker bind:value={h.media} />
          </Field>
        </div>
      </div>
    {/each}
    <button class="secondary small" onclick={addHint}>＋ подсказка</button>
  </section>

  <SlivEdit {level} path={P} />

  <RateLimitEdit {level} path={P} />

  <section class="card">
    <h3>Бонусы</h3>
    {#each level.bonuses ?? [] as b, i (b)}
      {@const bp = `${P}.bonuses[${i}]`}
      <div class="item" data-path={bp}>
        <div class="row itemhead">
          <span class="muted small">Бонус {i + 1}</span>
          <span class="grow"></span>
          <button class="ghost small del" onclick={() => level.bonuses?.splice(i, 1)}>Удалить</button>
        </div>
        {#each ed.issues.filter((x) => x.path === bp) as x, k (k)}<div class="msg">{x.message}</div>{/each}
        <div class="grid2">
          <Field label="Название" path="{bp}.title">
            <input bind:value={b.title} placeholder="Командное фото" />
          </Field>
          <Field label="id" path="{bp}.id">
            <input class="mono" bind:value={b.id} />
          </Field>
          {#if ed.hasTime}
            <Field label="Вычесть из времени команды" path="{bp}.time">
              <Dur bind:value={b.time} placeholder="5m" />
            </Field>
          {/if}
          {#if ed.hasPoints}
            <Field label="Баллы" path="{bp}.points">
              <input type="number" min="0" bind:value={b.points} />
            </Field>
          {:else if b.points}
            <Field label="Баллы" path="{bp}.points">
              <button class="ghost small del" onclick={() => delete b.points}>Убрать баллы ({b.points})</button>
            </Field>
          {/if}
          <div class="wide"><AnswerEdit answer={b.answer} path="{bp}.answer" /></div>
          <Field label="Пример / иллюстрация" path="{bp}.media" wide>
            <MediaPicker bind:value={b.media} />
          </Field>
        </div>
      </div>
    {/each}
    <button class="secondary small" onclick={addBonus}>＋ бонус</button>
  </section>
</div>

<style>
  .lvl {
    max-width: 920px;
  }
  .head {
    margin-bottom: 0.5rem;
  }
  .head h2 {
    margin: 0;
  }
  .host-notes {
    background: var(--warn-soft);
  }
  .del {
    color: var(--bad);
  }
  .item {
    border: 1px solid var(--line);
    border-radius: 10px;
    padding: 0.75rem;
    margin-bottom: 0.75rem;
    background: var(--bg);
  }
  .itemhead {
    margin-bottom: 0.5rem;
  }
  .deps {
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem 1rem;
    margin: 0.5rem 0;
  }
  input.bad {
    border-color: var(--bad);
  }
  .msg.block {
    margin-bottom: 0.75rem;
  }
</style>
