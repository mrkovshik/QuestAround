"""Прогон админки ведущего (ноутбук) вместе с игроком (телефон) в headless Chrome:
вход, загрузка сценария с ошибками и без, создание игры, регистрация, старт,
дашборд, модерация фото с клавиатуры, ввод кода за команду, штраф, таблица,
журнал, сообщение всем, завершение.

Запуск (на свежем сервере в памяти — скрипт рассчитывает на нумерацию версий с 1):
    QUESTAROUND_ADMIN_TOKEN=e2e-admin-token-000001 questaround serve -addr 127.0.0.1:8091 &
    python3 web/e2e/host_flow.py
"""
import os
import struct
import sys
import zlib

from playwright.sync_api import expect, sync_playwright

BASE = os.environ.get("QA_BASE", "http://127.0.0.1:8091")
ADMIN = os.environ.get("QA_ADMIN_TOKEN", "e2e-admin-token-000001")
CHROME = os.environ.get("QA_CHROME", "/usr/bin/google-chrome")
SHOTS = os.environ.get("QA_SHOTS", "/tmp/questaround-shots")
HERE = os.path.dirname(os.path.abspath(__file__))
os.makedirs(SHOTS, exist_ok=True)


def png(path):
    w, h = 320, 240
    rows = []
    for y in range(h):
        rows.append(b"\x00" + b"".join(bytes([(x * 3) % 256, (y * 2) % 256, 140]) for x in range(w)))
    def chunk(t, d):
        return struct.pack(">I", len(d)) + t + d + struct.pack(">I", zlib.crc32(t + d) & 0xFFFFFFFF)
    open(path, "wb").write(b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 2, 0, 0, 0))
                           + chunk(b"IDAT", zlib.compress(b"".join(rows))) + chunk(b"IEND", b""))


PHOTO = os.path.join(SHOTS, "host-photo.png")
png(PHOTO)
errors = []


def note(page, text):
    expect(page.locator(".note", has_text=text).first).to_be_visible(timeout=5000)


with sync_playwright() as p:
    browser = p.chromium.launch(executable_path=CHROME, args=["--no-sandbox"])
    laptop = browser.new_context(viewport={"width": 1440, "height": 900}, locale="ru-RU")
    host = laptop.new_page()
    host.on("pageerror", lambda e: errors.append(f"host: {e}"))
    host.on("dialog", lambda d: d.accept())

    # --- вход ---
    host.goto(BASE + "/admin")
    host.fill("#tok", "wrong")
    host.click("button:has-text('Войти')")
    expect(host.locator(".notice")).to_have_text("Неверный токен")
    host.fill("#tok", ADMIN)
    host.click("button:has-text('Войти')")
    expect(host.locator("h2", has_text="Игры")).to_be_visible()

    # --- сценарий с ошибками ---
    host.fill("textarea", "version: 1\nid: Плохой\ntitle: x\nlevels:\n  - id: a\n    task: t\n    answers: [{type: regex, pattern: '(['}]\n")
    host.click("text=Проверить и сохранить")
    expect(host.locator(".issues")).to_contain_text("строка 7")
    host.screenshot(path=f"{SHOTS}/h01-scenario-errors.png")

    # --- нормальный сценарий и игра ---
    host.fill("textarea", open(os.path.join(HERE, "demo.yaml"), encoding="utf-8").read())
    host.click("text=Проверить и сохранить")
    expect(host.locator(".notice", has_text="Загружено")).to_be_visible()
    host.fill("#title", "Горячий Ключ: пилот, 29 ноября")
    host.click("button:has-text('Создать игру')")
    expect(host.locator("h1")).to_have_text("Горячий Ключ: пилот, 29 ноября")
    code = host.locator("strong.code").inner_text()

    # --- регистрация: команда от ведущего ---
    host.click(".tabs >> text=Регистрация")
    host.fill("input[placeholder='Название команды']", "Корпоратив")
    host.click("button:has-text('Добавить')")
    expect(host.locator(".created")).to_contain_text("/join/")

    # --- игрок регистрирует свою команду с телефона ---
    phone = browser.new_context(viewport={"width": 390, "height": 844}, is_mobile=True, has_touch=True, locale="ru-RU")
    pl = phone.new_page()
    pl.on("pageerror", lambda e: errors.append(f"player: {e}"))
    pl.goto(f"{BASE}/g/{code}")
    pl.click("text=Создать команду")
    pl.fill("#team", "Ёжики")
    pl.fill("#name", "Коля")
    pl.fill("#contact", "@kolya")
    pl.click("button:has-text('Создать')")
    expect(pl.locator("text=Ждём старта")).to_be_visible()
    expect(host.locator("td", has_text="Ёжики")).to_be_visible(timeout=5000)  # SSE
    host.screenshot(path=f"{SHOTS}/h02-registration.png", full_page=True)

    # --- ведущий правит сценарий после анонса и меняет его в игре ---
    host.click("a.brand")
    fixed = open(os.path.join(HERE, "demo.yaml"), encoding="utf-8").read().replace("title: Сбор у парка", "title: Сбор у главного входа")
    host.fill("textarea", fixed)
    host.click("text=Проверить и сохранить")
    expect(host.locator(".notice", has_text="версия 2")).to_be_visible()
    host.click("td:has-text('Горячий Ключ: пилот')")
    expect(host.locator(".scn")).to_contain_text("версия 1")
    host.click(".scn button:has-text('Заменить')")
    note(host, "Сценарий заменён")
    expect(host.locator(".scn")).to_contain_text("версия 2")
    host.click(".tabs >> text=Регистрация")
    expect(host.locator("td", has_text="Ёжики")).to_be_visible()

    # --- старт ---
    host.click("button:has-text('Старт игры')")
    note(host, "Игра запущена")
    host.click(".tabs >> text=Команды")
    expect(host.locator("tbody tr")).to_have_count(2)
    expect(pl.locator("h2", has_text="Сбор у главного входа")).to_be_visible(timeout=5000)

    # --- игрок: неверный код, фото-бонус ---
    pl.fill(".codebar input", "0000")
    pl.click(".codebar button")
    with pl.expect_file_chooser() as fc:
        pl.click("text=Отправить фото")
    fc.value.set_files(PHOTO)
    expect(host.locator(".counter")).to_have_text("1", timeout=5000)
    host.screenshot(path=f"{SHOTS}/h03-dashboard.png", full_page=True)

    # --- модерация с клавиатуры ---
    host.click(".tabs >> text=Модерация")
    expect(host.locator(".item img")).to_be_visible(timeout=5000)
    host.screenshot(path=f"{SHOTS}/h04-moderation.png")
    host.keyboard.press("a")
    note(host, "Принято")
    expect(host.locator("text=Очередь пуста")).to_be_visible()
    expect(pl.locator(".badge", has_text="+5").first).to_be_visible(timeout=5000)

    # --- правка сценария во время игры: структуру нельзя, текст можно ---
    host.click("a.brand")
    v3 = fixed.replace("Найдите табличку с годом основания", "Найдите бронзовую табличку с годом основания")
    v4 = v3.replace("  - id: photo\n", "  - id: extra\n    task: лишний\n    answers: [{type: photo}]\n  - id: photo\n")
    for src, ver in ((v3, "версия 3"), (v4, "версия 4")):
        host.fill("textarea", src)
        host.click("text=Проверить и сохранить")
        expect(host.locator(".notice", has_text=ver)).to_be_visible()
    host.click("td:has-text('Горячий Ключ: пилот')")
    host.click(".scn button:has-text('Заменить')")  # по умолчанию — самая новая, v4
    expect(host.locator(".conflict")).to_contain_text("число уровней")
    host.locator(".scn select").select_option(label=host.locator(".scn option", has_text="версия 3").inner_text())
    host.click(".scn button:has-text('Заменить')")
    note(host, "Сценарий заменён")
    expect(pl.locator(".task", has_text="бронзовую табличку")).to_be_visible(timeout=8000)

    # --- карточка команды «Корпоратив»: код за команду, штраф ---
    host.click(".tabs >> text=Команды")
    host.click("tbody tr:has-text('Корпоратив')")
    expect(host.locator(".drawer h2")).to_have_text("Корпоратив")
    host.fill(".drawer input[placeholder='Код']", "1864")
    host.click(".drawer button:has-text('Отправить')")
    note(host, "Верно")
    host.fill("#reason", "сидели без связи")
    host.fill(".drawer input[placeholder='+5m / -2m']", "-2m")
    host.click(".drawer button:has-text('Изменить время')")
    note(host, "Время изменено")
    # причина не сбрасывается от обновлений по SSE
    expect(host.locator("#reason")).to_have_value("сидели без связи")
    host.screenshot(path=f"{SHOTS}/h05-team-drawer.png")
    host.keyboard.press("Escape")

    # --- таблица: открыть игрокам ---
    host.click(".tabs >> text=Таблица")
    host.click(".seg >> text=Показать")
    note(host, "открыта игрокам")
    expect(pl.locator("nav >> text=Таблица")).to_be_visible(timeout=5000)

    # --- сообщение всем ---
    host.fill("input[placeholder^='Сообщение всем']", "Через 20 минут сбор у фонтана")
    host.click("text=Отправить всем")
    expect(pl.locator(".msg")).to_contain_text("сбор у фонтана", timeout=5000)

    # --- журнал ---
    host.click(".tabs >> text=Журнал")
    expect(host.locator("td", has_text="«0000» → неверно")).to_be_visible(timeout=5000)
    expect(host.locator("td", has_text="сидели без связи")).to_be_visible()
    host.screenshot(path=f"{SHOTS}/h06-journal.png")

    # --- завершение ---
    host.click("button:has-text('Завершить игру')")
    note(host, "Игра завершена")
    expect(pl.locator("h2", has_text="Финиш!")).to_be_visible(timeout=5000)
    expect(pl.locator("text=Ведущий подводит итоги")).to_be_visible()
    # расшифровка баллов у игрока: фото-бонус +5
    expect(pl.locator(".ledger li", has_text="Бонус «Селфи всей командой у ворот»")).to_contain_text("+5")

    # --- утверждение итогов ---
    expect(host.locator(".finbanner")).to_be_visible()
    host.click(".finbanner >> text=К итогам")
    host.fill("#cmsg", "Награждение в 18:00 у фонтана!")
    host.screenshot(path=f"{SHOTS}/h07-approve.png", full_page=True)
    host.click("button:has-text('Утвердить итоги')")
    note(host, "Итоги утверждены")
    expect(host.locator("h2", has_text="Итоги (утверждены)")).to_be_visible()
    expect(pl.locator(".congrats")).to_contain_text("место", timeout=8000)
    expect(pl.locator(".congrats")).to_contain_text("Награждение в 18:00")
    pl.screenshot(path=f"{SHOTS}/h08-congrats.png", full_page=True)
    pl.click("text=Итоговая таблица")
    expect(pl.locator("h2", has_text="Итоги")).to_be_visible()
    host.screenshot(path=f"{SHOTS}/h09-final-table.png")

    # --- список игр ---
    host.click("a.brand")
    expect(host.locator("td", has_text="Горячий Ключ: пилот")).to_be_visible()
    host.screenshot(path=f"{SHOTS}/h10-runs.png")
    browser.close()

if errors:
    print("ОШИБКИ:", *errors, sep="\n")
    sys.exit(1)
print("HOST UI OK")
