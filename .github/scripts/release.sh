#!/usr/bin/env bash
# Prepare release assets, then verify their provenance before publication.
set -euo pipefail

phase="${1:-}"
version="${2:-}"
version="${version#v}"
binary="${3:-}"
bundle="${4:-}"
case "$phase" in
    prepare) ;;
    publish) [[ -s "$bundle" ]] || { echo 'Attestation bundle is required.' >&2; exit 1; } ;;
    *) echo 'Expected prepare or publish.' >&2; exit 1 ;;
esac
number='(0|[1-9][0-9]*)'
identifier='(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)'
if [[ ! "$version" =~ ^${number}\.${number}\.${number}(-${identifier}(\.${identifier})*)?$ ]]; then
    echo 'Expected a version such as 0.2.1 or 0.2.1-rc.1.' >&2
    exit 1
fi
if [[ "${GITHUB_ACTIONS:-}" == true && "${GITHUB_REF:-}" != refs/heads/main ]]; then
    echo 'Release must run from main.' >&2
    exit 1
fi
actual_version="$("$binary" version | cut -d ' ' -f 2)"
if [[ "$version" != "$actual_version" ]]; then
    echo "Requested version $version differs from source version $actual_version." >&2
    exit 1
fi
if [[ -n "$(git status --porcelain --untracked-files=normal)" ]]; then
    echo 'Commit or preserve local changes before releasing.' >&2
    exit 1
fi
git fetch origin main --tags
if ! git merge-base --is-ancestor HEAD origin/main; then
    echo 'Release commit must be on origin/main.' >&2
    exit 1
fi
tag="v$version"
if git show-ref --verify --quiet "refs/tags/$tag"; then
    echo "Tag $tag already exists; choose an unreleased version." >&2
    exit 1
fi
commit="$(git rev-parse HEAD)"
if [[ "$phase" == prepare ]]; then
    git tag "$tag" "$commit"
    trap 'git tag -d "$tag" >/dev/null' EXIT
    just goreleaser release --clean --skip=publish
fi

shopt -s nullglob
assets=(output/release/tech-radar_*.tar.gz output/release/tech-radar_*.zip output/release/*_checksums.txt)
if [[ "${#assets[@]}" -ne 7 ]]; then
    echo 'Expected six archives and one checksum file.' >&2
    exit 1
fi
for platform in darwin linux windows; do
    extension=tar.gz
    if [[ "$platform" == windows ]]; then extension=zip; fi
    for architecture in amd64 arm64; do
        test -s "output/release/tech-radar_${version}_${platform}_${architecture}.$extension"
    done
done
checksums="tech-radar_${version}_checksums.txt"
test -s "output/release/$checksums"
test -f output/release/CHANGELOG.md
if [[ "$phase" == prepare ]]; then
    if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
        {
            printf 'subjects<<EOF\n'
            printf '%s\n' "${assets[@]}"
            printf 'EOF\n'
        } >> "$GITHUB_OUTPUT"
    fi
    exit 0
fi

# Recheck the exact bytes and signer after the attestation step, before upload.
if command -v sha256sum >/dev/null; then
    (cd output/release && sha256sum --check "$checksums")
else
    (cd output/release && shasum -a 256 --check "$checksums")
fi
for asset in "${assets[@]}"; do
    gh attestation verify "$asset" --bundle "$bundle" \
        --repo beardedeagle/tech-radar \
        --cert-identity https://github.com/beardedeagle/tech-radar/.github/workflows/release.yml@refs/heads/main \
        --source-ref refs/heads/main --source-digest "$commit" --signer-digest "$commit" \
        --deny-self-hosted-runners
done
flags=()
if [[ "$version" == *-* ]]; then flags+=(--prerelease); fi
gh release create "$tag" "${assets[@]}" --repo beardedeagle/tech-radar \
    --target "$commit" --title "$tag" --notes-file output/release/CHANGELOG.md "${flags[@]}"
