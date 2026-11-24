<script lang="ts">
  // Настройки игры: всё, кроме уровней.
  import Dur from './Dur.svelte'
  import Field from './Field.svelte'
  import { COLUMNS, DEFAULTS, ID_RE, NORMALIZE, slugify } from './model'
  import { useEditor } from './state.svelte'

  let { existingSlug }: { existingSlug?: string } = $props()
  const ed = useEditor()
  const d = $derived(ed.draft)

  // Для нового сценария id подставляется из названия, пока его не правили руками.
  let idTouched = $state(false)
  $effect(() => {
    if (!existingSlug && !idTouched) d.id = slugify(d.title)
  })

  const columns = $derived(d.leaderboard.columns ?? DEFAULTS.columns)
  function toggleColumn(c: string) {
    const set = new Set(columns)
    if (set.has(c)) set.delete(c)
    else set.add(c)
    d.leaderboard.columns = COLUMNS.map(([k]) => k).filter((k) => set.has(k))
  }

  const norm = $derived(d.defaults.answer_normalize ?? DEFAULTS.normalize)
  function toggleNorm(op: string) {
    const set = new Set(norm)
    if (set.has(op)) set.delete(op)
    else set.add(op)
    d.defaults.answer_normalize = NORMALIZE.map(([k]) => k).filter((k) => set.has(k))
  }

  const solo = $derived(d.participants.mode === 'solo')
  const rate = $derived(d.defaults.rate_limit)
</script>

<div class="settings">
  <h2>Настройки игры</h2>

  <section class="card grid2">
    <Field label="Название" path="title" wide>
      <input bind:value={d.title} placeholder="Горячий Ключ: пилот" />
    </Field>
    <Field
      label="id сценария"
      path="id"
      hint={existingSlug && d.id !== existingSlug
        ? `Внимание: с другим id сохранится новый сценарий, а не версия «${existingSlug}»`
        : 'Латиница, цифры, - и _. По id версии одного сценария связаны между собой'}
    >
      <input class="mono" class:bad={!!d.id && !ID_RE.test(d.id)} bind:value={d.id} oninput={() => (idTouched = true)} />
    </Field>
    <Field label="Описание (для ведущего)" path="description" wide>
      <textarea rows="2" bind:value={d.description}></textarea>
    </Field>
  </section>

  <section class="card grid3">
    <h3 class="wide">Участники</h3>
    <Field label="Формат" path="participants.mode">
      <select bind:value={d.participants.mode}>
        <option value="team">команды</option>
        <option value="solo">соло</option>
      </select>
    </Field>
    {#if !solo && d.participants.team_size}
      <Field label="Игроков в команде: от" path="participants.team_size.min">
        <input type="number" min="1" bind:value={d.participants.team_size.min} placeholder="1" />
      </Field>
      <Field label="до" path="participants.team_size.max">
        <input type="number" min="1" bind:value={d.participants.team_size.max} placeholder="5" />
      </Field>
      <Field label="Штрафные подсказки берёт" path="participants.hint_requests">
        <select bind:value={d.participants.hint_requests}>
          <option value={undefined}>только капитан</option>
          <option value="anyone">любой игрок</option>
        </select>
      </Field>
    {/if}
  </section>

  <section class="card grid3">
    <h3 class="wide">Зачёт и время</h3>
    <Field label="Зачёт" path="scoring.mode" hint={ed.scoring === 'time'
        ? 'Как в Энкаунтере: больше уровней, при равенстве — меньше время. Штрафы и бонусы — временем'
        : ed.scoring === 'points'
          ? 'Больше баллов; при равенстве — больше уровней'
          : 'Больше баллов; при равенстве — меньше время'}>
      <select bind:value={d.scoring.mode}>
        <option value="time">по времени</option>
        <option value="points">по баллам</option>
        <option value="mixed">баллы + время</option>
      </select>
    </Field>
    <Field label="Лимит времени команды" path="timing.time_limit" hint="От старта команды; после — автофиниш. Пусто — без лимита">
      <Dur bind:value={d.timing.time_limit} placeholder="3h" />
    </Field>
  </section>

  <section class="card grid3">
    <h3 class="wide">Старт и маршрут</h3>
    <Field label="Старт команд" path="start.mode">
      <select bind:value={d.start.mode}>
        <option value={undefined}>все одновременно</option>
        <option value="staggered">по очереди с интервалом</option>
        <option value="manual">вручную, по кнопке у команды</option>
      </select>
    </Field>
    {#if d.start.mode === 'staggered'}
      <Field label="Интервал" path="start.interval">
        <Dur bind:value={d.start.interval} placeholder="5m" />
      </Field>
      <Field label="Порядок старта" path="start.order">
        <select bind:value={d.start.order}>
          <option value={undefined}>по регистрации</option>
          <option value="random">случайный</option>
          <option value="manual">задаёт ведущий</option>
        </select>
      </Field>
    {/if}
    <Field label="Как открываются уровни" path="flow.mode" wide>
      <select bind:value={d.flow.mode}>
        <option value="linear">цепочкой — по одному</option>
        <option value="storm">штурм — все сразу</option>
        <option value="graph">по условиям (граф)</option>
      </select>
    </Field>
    {#if ed.flow === 'linear'}
      <Field
        label="Маршруты команд"
        path="flow.routes"
        wide
        hint={d.flow.routes === 'rotate'
          ? 'Команда N начинает с N-го уровня и идёт по кругу. Хорошо, если точки стоят кольцом'
          : d.flow.routes === 'random'
            ? 'Длина маршрутов может сильно отличаться — осторожно с зачётом по времени'
            : d.flow.routes === 'explicit'
              ? 'Маршруты задаются на вкладке «Маршруты»'
              : 'Все проходят уровни в порядке списка'}
      >
        <select bind:value={d.flow.routes}>
          <option value={undefined}>одинаковые у всех</option>
          <option value="rotate">ротация — разнести команды по точкам</option>
          <option value="random">случайные</option>
          <option value="explicit">задать вручную</option>
        </select>
      </Field>
    {/if}
  </section>

  <section class="card grid3">
    <h3 class="wide">Таблица результатов</h3>
    <Field label="Игроки видят таблицу" path="leaderboard.visibility" hint="Ведущий может включить или скрыть её кнопкой во время игры">
      <select bind:value={d.leaderboard.visibility}>
        <option value={undefined}>всю игру</option>
        <option value="after_finish">после финиша своей команды</option>
        <option value="hidden">после окончания игры</option>
        <option value="host">когда включит ведущий</option>
      </select>
    </Field>
    <Field label="Заморозить за … до конца" path="leaderboard.freeze_before_end" hint="Игроки видят снимок на момент заморозки">
      <Dur bind:value={d.leaderboard.freeze_before_end} placeholder="30m" />
    </Field>
    <Field label="Колонки" path="leaderboard.columns">
      <div class="chips">
        {#each COLUMNS as [k, t] (k)}
          <label class="chk"><input type="checkbox" checked={columns.includes(k)} onchange={() => toggleColumn(k)} /> {t}</label>
        {/each}
      </div>
    </Field>
  </section>

  <section class="card grid3">
    <h3 class="wide">Ответы и подсказки по умолчанию</h3>
    <Field label="Сравнение ответов" path="defaults.answer_normalize" wide>
      <div class="chips">
        {#each NORMALIZE as [k, t] (k)}
          <label class="chk"><input type="checkbox" checked={norm.includes(k)} onchange={() => toggleNorm(k)} /> {t}</label>
        {/each}
      </div>
    </Field>
    <Field
      label="Расписание автоподсказок"
      path="defaults.hint_after"
      hint="Через сколько после открытия уровня откроется 1-я, 2-я… подсказка по таймеру, если у неё не указано своё время. Через запятую"
    >
      <input
        class="mono"
        placeholder="15m, 30m"
        value={(d.defaults.hint_after ?? []).join(', ')}
        onchange={(e) =>
          (d.defaults.hint_after = e.currentTarget.value
            .split(/[,;\s]+/)
            .filter(Boolean)
            .map((x) => (/^\d+$/.test(x) ? x + 'm' : x)))}
      />
    </Field>
    {#if rate}
      <!-- Старые сценарии: общая защита на всю игру. Новую задают у уровней. -->
      <Field label="Общая защита от перебора" path="defaults.rate_limit" wide>
        <div class="row wrap">
          <span>
            {rate.wrong_answers ?? '?'} неверных кодов за {rate.per ?? '?'} — ввод закрыт на всех уровнях без своей защиты.
          </span>
          <button class="ghost small" onclick={() => delete d.defaults.rate_limit}>Убрать</button>
        </div>
        <div class="hint">Защиту лучше задавать у нужных уровней: там же выбирается действие — блокировка или штраф.</div>
      </Field>
    {/if}
  </section>
</div>

<style>
  .settings {
    max-width: 920px;
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.25rem 1rem;
  }
  input.bad {
    border-color: var(--bad);
  }
</style>
