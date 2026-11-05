#!/bin/sh
# Обновление до последней версии из GitHub: забрать код, пересобрать, перезапустить.
# Игры не прерываются надолго: состояние в базе, клиенты переподключатся сами.
set -eu
cd "$(dirname "$0")"
git pull --ff-only
docker compose up -d --build
docker image prune -f >/dev/null
docker compose ps
