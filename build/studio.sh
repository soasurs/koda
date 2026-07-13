#!/bin/sh

set -eu

repository=${KODA_STUDIO_REPOSITORY:-https://github.com/soasurs/koda-studio.git}
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
output=$root/internal/studio/dist
version_file=$root/build/studio/version.txt
workdir=$(mktemp -d "${TMPDIR:-/tmp}/koda-studio.XXXXXX")

cleanup() {
	rm -rf "$workdir"
}
trap cleanup EXIT HUP INT TERM

version=$(tr -d '[:space:]' <"$version_file")
if [ -z "$version" ]; then
	echo "$version_file must contain a Koda Studio release tag" >&2
	exit 1
fi

echo "building Koda Studio $version"
git clone --branch "$version" --depth 1 "$repository" "$workdir/source"
(
	cd "$workdir/source"
	pnpm install --frozen-lockfile
	pnpm build
)

staged=$workdir/dist
cp -R "$workdir/source/dist" "$staged"
rm -rf "$output"
mkdir -p "$(dirname "$output")"
mv "$staged" "$output"

echo "embedded assets updated in $output"
