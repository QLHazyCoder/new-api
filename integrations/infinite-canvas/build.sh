#!/bin/sh
set -eu
canvas_integration_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
canvas_repo_root=$(CDPATH= cd -- "$canvas_integration_dir/../.." && pwd)
canvas_build_dir=$(mktemp -d)
trap 'if [ -n "${CANVAS_KEEP_BUILD:-}" ]; then printf "Canvas checkout: %s\n" "$canvas_build_dir"; else rm -rf -- "$canvas_build_dir"; fi' EXIT
canvas_revision=$(bun -e 'const lock=await Bun.file(process.argv[1]).json();console.log(lock.commit)' "$canvas_integration_dir/source.lock")
canvas_repository=$(bun -e 'const lock=await Bun.file(process.argv[1]).json();console.log(lock.repository)' "$canvas_integration_dir/source.lock")
if [ -n "${INFINITE_CANVAS_DIR:-}" ]; then
  git -C "$INFINITE_CANVAS_DIR" archive "$canvas_revision" | tar -x -C "$canvas_build_dir"
else
  git clone --filter=blob:none --no-checkout "$canvas_repository" "$canvas_build_dir"
  git -C "$canvas_build_dir" checkout --detach "$canvas_revision"
fi
bun "$canvas_integration_dir/prepare.mjs" "$canvas_build_dir"
(
  cd "$canvas_build_dir/web"
  bun install --frozen-lockfile
  bun run typecheck
  bun run build
)
mkdir -p "$canvas_repo_root/web/dist/canvas-app"
cp -R "$canvas_build_dir/web/dist/." "$canvas_repo_root/web/dist/canvas-app/"
cp "$canvas_build_dir/LICENSE" "$canvas_repo_root/web/dist/canvas-app/LICENSE"
printf '%s\n' "$canvas_revision" > "$canvas_repo_root/web/dist/canvas-app/REVISION"
