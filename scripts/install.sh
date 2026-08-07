#!/bin/sh

set -eu

repository="fanhuadesenlinnn/sshm"
version=${SSHM_VERSION:-latest}
install_dir=${SSHM_INSTALL_DIR:-}
explicit_install_dir=false

usage() {
	cat <<'EOF'
Install sshm from GitHub Releases.

Usage:
  install.sh [--version vX.Y.Z] [--install-dir DIR]

Environment:
  SSHM_VERSION       Version to install; defaults to latest.
  SSHM_INSTALL_DIR   Installation directory; defaults to /usr/local/bin.
EOF
}

fail() {
	printf 'sshm installer: %s\n' "$*" >&2
	exit 1
}

while [ "$#" -gt 0 ]; do
	case "$1" in
	--version)
		[ "$#" -ge 2 ] || fail "--version requires a value"
		version=$2
		shift 2
		;;
	--install-dir)
		[ "$#" -ge 2 ] || fail "--install-dir requires a value"
		install_dir=$2
		explicit_install_dir=true
		shift 2
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		fail "unknown option: $1"
		;;
	esac
done

if [ "$version" != "latest" ] && ! printf '%s\n' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$'; then
	fail "version must be latest or a tag such as v1.2.3"
fi

if [ -z "$install_dir" ]; then
	install_dir=/usr/local/bin
fi

case "$(uname -s)" in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) fail "unsupported operating system: $(uname -s)" ;;
esac

case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) fail "unsupported architecture: $(uname -m)" ;;
esac

asset="sshm_${os}_${arch}.tar.gz"
if [ -n "${SSHM_RELEASE_BASE_URL:-}" ]; then
	release_base=$SSHM_RELEASE_BASE_URL
elif [ "$version" = "latest" ]; then
	release_base="https://github.com/${repository}/releases/latest/download"
else
	release_base="https://github.com/${repository}/releases/download/${version}"
fi

download() {
	url=$1
	destination=$2
	if command -v curl >/dev/null 2>&1; then
		curl --proto '=https' --tlsv1.2 -fsSL --retry 3 --retry-delay 1 -o "$destination" "$url"
	elif command -v wget >/dev/null 2>&1; then
		wget -q -O "$destination" "$url"
	else
		fail "curl or wget is required"
	fi
}

sha256_file() {
	file=$1
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$file" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$file" | awk '{print $1}'
	elif command -v openssl >/dev/null 2>&1; then
		openssl dgst -sha256 "$file" | awk '{print $NF}'
	else
		fail "sha256sum, shasum, or openssl is required to verify the download"
	fi
}

temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/sshm-install.XXXXXX")
cleanup() {
	rm -rf -- "$temp_dir"
}
trap cleanup EXIT HUP INT TERM

archive="$temp_dir/$asset"
checksums="$temp_dir/checksums.txt"

printf 'Downloading %s (%s/%s)...\n' "$version" "$os" "$arch"
download "$release_base/$asset" "$archive"
download "$release_base/checksums.txt" "$checksums"

expected=$(awk -v name="$asset" '$2 == name {print $1; exit}' "$checksums")
[ -n "$expected" ] || fail "checksums.txt does not contain $asset"
actual=$(sha256_file "$archive")
expected=$(printf '%s' "$expected" | tr '[:upper:]' '[:lower:]')
actual=$(printf '%s' "$actual" | tr '[:upper:]' '[:lower:]')
[ "$actual" = "$expected" ] || fail "SHA-256 verification failed for $asset"
printf 'Verified SHA-256: %s\n' "$actual"

tar -xzf "$archive" -C "$temp_dir"
[ -f "$temp_dir/sshm" ] || fail "$asset does not contain the sshm binary"

install_without_sudo() {
	directory=$1
	mkdir -p "$directory"
	install -m 0755 "$temp_dir/sshm" "$directory/sshm"
}

if mkdir -p "$install_dir" 2>/dev/null && [ -w "$install_dir" ]; then
	install_without_sudo "$install_dir"
elif command -v sudo >/dev/null 2>&1; then
	printf 'Installing to %s requires administrator permission.\n' "$install_dir"
	sudo mkdir -p "$install_dir"
	sudo install -m 0755 "$temp_dir/sshm" "$install_dir/sshm"
elif [ "$explicit_install_dir" = false ] && [ -n "${HOME:-}" ]; then
	install_dir="$HOME/.local/bin"
	printf 'No administrator permission available; installing to %s instead.\n' "$install_dir"
	install_without_sudo "$install_dir"
else
	fail "cannot write to $install_dir and sudo is unavailable"
fi

destination="$install_dir/sshm"
printf 'Installed: %s\n' "$destination"
"$destination" --version

case ":${PATH:-}:" in
*":$install_dir:"*) ;;
*)
	printf '\nAdd sshm to your PATH:\n'
	printf '  export PATH="%s:%s"\n' "$install_dir" "\$PATH"
	;;
esac
