#!/bin/sh
set -eu

repository="rot13maxi/pane.run"
version="${PANE_VERSION:-latest}"
install_dir="${PANE_INSTALL_DIR:-${HOME}/.local/bin}"

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) echo "pane: unsupported operating system: $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "pane: unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

artifact="pane-${os}-${arch}.tar.gz"
if [ "$version" = latest ]; then
  release_url="https://github.com/${repository}/releases/latest/download"
else
  release_url="https://github.com/${repository}/releases/download/${version}"
fi

tmp="$(mktemp -d "${TMPDIR:-/tmp}/pane-install.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

curl -fL --retry 3 --proto '=https' --tlsv1.2 \
  "${release_url}/${artifact}" -o "${tmp}/${artifact}"
curl -fL --retry 3 --proto '=https' --tlsv1.2 \
  "${release_url}/checksums.txt" -o "${tmp}/checksums.txt"

expected="$(awk -v name="$artifact" '$2 == name { print $1 }' "${tmp}/checksums.txt")"
[ -n "$expected" ] || { echo "pane: checksum missing for ${artifact}" >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "${tmp}/${artifact}" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  actual="$(shasum -a 256 "${tmp}/${artifact}" | awk '{print $1}')"
else
  echo "pane: sha256sum or shasum is required" >&2
  exit 1
fi
[ "$actual" = "$expected" ] || { echo "pane: checksum verification failed" >&2; exit 1; }

tar -xzf "${tmp}/${artifact}" -C "$tmp"
mkdir -p "$install_dir"
install -m 0755 "${tmp}/pane" "${install_dir}/pane"

echo "Pane installed at ${install_dir}/pane"
case ":${PATH}:" in
  *":${install_dir}:"*) ;;
  *) echo "Add ${install_dir} to your PATH." ;;
esac
echo "To install the Pane skill for Codex: pane skill install codex"
