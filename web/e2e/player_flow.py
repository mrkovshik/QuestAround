"""Прогон PWA игрока в headless Chrome: регистрация, лобби, старт, ответы, фото,
офлайн-очередь, таблица.

Запуск (нужны python-пакет playwright и Chrome/Chromium):
    QUESTLINE_ADMIN_TOKEN=e2e-admin-token-000001 questline serve -addr 127.0.0.1:8091 &
    python3 web/e2e/player_flow.py

Переменные: QL_BASE (по умолчанию http://127.0.0.1:8091), QL_ADMIN_TOKEN, QL_CHROME.
Скриншоты — в QL_SHOTS (по умолчанию /tmp/questline-shots).
"""
import json
import struct
import sys
import time
import urllib.request
import zlib

from playwright.sync_api import sync_playwright, expect

import os

BASE = os.environ.get("QL_BASE", "http://127.0.0.1:8091")
ADMIN = os.environ.get("QL_ADMIN_TOKEN", "e2e-admin-token-000001")
CHROME = os.environ.get("QL_CHROME", "/usr/bin/google-chrome")
SHOTS = os.environ.get("QL_SHOTS", "/tmp/questline-shots")
HERE = os.path.dirname(os.path.abspath(__file__))


def _post(path, data, ctype):
    req = urllib.request.Request(BASE + path, data=data, method="POST",
                                 headers={"Authorization": "Bearer " + ADMIN, "Content-Type": ctype})
    with urllib.request.urlopen(req) as r:
        return json.loads(r.read())


_sv = _post("/admin/v1/scenarios", open(os.path.join(HERE, "demo.yaml"), "rb").read(), "application/yaml")
RUN = _post("/admin/v1/runs", json.dumps({"scenario_version_id": _sv["id"], "title": "Горячий Ключ: тест"}).encode(),
            "application/json")
CODE = RUN["public_code"]


def admin(method, path, body=None):
    req = urllib.request.Request(BASE + "/admin/v1/runs/" + RUN["id"] + path, method=method,
                                 data=json.dumps(body).encode() if body is not None else None,
                                 headers={"Authorization": "Bearer " + ADMIN})
    with urllib.request.urlopen(req) as r:
        return json.loads(r.read() or b"null")


def png(path):
    w, h = 64, 48
    raw = b"".join(b"\x00" + bytes([40, 120, 90] * w) for _ in range(h))
    def chunk(t, d):
        return struct.pack(">I", len(d)) + t + d + struct.pack(">I", zlib.crc32(t + d) & 0xFFFFFFFF)
    data = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 2, 0, 0, 0)) + \
        chunk(b"IDAT", zlib.compress(raw)) + chunk(b"IEND", b"")
    open(path, "wb").write(data)


def toast(page, text):
    expect(page.locator(".toast", has_text=text).first).to_be_visible(timeout=5000)


os.makedirs(SHOTS, exist_ok=True)
PHOTO = os.path.join(SHOTS, "photo.png")
png(PHOTO)
errors = []

try:
  pw_cm = sync_playwright()
  p = pw_cm.__enter__()
  if True:
      browser = p.chromium.launch(executable_path=CHROME, args=["--no-sandbox"])
      phone = dict(viewport={"width": 390, "height": 844}, device_scale_factor=2, is_mobile=True, has_touch=True,
                   locale="ru-RU", geolocation={"latitude": 44.6338, "longitude": 39.1352, "accuracy": 10},
                   permissions=["geolocation"])

      # --- капитан ---
      cap_ctx = browser.new_context(**phone)
      cap = cap_ctx.new_page()
      cap.on("pageerror", lambda e: errors.append(f"captain: {e}"))
      cap.on("console", lambda m: m.type == "error" and errors.append(f"captain console: {m.text}"))
      cap.goto(BASE + "/")
      cap.screenshot(path=f"{SHOTS}/01-home.png")
      cap.fill("#code", CODE.lower())
      cap.click("text=Продолжить")
      expect(cap.locator("h1")).to_have_text("Горячий Ключ: тест")
      cap.screenshot(path=f"{SHOTS}/02-game-info.png")
      cap.click("text=Создать команду")
      cap.fill("#team", "Ёжики")
      cap.fill("#name", "Коля")
      cap.fill("#contact", "@kolya")
      cap.screenshot(path=f"{SHOTS}/03-create.png")
      cap.click("button:has-text('Создать')")
      expect(cap.locator("text=Ждём старта")).to_be_visible()
      cap.click("text=QR-код")
      expect(cap.locator(".qr svg")).to_be_visible()
      cap.screenshot(path=f"{SHOTS}/04-lobby.png", full_page=True)
      invite = cap.locator(".code").inner_text()

      # --- игрок по ссылке-приглашению ---
      pl_ctx = browser.new_context(**phone)
      pl = pl_ctx.new_page()
      pl.on("pageerror", lambda e: errors.append(f"player: {e}"))
      pl.goto(f"{BASE}/g/{CODE}/join/{invite}")
      expect(pl.locator("#inv")).to_have_value(invite)
      pl.fill("#name", "Маша")
      pl.click("button:has-text('Вступить')")
      expect(pl.locator("text=Ждём старта")).to_be_visible()
      expect(pl).to_have_url(f"{BASE}/g/{CODE}")
      # капитан видит нового игрока без перезагрузки (SSE)
      expect(cap.locator(".players li", has_text="Маша")).to_be_visible(timeout=5000)

      # --- старт ---
      admin("POST", "/start")
      expect(cap.locator("h2", has_text="Сбор у парка")).to_be_visible(timeout=5000)
      expect(pl.locator("h2", has_text="Сбор у парка")).to_be_visible(timeout=5000)
      cap.screenshot(path=f"{SHOTS}/05-level.png", full_page=True)

      # неверный код, потом верный
      pl.fill(".codebar input", "1111")
      pl.click(".codebar button")
      toast(pl, "Неверно")
      pl.screenshot(path=f"{SHOTS}/06-wrong.png")

      # подсказка: игрок не может, капитан берёт со штрафом
      expect(pl.locator("text=может взять капитан")).to_be_visible()
      cap.once("dialog", lambda d: d.accept())
      cap.click("text=Взять подсказку 2")
      expect(cap.locator(".hint", has_text="каменном столбе")).to_be_visible(timeout=5000)
      expect(pl.locator(".hint", has_text="каменном столбе")).to_be_visible(timeout=5000)

      # фото-бонус
      with pl.expect_file_chooser() as fc:
          pl.click("text=Отправить фото")
      fc.value.set_files(PHOTO)
      toast(pl, "на проверку")
      expect(pl.locator(".badge", has_text="на проверке")).to_be_visible(timeout=5000)

      pl.fill(".codebar input", " 1864 ")
      pl.click(".codebar button")
      toast(pl, "Верно")
      expect(pl.locator("h2", has_text="Две надписи")).to_be_visible(timeout=5000)
      expect(cap.locator("h2", has_text="Две надписи")).to_be_visible(timeout=5000)

      # модерация фото ведущим
      import urllib.request as u
      q = json.loads(u.urlopen(u.Request(BASE + "/admin/v1/runs/" + RUN["id"] + "/moderation",
                                         headers={"Authorization": "Bearer " + ADMIN})).read())
      assert len(q) == 1, q
      admin("POST", f"/moderation/{q[0]['seq']}", {"approved": True})

      # уровень с двумя секторами
      cap.fill(".codebar input", "Лев")
      cap.click(".codebar button")
      toast(cap, "Верно")
      expect(cap.locator(".dot.on")).to_have_count(1, timeout=5000)
      cap.screenshot(path=f"{SHOTS}/07-sectors.png", full_page=True)
      cap.fill(".codebar input", "1987")
      cap.click(".codebar button")

      # выбор варианта
      expect(pl.locator("h2", has_text="Знаток")).to_be_visible(timeout=5000)
      pl.screenshot(path=f"{SHOTS}/08-choice.png", full_page=True)
      pl.click("button:has-text('Петушок')")
      toast(pl, "Верно")

      # фото-задание — офлайн: ответ уходит в очередь и доезжает после восстановления сети
      expect(pl.locator("h2", has_text="Мост")).to_be_visible(timeout=5000)
      pl_ctx.set_offline(True)
      with pl.expect_file_chooser() as fc:
          pl.click("text=Сфотографировать")
      fc.value.set_files(PHOTO)
      toast(pl, "ответ сохранён")
      expect(pl.locator(".net")).to_contain_text("В очереди ответов: 1", timeout=5000)
      pl.screenshot(path=f"{SHOTS}/09-offline.png")
      pl_ctx.set_offline(False)
      pl.evaluate("dispatchEvent(new Event('online'))")
      toast(pl, "на проверку")
      q = json.loads(u.urlopen(u.Request(BASE + "/admin/v1/runs/" + RUN["id"] + "/moderation",
                                         headers={"Authorization": "Bearer " + ADMIN})).read())
      assert len(q) == 1 and q[0]["kind"] == "photo", q
      admin("POST", "/broadcast", {"text": "Все молодцы! Финиш у фонтана."})
      admin("POST", f"/moderation/{q[0]['seq']}", {"approved": True})

      expect(cap.locator("h2", has_text="Финиш!")).to_be_visible(timeout=5000)
      expect(cap.locator(".msg")).to_contain_text("Финиш у фонтана")
      cap.screenshot(path=f"{SHOTS}/10-finish.png", full_page=True)
      cap.click("nav >> text=Таблица")
      expect(cap.locator("tr.me")).to_contain_text("Ёжики")
      cap.screenshot(path=f"{SHOTS}/11-table.png", full_page=True)

      # тёмная тема
      dark = browser.new_context(**phone, color_scheme="dark")
      dp = dark.new_page()
      dp.goto(f"{BASE}/g/{CODE}")
      dp.screenshot(path=f"{SHOTS}/12-dark.png")

      browser.close()

finally:
  print("PAGE ERRORS:", *errors, sep="\n")
if errors:
    print("ОШИБКИ В КОНСОЛИ:", *errors, sep="\n")
    sys.exit(1)
print("UI OK")
