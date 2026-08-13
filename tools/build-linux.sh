#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
output_dir="${1:-${root_dir}/release/UtauTTS-linux}"

for command_name in go cmake; do
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "${command_name} is required" >&2
    exit 1
  fi
done

mkdir -p "${output_dir}" "${root_dir}/build/native" "${root_dir}/build/qt-linux"
cd "${root_dir}"
export CGO_ENABLED=1
go test ./...
go build -trimpath -o "${output_dir}/utautts-server" ./cmd/utautts-server
go build -trimpath -buildmode=c-shared -o "${root_dir}/build/native/libutautts_native.so" ./cmd/utautts-native
cmake -S "${root_dir}/qt" -B "${root_dir}/build/qt-linux" -DCMAKE_BUILD_TYPE=Release
cmake --build "${root_dir}/build/qt-linux" --config Release
cp "${root_dir}/build/qt-linux/utautts" "${output_dir}/utautts"
cp "${root_dir}/build/native/libutautts_native.so" "${output_dir}/libutautts_native.so"
echo "Built Linux package at ${output_dir}"
