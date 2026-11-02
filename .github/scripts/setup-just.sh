#!/usr/bin/env bash
set -euo pipefail

version=1.54.0
case "$(uname -s)/$(uname -m)" in
    Linux/x86_64) target=x86_64-unknown-linux-musl; checksum=193e2b04bac8ff3568ff81cf62f8c9db08f8083e16665e28d400dd35bac47ffd ;;
    Linux/aarch64) target=aarch64-unknown-linux-musl; checksum=2162accb54d087c6fbd165c1baedccb0f93b4953af7f00309b8f928016c49cae ;;
    Darwin/x86_64) target=x86_64-apple-darwin; checksum=3f1adefef12bcdf99134a44bde53b779e83726381e15de92113ce4bf2a3cf61f ;;
    Darwin/arm64) target=aarch64-apple-darwin; checksum=a776fc5015491f84580875bc6dad3de59fa4f47a309ccac49d1287cb55f9b599 ;;
    MINGW*/x86_64) target=x86_64-pc-windows-msvc; checksum=860e21474e956eaa9879d62f68cc24530254eefe14ac108eaf707c0daf56a6d0 ;;
    MINGW*/aarch64) target=aarch64-pc-windows-msvc; checksum=53037b100843ba20826506aa8857c3b209d01542643570190a51692e3e91068f ;;
    *) echo 'Unsupported platform for the just installer.' >&2; exit 1 ;;
esac

destination="$(pwd)/output/tools/just-$version"
temporary="$(mktemp -d)"
trap 'rm -rf "$temporary"' EXIT
extension=tar.gz
[[ "$target" != *windows* ]] || extension=zip
archive="$temporary/just.$extension"
curl --fail --silent --show-error --location --retry 3 \
    "https://github.com/casey/just/releases/download/$version/just-$version-$target.$extension" -o "$archive"
if command -v sha256sum >/dev/null; then
    actual="$(sha256sum "$archive" | cut -d ' ' -f 1)"
else
    actual="$(shasum -a 256 "$archive" | cut -d ' ' -f 1)"
fi
[[ "$actual" == "$checksum" ]] || { echo 'just checksum mismatch.' >&2; exit 1; }
mkdir -p "$destination"
if [[ "$extension" == zip ]]; then
    unzip -q -o "$archive" just.exe -d "$destination"
else
    tar -xzf "$archive" -C "$destination" just
fi
if [[ -n "${GITHUB_PATH:-}" ]]; then
    if command -v cygpath >/dev/null; then destination="$(cygpath -w "$destination")"; fi
    echo "$destination" >> "$GITHUB_PATH"
else
    echo "Installed just in $destination. Add that directory to PATH."
fi
