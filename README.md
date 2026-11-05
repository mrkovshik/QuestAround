# questline

Движок городских игр: квесты, фото-охота, квесты-прогулки (позже онлайн-квизы).
Спецификация лежит в проекте «велоигры в Нови Саде»: `архитектура-платформы.md`,
`спецификация-сценарий-и-api.md`, `регистрация-команд.md`.

## Быстрый старт

```sh
go mod tidy                 # подтянуть gopkg.in/yaml.v3 и создать go.sum
make test
go run ./cmd/questline validate internal/scenario/testdata/pilot.yaml
go run ./cmd/questline routes -teams 5 internal/scenario/testdata/pilot.yaml
```

## Структура

```
cmd/questline/            CLI для авторов: validate, routes
internal/
  scenario/               YAML-сценарий: типы, дефолты, валидация с номерами строк
    testdata/pilot.yaml   пример сценария (гибрид, пилот в Горячем Ключе)
  routes/                 маршруты команд: same | rotate | random | explicit, pin, chain
  core/                   движок Run: события, команды, flow, проверка ответов, лидерборд
  store/                  (todo) Postgres: журнал событий + справочники
  transport/http/         (todo) API игрока/ведущего, SSE, PWA
  transport/telegram/     (todo) бот — второй транспорт
```

## Что уже есть

**scenario**
- Строгий парсинг: опечатка в ключе — ошибка (`KnownFields`).
- Все ошибки за один проход, с номером строки и путём: `строка 12: levels[1].answers[0].pattern: …`.
- Значения по умолчанию: team 1–5, hint_requests: captain, start: simultaneous,
  flow: linear + routes: same, scoring: points, leaderboard: always,
  answer_normalize: trim, lower, yo_to_e.
- Автоподсказки без `after` получают время из `defaults.hint_after` по порядку.
- Проверки: id, типы ответов и лишние поля в них, regex компилируется, координаты,
  require, подсказки и штрафы, бонусы, pin/chain, explicit-маршруты, граф unlock
  (ссылки, циклы, стартовый уровень), staggered без interval, заморозка лидерборда.

**routes**
- `rotate`: команды равномерно разнесены по кругу (`offset = i·n/teams`);
  если команд больше, чем подвижных блоков, стартовые точки повторяются равномерно.
- `random`: детерминирован по `seed` и номеру команды (seed хранить в Run).
- Цепочки (`chain`) двигаются одним блоком, `pin: first/last` на месте.
- `Validate` проверяет любой маршрут; тест гоняет все режимы на 1–12 команд.

## Отличия от спецификации v0

- `defaults.hints` → `defaults.hint_after: [15m, 30m]` — расписание для подсказок
  уровня без явного `after` (текст подсказки всегда задаётся в уровне).
- `participants.hint_requests: captain | anyone` — кто берёт штрафные подсказки.
- `choice.correct` — значение из `options` (не индекс).
- `timing.mode: sync` пока отклоняется валидатором.

**core**
- `Game.Handle(now, cmd)` → события или ошибка без изменений состояния; `Replay(events)` → то же состояние.
- Команды: RegisterTeam, StartRun, StartTeam, SubmitAnswer, RequestHint, Moderate,
  Override, SetLeaderboard, SendBroadcast, FinishRun, Tick.
- Flow linear (маршруты из `routes`), storm, graph (all_of / any_of / count+of).
- Ответы: код/regex/choice с нормализацией, гео (радиус + порог точности → `inaccurate`),
  фото/текст → очередь модерации. Секторы (`require`), бонусы, повторы (`duplicate`),
  идемпотентность по `client_event_id`, rate limit на неверные коды.
- Подсказки по таймеру и по запросу (штраф временем/баллами, только капитан — по настройке).
- Старт simultaneous / staggered / manual, лимит времени от старта команды.
- Лидерборд: time / points / mixed, общие места при равенстве, видимость
  (сценарий + переключатель ведущего), заморозка со снимком.
- `NextDeadline()` — когда актору прислать Tick.
- Тесты: после каждого теста журнал переигрывается (напрямую и через JSON) и сверяется с состоянием.

## Решения, принятые в core

- Ответ кодом без `LevelID` ищется по всем открытым уровням (для storm/graph);
  порядок: незасчитанные ответы → бонусы → проверка на повтор.
- Бонусы можно сдавать и на пройденном уровне, пока команда в игре.
- Время команды обрезается лимитом, даже если Tick пришёл с опозданием.
- Заморозка лидерборда считается от конца лимита последней стартовавшей команды.
- В режиме `time` бонусы не уменьшают время (бонусы — только баллы).

## Дальше

1. Актор Run: горутина + канал команд + таймер на `NextDeadline`, подписки для SSE.
2. `store`: схема Postgres, миграции, append-only журнал, загрузка/replay при старте.
3. `transport/http` + PWA.
