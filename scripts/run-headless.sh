#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd -- "$script_dir/.." && pwd)"

"$script_dir/launch-chrome.sh" --headless

if [[ ! -x "$project_dir/bin/linx" ]]; then
  make -C "$project_dir" build
fi

exec "$project_dir/bin/linx" "$@"
