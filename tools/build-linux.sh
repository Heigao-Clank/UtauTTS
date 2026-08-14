#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
output_dir="${1:-${root_dir}/release/UtauTTS-linux}"

for command_name in go cmake unzip; do
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
cp "${root_dir}/LICENSE" "${output_dir}/LICENSE"
cp "${root_dir}/THIRD_PARTY_NOTICES.txt" "${output_dir}/THIRD_PARTY_NOTICES.txt"
cp "${root_dir}/README.md" "${output_dir}/README.md"

license_dir="${output_dir}/licenses/Go"
mkdir -p "${license_dir}"
cp "$(go env GOROOT)/LICENSE" "${license_dir}/GO-LICENSE.txt"
for module in golang.org/x/text github.com/ikawaha/kagome/v2 github.com/ikawaha/kagome-dict/ipa; do
  module_info="$(go list -m -f '{{.Dir}}|{{.Version}}' "${module}")"
  module_dir="${module_info%%|*}"
  module_version="${module_info#*|}"
  module_name="${module//\//_}"
  module_name="${module_name//./_}"
  cp "${module_dir}/LICENSE" "${license_dir}/${module_name}-${module_version}-LICENSE.txt"
  if [[ -f "${module_dir}/NOTICE.txt" ]]; then
    cp "${module_dir}/NOTICE.txt" "${license_dir}/${module_name}-${module_version}-NOTICE.txt"
  fi
done
mkdir -p "${output_dir}/models"
cp -R "${root_dir}/models/." "${output_dir}/models/"
mkdir -p "${output_dir}/plugins"
mkdir -p "${output_dir}/plugins/renderers"
cp -R "${root_dir}/plugins/renderers/." "${output_dir}/plugins/renderers/"
mkdir -p "${output_dir}/voice"
if compgen -G "${root_dir}/voice/*.zip" > /dev/null; then
  for archive in "${root_dir}"/voice/*.zip; do
    unzip -q -o "${archive}" -d "${output_dir}/voice"
  done
fi
echo "Built Linux package at ${output_dir}"
