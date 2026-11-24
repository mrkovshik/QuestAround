"""Прогон конструктора сценариев в headless Chrome:
новый сценарий → ошибки сервера у полей → уровни с кодом, геоточкой, подсказками и бонусом →
перетаскивание → маршруты → сохранение версии 1 → правка → версия 2 → черновик в браузере →
YAML → тестовая игра.

Запуск (на свежем сервере в памяти):
    QUESTAROUND_ADMIN_TOKEN=e2e-admin-token-000001 questaround serve -addr 127.0.0.1:8091 &
    python3 web/e2e/editor_flow.py
"""
import os
import sys

from playwright.sync_api import expect, sync_playwright

BASE = os.environ.get("QA_BASE", "http://127.0.0.1:8091")
ADMIN = os.environ.get("QA_ADMIN_TOKEN", "e2e-admin-token-000001")
CHROME = os.environ.get("QA_CHROME", "/usr/bin/google-chrome")
SHOTS = os.environ.get("QA_SHOTS", "/tmp/questaround-shots")
os.makedirs(SHOTS, exist_ok=True)
errors = []

with sync_playwright() as p:
    browser = p.chromium.launch(executable_path=CHROME, args=["--no-sandbox"])
    ctx = browser.new_context(viewport={"width": 1440, "height": 900}, locale="ru-RU")
    page = ctx.new_page()
    page.on("pageerror", lambda e: errors.append(str(e)))
    page.on("dialog", lambda d: d.accept())

    page.goto(BASE + "/admin")
    page.fill("#tok", ADMIN)
    page.click("button:has-text('Войти')")
    page.click("button:has-text('Новый сценарий')")
    expect(page).to_have_url(BASE + "/admin/scenarios/new")
    ed = page.locator(".editor")

    # --- пустой сценарий: сервер показывает ошибки у полей и в списке ---
    expect(ed.locator(".status")).to_contain_text("ошибок:", timeout=5000)
    expect(ed.locator(".issues")).to_contain_text("не задано название")
    page.screenshot(path=f"{SHOTS}/e01-new.png")

    # клик по ошибке уровня ведёт к полю
    ed.locator(".issues button", has_text="пустое задание").click()
    expect(ed.locator("h2", has_text="Уровень 1")).to_be_visible()

    # --- настройки ---
    ed.locator(".nav", has_text="Настройки игры").click()
    ed.locator("[data-path='title'] input").fill("Тест конструктора")
    ed.locator("[data-path='host_notes'] textarea").fill("Сбор в 10:00 у фонтана")
    expect(ed.locator("[data-path='id'] input")).to_have_value("test-konstruktora")
    ed.locator("[data-path='flow.routes'] select").select_option("rotate")

    # --- уровень 1: старт с кодом ---
    ed.locator(".levels .lv").first.click()
    lvl = ed.locator(".lvl")
    lvl.locator("[data-path='levels[0].title'] input").fill("Сбор")
    lvl.locator("[data-path='levels[0].id'] input").fill("start")
    lvl.locator("[data-path='levels[0].task'] textarea").fill("Найдите год на табличке")
    lvl.locator("[data-path='levels[0].host_notes'] textarea").fill("Табличку проверить утром")
    lvl.locator("[data-path='levels[0].answers[0].values'] textarea").fill("1864")
    lvl.locator("[data-path='levels[0].pin'] select").select_option("first")
    # подсказка: по запросу со штрафом и сама через 20 минут
    lvl.locator("button", has_text="＋ подсказка").click()
    lvl.locator("[data-path='levels[0].hints[0].text'] textarea").fill("Слева от ворот")
    lvl.locator("[data-path='levels[0].hints[0].on_request'] input").check()
    expect(lvl.locator("[data-path='levels[0].hints[0].penalty.time'] input")).to_have_value("10m")
    after = lvl.locator("[data-path='levels[0].hints[0].after'] input")
    after.fill("20")
    after.blur()
    expect(after).to_have_value("20m")
    expect(lvl.locator("[data-path='levels[0].hints[0]'] .badge")).to_have_text("по запросу или сама через 20 мин")
    # вторая: 0 без «по запросу» — сервер ругается, с «по запросу» — только по запросу
    lvl.locator("button", has_text="＋ подсказка").click()
    lvl.locator("[data-path='levels[0].hints[1].text'] textarea").fill("Ищите на столбе")
    after1 = lvl.locator("[data-path='levels[0].hints[1].after'] input")
    after1.fill("0")
    after1.blur()
    expect(lvl.locator("[data-path='levels[0].hints[1].after'] .msg")).to_contain_text("никогда не откроется", timeout=5000)
    lvl.locator("[data-path='levels[0].hints[1].on_request'] input").check()
    expect(lvl.locator("[data-path='levels[0].hints[1]'] .badge")).to_have_text("только по запросу")
    expect(lvl.locator("[data-path='levels[0].hints[1].after'] .msg")).to_have_count(0, timeout=5000)

    # сливы под подсказками: локация без адреса у уровня без геоточки — ошибка
    lvl.locator("[data-path='levels[0].location'] label.chk input").check()
    expect(lvl.locator("[data-path='levels[0].location'] .msg")).to_contain_text("слив локации пустой", timeout=5000)
    lvl.locator("[data-path='levels[0].location.text'] textarea").fill("Главный вход в парк")
    expect(lvl.locator("[data-path='levels[0].location'] .msg")).to_have_count(0, timeout=5000)
    expect(lvl.locator("[data-path='levels[0].location.penalty.time'] input")).to_have_value("15m")
    lvl.locator("[data-path='levels[0].skip'] label.chk input").check()
    expect(lvl.locator("[data-path='levels[0].skip.penalty.time'] input")).to_have_value("30m")
    sk_av = lvl.locator("[data-path='levels[0].skip.available_after'] input")
    sk_av.fill("45")
    sk_av.blur()
    expect(sk_av).to_have_value("45m")
    page.screenshot(path=f"{SHOTS}/e02a-sliv.png", full_page=True)

    # защита от перебора у уровня: по умолчанию блокировка, переключаем на штраф
    rl = lvl.locator("[data-path='levels[0].rate_limit']")
    rl.locator("label.chk input").check()
    expect(rl.locator("[data-path='levels[0].rate_limit.wrong_answers'] input")).to_have_value("5")
    expect(rl.locator("[data-path='levels[0].rate_limit.block_for'] input")).to_have_value("5m")
    expect(rl.locator(".sum")).to_contain_text("ввод кодов на уровне закрывается на 5 мин")
    rl.locator("[data-path='levels[0].rate_limit.action'] select").select_option("penalty")
    expect(rl.locator("[data-path='levels[0].rate_limit.penalty.time'] input")).to_have_value("5m")
    expect(rl.locator(".sum")).to_contain_text("+5 мин к времени")

    # --- уровень 2: геоточка + бонус-фото ---
    ed.locator("button", has_text="＋ уровень").click()
    lvl.locator("[data-path='levels[1].title'] input").fill("Мост")
    lvl.locator("[data-path='levels[1].task'] textarea").fill("Встаньте в центре моста")
    lvl.locator("[data-path='levels[1].answers[0].type'] select").select_option("geo")
    lvl.locator("[data-path='levels[1].answers[0].point'] input").fill("44.6338, 39.1352")
    lvl.locator("button", has_text="＋ бонус").click()
    lvl.locator("[data-path='levels[1].bonuses[0].title'] input").fill("Фото на мосту")
    # карта открывается и закрывается
    lvl.locator("button", has_text="На карте").click()
    expect(page.locator(".leaflet-container")).to_be_visible()
    page.screenshot(path=f"{SHOTS}/e02-map.png")
    page.locator(".modal button", has_text="Отмена").click()
    # слив локации без адреса: точка возьмётся из геоответа, штраф можно убрать
    lvl.locator("[data-path='levels[1].location'] label.chk input").check()
    expect(lvl.locator("[data-path='levels[1].location.point']")).to_contain_text("покажем точку ответа уровня")
    lvl.locator("[data-path='levels[1].location.penalty.time'] input").fill("")

    # --- уровни 3 и 4 ---
    for title in ["Парк", "Финиш"]:
        ed.locator("button", has_text="＋ уровень").click()
        lvl.locator("[data-path$='].title'] input").first.fill(title)
        lvl.locator("[data-path$='].task'] textarea").first.fill("Задание: " + title)
        lvl.locator("[data-path$='].values'] textarea").first.fill("ok")
    lvl.locator("[data-path='levels[3].pin'] select").select_option("last")

    # перетаскивание: «Парк» (3) перед «Мост» (2)
    items = ed.locator(".levels li")
    items.nth(2).drag_to(items.nth(1))
    expect(ed.locator(".levels .t").nth(1)).to_have_text("Парк")

    expect(ed.locator(".status")).to_have_text("✓ ошибок нет", timeout=5000)
    page.screenshot(path=f"{SHOTS}/e03-level.png", full_page=True)

    # --- расписание подсказок и сливов ---
    ed.locator(".nav", has_text="Подсказки и сливы").click()
    sched = ed.locator(".sched")
    first = sched.locator("tbody tr").first
    expect(first.locator("[data-path='levels[0].hints[0]'] input")).to_have_value("20m")
    expect(first.locator("[data-path='levels[0].hints[0]'] .tag")).to_have_text("запрос +10 мин или сама")
    expect(first.locator("[data-path='levels[0].hints[1]'] .tag")).to_have_text("только запрос +10 мин")
    expect(first).to_contain_text("защита от перебора")
    sk = first.locator("[data-path='levels[0].skip'] input")
    sk.fill("50")
    sk.blur()
    expect(sched.locator(".tl")).to_have_count(0)
    page.screenshot(path=f"{SHOTS}/e03a-schedule.png", full_page=True)
    sched.locator("tbody tr").first.locator("button.link").click()
    expect(lvl.locator("[data-path='levels[0].skip.available_after'] input")).to_have_value("50m")

    # --- маршруты: ротация, финиш у всех последний ---
    ed.locator(".nav", has_text="Маршруты").click()
    rows = ed.locator("table.grid tr")
    expect(rows).to_have_count(5, timeout=5000)
    expect(rows.nth(0)).to_contain_text("Сбор→Парк→Мост→Финиш")
    expect(rows.nth(4)).to_contain_text("Сбор→Мост→Парк→Финиш")
    page.screenshot(path=f"{SHOTS}/e04-routes.png")

    # --- сохранение версии 1 ---
    ed.locator(".bar button", has_text="Сохранить").click()
    expect(ed.locator(".note.ok")).to_contain_text("версия 1")
    expect(ed.locator(".bar button", has_text="Сохранено")).to_be_disabled()
    assert "/admin/scenarios/" in page.url and not page.url.endswith("/new")

    # --- ошибка в режиме «по времени»: баллы запрещены ---
    ed.locator(".levels .lv").first.click()
    ed.locator(".nav", has_text="Настройки игры").click()
    ed.locator("[data-path='scoring.mode'] select").select_option("points")
    ed.locator(".levels .lv").first.click()
    lvl.locator("[data-path='levels[0].points'] input").fill("10")
    ed.locator(".nav", has_text="Настройки игры").click()
    ed.locator("[data-path='scoring.mode'] select").select_option("time")
    expect(ed.locator(".issues")).to_contain_text("баллов нет", timeout=5000)
    expect(ed.locator(".bar button", has_text="Сохранить")).to_be_disabled()
    ed.locator(".issues button", has_text="баллов нет").click()
    lvl.locator("button", has_text="Убрать баллы").click()
    expect(ed.locator(".status")).to_have_text("✓ ошибок нет", timeout=5000)

    # --- правка → черновик переживает перезагрузку → версия 2 ---
    ed.locator(".levels .lv").nth(1).click()
    lvl.locator("[data-path='levels[1].task'] textarea").fill("Найдите самый старый дуб")
    expect(ed.locator(".bar button", has_text="Сохранить версию 2")).to_be_enabled(timeout=5000)
    page.wait_for_timeout(800)  # автосохранение черновика
    page.reload()
    expect(ed.locator(".restore")).to_be_visible()
    ed.locator(".restore button", has_text="Восстановить").click()
    ed.locator(".levels .lv").nth(1).click()
    expect(lvl.locator("[data-path='levels[1].task'] textarea")).to_have_value("Найдите самый старый дуб")
    ed.locator(".bar button", has_text="Сохранить версию 2").click()
    expect(ed.locator(".note.ok")).to_contain_text("версия 2")

    # --- YAML ---
    ed.locator(".nav", has_text="YAML").click()
    src = ed.locator(".yaml textarea").input_value()
    for want in ["id: test-konstruktora", "host_notes: Сбор в 10:00 у фонтана", "    host_notes: Табличку проверить утром", "routes: rotate", "pin: first", "point: [44.6338, 39.1352]",
                 "on_request: true", "after: 20m", "after: 0", "penalty: {time: 10m}",
                 "    location:\n      text: Главный вход в парк\n      penalty: {time: 15m}",
                 "    skip:\n      penalty: {time: 30m}\n      available_after: 50m",
                 "    rate_limit:\n      wrong_answers: 5\n      per: 1m\n      action: penalty\n      penalty: {time: 5m}", "    location: {}"]:
        assert want in src, f"нет в YAML: {want}\n{src}"
    page.screenshot(path=f"{SHOTS}/e05-yaml.png")

    # --- тестовая игра ---
    with ctx.expect_page() as newp:
        ed.locator("button", has_text="Тестовая игра").click()
    run = newp.value
    expect(run.locator("h1")).to_contain_text("Тест конструктора — тест")
    notes = run.locator("details.notes")
    expect(notes).to_contain_text("Сбор в 10:00 у фонтана")
    expect(notes).to_contain_text("Сбор: Табличку проверить утром")

    # --- в списке сценариев ---
    page.goto(BASE + "/admin")
    row = page.locator("tr", has_text="test-konstruktora")
    expect(row).to_contain_text("2")
    page.screenshot(path=f"{SHOTS}/e06-list.png")

    browser.close()

if errors:
    print("ОШИБКИ JS:", *errors, sep="\n  ")
    sys.exit(1)
print("ok: конструктор")
