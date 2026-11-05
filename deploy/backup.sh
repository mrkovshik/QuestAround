#!/bin/sh
# Ручной бэкап «прямо сейчас»: база + фото в deploy/backups.
# Скачать к себе:  scp root@IP:/opt/questline/deploy/backups/* .
set -eu
cd "$(dirname "$0")"
mkdir -p backups
ts=$(date +%Y%m%d-%H%M)
docker compose exec -T postgres pg_dump -U questline questline | gzip > "backups/questline-$ts.sql.gz"
docker compose run --rm --no-deps -T --entrypoint tar app czf - -C /data media > "backups/media-$ts.tar.gz"
ls -lh backups | tail -5
