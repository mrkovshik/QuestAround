#!/bin/sh
# Восстановление базы из дампа: ./restore.sh backups/questaround-YYYYMMDD-HHMM.sql.gz
# ВНИМАНИЕ: текущие данные в базе будут заменены.
set -eu
cd "$(dirname "$0")"
dump=${1:?укажите файл дампа}
printf 'Заменить базу данными из %s? Введите yes: ' "$dump"
read -r ok
[ "$ok" = "yes" ] || { echo "отменено"; exit 1; }
docker compose stop app
docker compose exec -T postgres psql -U questaround -d postgres -c 'DROP DATABASE IF EXISTS questaround' -c 'CREATE DATABASE questaround OWNER questaround'
gunzip -c "$dump" | docker compose exec -T postgres psql -q -U questaround -d questaround
docker compose start app
echo "готово"
