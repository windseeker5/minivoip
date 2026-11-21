#!/bin/sh
# Gives `task dev` a fresh copy of the real minivoip's data (data/ -> data-dev/), so
# trying things never touches real accounts, messages or notes. Replaces data-dev/.
# Stop `task dev` first; the container can keep running.
set -eu
cd "$(dirname "$0")/.."
rm -rf data-dev
mkdir -p -m 700 data-dev
sqlite3 data/minivoip.db ".backup 'data-dev/minivoip.db'"
for d in logos prompts voicemail notes; do
  [ -d "data/$d" ] && cp -a "data/$d" data-dev/
done
echo "data-dev/ is a fresh copy of data/"
