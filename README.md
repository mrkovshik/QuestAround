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
  core/                   (todo) движок: события, актор на Run, flow, scoring
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

## Дальше

1. `core`: типы событий, проекция состояния команды, проверка ответов (`Normalize` уже есть).
2. Актор на Run: команды → события, таймеры подсказок/лимита/стартов.
3. `store`: схема Postgres, миграции, append-only журнал.
4. `transport/http` + PWA.
