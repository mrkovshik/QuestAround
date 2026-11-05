# Деплой: от нуля до работающей игры

Что получится: сервер в России (доступен и из РФ, и из Сербии) с вашим доменом и HTTPS.
На нём в Docker работают Postgres, приложение и Caddy (сам получает и продлевает сертификат),
а раз в сутки снимается бэкап базы.

Понадобится около часа, из них большая часть — ожидание (DNS, сборка).
Названия пунктов в панелях хостера и регистратора могут немного отличаться — ориентируйтесь по смыслу.

---

## 1. SSH-ключ (на своём компьютере, один раз)

Сервер лучше открывать по ключу, а не по паролю.

```sh
ssh-keygen -t ed25519 -C "questline"      # Enter на все вопросы
cat ~/.ssh/id_ed25519.pub                 # эту строку вставим в панели хостера
```

## 2. Сервер (VPS)

Подойдёт любой российский облачный хостинг — Timeweb Cloud, Selectel и т.п.
Зарубежные (Hetzner, DigitalOcean, AWS) из России работают с перебоями — не берите.
Cloudflare перед сайтом тоже не ставьте.

1. Зарегистрируйтесь у хостера, пополните баланс (карта РФ / СБП).
2. Создайте облачный сервер:
   - **ОС:** Ubuntu 24.04
   - **Регион:** Москва или Санкт-Петербург
   - **Конфигурация:** 1 vCPU, 2 ГБ RAM, 20–30 ГБ NVMe — хватит с запасом
   - **SSH-ключ:** вставьте строку из шага 1
3. Запишите **IP-адрес** сервера.

Проверка: `ssh root@IP` — должны попасть на сервер без пароля.

## 3. Домен

1. Купите домен у любого регистратора (REG.RU, RU-CENTER, у того же хостера…).
   - Для **.ru / .рф** регистратор попросит паспортные данные — так требуют правила зоны.
   - Зоны вроде **.online / .site / .com** обычно без этого.
2. В управлении DNS домена создайте **A-запись**:
   - имя: `@` (сам домен) или поддомен, например `quest`
   - значение: IP сервера
3. Подождите, пока запись разойдётся (от минут до пары часов). Проверка на своём компьютере:
   ```sh
   nslookup ваш-домен.ru      # должен показать IP сервера
   ```

## 4. Подготовка сервера

Все команды дальше — на сервере (`ssh root@IP`).

```sh
# обновления
apt update && apt -y upgrade

# файрвол: SSH и веб
ufw allow OpenSSH && ufw allow 80 && ufw allow 443 && ufw --force enable

# swap 2 ГБ — чтобы сборка не упёрлась в память
fallocate -l 2G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile
echo '/swapfile none swap sw 0 0' >> /etc/fstab

# Docker и git из репозиториев Ubuntu
apt -y install docker.io docker-compose-v2 git
systemctl enable --now docker
```

Docker Hub из России открывается не всегда. Подключите зеркала заранее:

```sh
cat > /etc/docker/daemon.json <<'EOF'
{ "registry-mirrors": ["https://dockerhub.timeweb.cloud", "https://mirror.gcr.io"] }
EOF
systemctl restart docker
docker pull alpine && echo "Docker работает"
```

## 5. Код

Репозиторий приватный — дадим серверу доступ только на чтение через deploy key.

```sh
ssh-keygen -t ed25519 -f ~/.ssh/github -N "" -C "questline-server"
cat ~/.ssh/github.pub
```

На GitHub: репозиторий → **Settings → Deploy keys → Add deploy key**, вставьте строку,
галочку «Allow write access» **не** ставьте.

```sh
cat >> ~/.ssh/config <<'EOF'
Host github.com
  IdentityFile ~/.ssh/github
EOF
git clone git@github.com:mrkovshik/QuestLine.git /opt/questline
```

## 6. Настройки и запуск

```sh
cd /opt/questline/deploy
cp .env.example .env
sed -i "s/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=$(openssl rand -hex 24)/" .env
sed -i "s/^ADMIN_TOKEN=.*/ADMIN_TOKEN=$(openssl rand -hex 24)/" .env
nano .env        # впишите DOMAIN и ACME_EMAIL
chmod 600 .env

docker compose up -d --build     # первая сборка — 3–7 минут
docker compose ps                # все сервисы должны быть running / healthy
docker compose logs -f caddy     # дождитесь строки про certificate obtained, Ctrl+C
```

Откройте `https://ваш-домен/admin` и войдите с токеном:

```sh
grep ADMIN_TOKEN .env
```

Сохраните токен в менеджер паролей — это ключ от всех игр.

## 7. Перед пилотом

- [ ] С телефона **по мобильному интернету** (не Wi-Fi) открывается `https://ваш-домен`.
- [ ] Загрузите сценарий, создайте игру, зарегистрируйте тестовую команду со второго телефона.
- [ ] Проверьте «Я на месте» (геолокация) и фото (камера) — браузер должен спросить разрешения.
- [ ] Модерация фото и ввод кода за команду в админке работают.
- [ ] Сделайте бэкап и скачайте его к себе (ниже).

## Обновление

```sh
/opt/questline/deploy/update.sh
```

Код подтянется из GitHub, образ пересоберётся, приложение перезапустится. Игры не теряются —
всё в базе; телефоны игроков переподключатся сами. Во время игры обновлять без нужды не стоит.

## Бэкапы

- Автоматически: дамп базы раз в сутки в `/opt/questline/deploy/backups`, хранятся 14 дней.
- Вручную (база + фото), например перед игрой и после неё:
  ```sh
  /opt/questline/deploy/backup.sh
  ```
- Скачать к себе (на своём компьютере):
  ```sh
  scp 'root@IP:/opt/questline/deploy/backups/*' ./questline-backups/
  ```
- Восстановить базу:
  ```sh
  /opt/questline/deploy/restore.sh backups/questline-YYYYMMDD-HHMM.sql.gz
  ```

## Если что-то не так

| Симптом | Что проверить |
|---|---|
| `docker pull` / сборка: `Forbidden`, `timeout` | зеркала в `/etc/docker/daemon.json` (шаг 4), `systemctl restart docker` |
| Сборка падает на `go mod download` | повторите `docker compose up -d --build`; модули берутся с запасного зеркала или из GitHub |
| Нет HTTPS, в логах caddy ошибки ACME | A-запись указывает на IP сервера? открыт порт 80 (`ufw status`)? |
| Сайт открывается, а `/admin` не пускает | токен — строка `ADMIN_TOKEN` из `.env` целиком |
| Приложение перезапускается | `docker compose logs app` — чаще всего опечатка в `.env` |
| Нужно посмотреть логи | `docker compose logs -f app` (или `caddy`, `postgres`, `backup`) |

## Важно про связь в регионах РФ

При ограничениях мобильного интернета работают только сервисы из «белого списка» — наш сайт
в такие моменты не откроется, где бы ни стоял сервер. На этот случай:

- ответы и фото, отправленные без сети, копятся в телефоне и уйдут сами;
- ведущий может ввести код за команду (карточка команды → «Ввести код за команду»),
  если команда продиктует его по телефону; домашний/проводной интернет ведущего при этом работает.
