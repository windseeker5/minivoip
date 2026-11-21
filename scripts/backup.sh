#!/bin/sh
# Nightly copy of the real minivoip's data (the container's data/ folder): the
# database, safely copied while it runs, plus recordings, logos, voicemails and the
# Firebase files. One file a day in ~/minivoip-backups, kept 14 days.
# Run by cron at 3:30 (crontab -l). To restore: stop the container, unpack the
# day's file over data/, put its minivoip.db in data/, start the container.
set -eu
cd "$(dirname "$0")/.."
dest="${MINIVOIP_BACKUP_DIR:-$HOME/minivoip-backups}"
mkdir -p -m 700 "$dest"
day=$(date +%Y-%m-%d)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

sqlite3 data/minivoip.db ".backup '$tmp/minivoip.db'"
tar -czf "$dest/minivoip-$day.tar.gz.part" \
  -C "$tmp" minivoip.db \
  -C "$PWD" --exclude='data/*.db' --exclude='data/*.db-*' --exclude='data/notes' data
mv "$dest/minivoip-$day.tar.gz.part" "$dest/minivoip-$day.tar.gz"
find "$dest" -name 'minivoip-*.tar.gz' -mtime +14 -delete
echo "$(date '+%F %T') backup: $dest/minivoip-$day.tar.gz ($(du -h "$dest/minivoip-$day.tar.gz" | cut -f1))"
