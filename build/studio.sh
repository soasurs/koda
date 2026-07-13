#!/bin/sh

set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
source_dir=$root/studio
output=$root/internal/studio/dist
workdir=$(mktemp -d "${TMPDIR:-/tmp}/koda-studio.XXXXXX")

cleanup() {
	rm -rf "$workdir"
}
trap cleanup EXIT HUP INT TERM

echo "building Koda Studio"
pnpm --dir "$source_dir" install --frozen-lockfile
pnpm --dir "$source_dir" build

staged=$workdir/dist
cp -R "$source_dir/dist" "$staged"
rm -rf "$output"
mkdir -p "$(dirname "$output")"
mv "$staged" "$output"

echo "embedded assets updated in $output"
