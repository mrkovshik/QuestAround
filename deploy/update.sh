#!/bin/sh
# Обновление до последней версии из GitHub: забрать код, пересобрать, перезапустить.
# Игры не прерываются надолго: состояние в базе, клиенты переподключатся сами.
#
# Запуск вручную:          /opt/questaround/deploy/update.sh [force]
# Запуск из GitHub Actions: по SSH-ключу с command="…/update.sh" в authorized_keys;
#   аргументы приходят в SSH_ORIGINAL_COMMAND: "deploy <sha> [force]".
#
# Если идёт игра, обновление откладывается (код выхода 75) — чтобы не дёргать
# игроков посреди уровня. «force» — обновить всё равно.
set -eu
cd "$(dirname "$0")"

SHA=""
FORCE=""
# shellcheck disable=SC2086
for arg in ${SSH_ORIGINAL_COMMAND:-} "$@"; do
  case "$arg" in
    deploy) ;;
    force) FORCE=1 ;;
    *)
      if printf '%s' "$arg" | grep -Eq '^[0-9a-f]{40}$'; then
        SHA="$arg"
      else
        echo "неизвестный аргумент: $arg" >&2
        exit 2
      fi
      ;;
  esac
done

# Не единственный ли это запуск (два пуша подряд).
exec 9>/tmp/questaround-update.lock
if ! flock -n 9; then
  echo "Обновление уже идёт — ждём его." >&2
  flock 9
fi

if [ -z "$FORCE" ]; then
  # Спрашиваем у работающего приложения список игр (токен ведущего — в окружении контейнера).
  runs=$(docker compose exec -T app sh -c \
    'wget -qO- --header "Authorization: Bearer $QUESTAROUND_ADMIN_TOKEN" http://127.0.0.1:8080/admin/v1/runs' \
    2>/dev/null || true)
  if printf '%s' "$runs" | grep -q '"status":"running"'; then
    echo "Сейчас идёт игра — обновление отложено, чтобы не перезапускать сервер посреди уровня." >&2
    echo "Обновить всё равно: $0 force  (или Actions → Deploy → Run workflow с force)." >&2
    exit 75
  fi
fi

git fetch --quiet origin main
if [ -n "$SHA" ]; then
  git merge --ff-only --quiet "$SHA"
else
  git merge --ff-only --quiet origin/main
fi
echo "Версия: $(git log -1 --format='%h %s')"

docker compose up -d --build
docker image prune -f >/dev/null

# Ждём, пока приложение ответит.
for _ in $(seq 1 30); do
  if docker compose exec -T app wget -qO- http://127.0.0.1:8080/healthz >/dev/null 2>&1; then
    docker compose ps
    echo "Готово."
    exit 0
  fi
  sleep 2
done
docker compose ps
docker compose logs --tail=50 app
echo "Приложение не ответило за минуту — смотрите логи выше." >&2
exit 1
