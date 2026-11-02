set shell := ["bash", "-euo", "pipefail", "-c"]
set windows-shell := ["bash", "-euo", "pipefail", "-c"]
set positional-arguments

automation_go := "go1.27.3"
actionlint_version := "1.7.12"
lefthook_version := "2.2.2"
goreleaser_version := "2.19.0"
hugo_version := "0.169.0"
actions_lock_revision := "113f91d77e0af03838b6ae3bef0e038cbab4c696"
codeql_version := "2.27.3"
exe := if os() == "windows" { ".exe" } else { "" }
binary := "tech-radar" + exe
tools_dir := justfile_directory() / "output" / "tools"
actionlint_dir := tools_dir / ("actionlint-" + actionlint_version)
lefthook_dir := tools_dir / ("lefthook-" + lefthook_version)
goreleaser_dir := tools_dir / ("goreleaser-" + goreleaser_version)
hugo_dir := tools_dir / ("hugo-" + hugo_version)
actions_lock_dir := tools_dir / ("actions-lock-" + actions_lock_revision)
codeql_dir := tools_dir / ("codeql-" + codeql_version)

default:
    @just --list

# Install pinned tools and local Git hooks.
setup: tools
    just lefthook install

# Keep development tools outside the application's Go module and PATH.
tools: _actionlint _lefthook _goreleaser _hugo _actions-lock

# Run every check used by CI on the current platform.
ci: check test build docs-build release-snapshot security

# Run the native Go checks on each CI platform.
ci-go: check-go test build

# Run the pre-commit checks without modifying source files.
check: check-go check-automation

check-go:
    mkdir -p output
    gofmt -l cmd internal | tee output/gofmt.txt
    test ! -s output/gofmt.txt
    go mod verify
    go vet ./...

check-automation: _actionlint _lefthook _goreleaser _actions-lock
    just --fmt --check
    "{{ actionlint_dir }}/actionlint{{ exe }}"
    just lefthook validate
    just goreleaser check
    "{{ actions_lock_dir }}/gh-actions-lock{{ exe }}" --verify-local

# Format Go code and the Justfile.
fmt:
    gofmt -w cmd internal
    just --fmt

test *packages="./...":
    go test -race "$@"

build:
    mkdir -p output
    go build -trimpath -o "{{ binary }}" ./cmd/tech-radar

version: build
    "./{{ binary }}" version

# Build the handbook using its actual Pages URL unless explicitly overridden.
docs-build: _hugo
    "{{ hugo_dir }}/hugo{{ exe }}" --source docs --minify --panicOnWarning --baseURL "${HUGO_BASEURL:-https://beardedeagle.github.io/tech-radar/}"

docs-serve: _hugo
    "{{ hugo_dir }}/hugo{{ exe }}" server --source docs --bind 127.0.0.1

# Build all six release archives without tags or publication.
release-snapshot: _goreleaser
    just goreleaser release --snapshot --clean --skip=publish

# Run the release workflow on main using the chosen source version.
release version:
    gh workflow run release.yml --repo beardedeagle/tech-radar --ref main -f "version=$1"

# Regenerate after changing workflow dependencies. Review the lockfile diff.
actions-lock: _actions-lock
    "{{ actions_lock_dir }}/gh-actions-lock{{ exe }}" --no-narrow --relock

# Verify remote action provenance and analyze Go and workflow code.
security: actions-verify codeql

actions-verify: _actions-lock
    "{{ actions_lock_dir }}/gh-actions-lock{{ exe }}" --verify

codeql: _codeql
    rm -rf output/codeql
    mkdir -p output/codeql
    "{{ codeql_dir }}/codeql/codeql{{ exe }}" database create output/codeql/go --language=go --command='just _codeql-go-extract' --no-tracing --source-root=.
    "{{ codeql_dir }}/codeql/codeql{{ exe }}" database analyze output/codeql/go codeql/go-queries:codeql-suites/go-security-extended.qls --format=sarifv2.1.0 --sarif-category=go --output=output/codeql/go.sarif --threads=2
    "{{ codeql_dir }}/codeql/codeql{{ exe }}" database create output/codeql/actions --language=actions --source-root=.
    "{{ codeql_dir }}/codeql/codeql{{ exe }}" database analyze output/codeql/actions codeql/actions-queries:codeql-suites/actions-security-extended.qls --format=sarifv2.1.0 --sarif-category=actions --output=output/codeql/actions.sarif --threads=2

# Extract the root module directly; autobuild can rewrite module pins.
[private]
_codeql-go-extract:
    GOFLAGS="${GOFLAGS:-} -mod=readonly" "${CODEQL_EXTRACTOR_GO_ROOT}/tools/${CODEQL_PLATFORM}/go-extractor{{ exe }}" ./...

# Invoke the pinned tools without depending on globally installed copies.
lefthook *args: _lefthook
    "{{ lefthook_dir }}/lefthook{{ exe }}" "$@"

goreleaser *args: _goreleaser
    "{{ goreleaser_dir }}/goreleaser{{ exe }}" "$@"

[private]
_release-publish version: build _goreleaser
    #!/usr/bin/env bash
    set -euo pipefail
    version="${1#v}"
    number='(0|[1-9][0-9]*)'
    identifier='(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)'
    if [[ ! "$version" =~ ^${number}\.${number}\.${number}(-${identifier}(\.${identifier})*)?$ ]]; then
        echo "Expected a version such as 0.2.0 or 0.2.0-rc.1." >&2
        exit 1
    fi
    if [[ "${GITHUB_ACTIONS:-}" == true && "${GITHUB_REF:-}" != refs/heads/main ]]; then
        echo "Release must run from main." >&2
        exit 1
    fi
    actual_version="$("./{{ binary }}" version | cut -d ' ' -f 2)"
    if [[ "$version" != "$actual_version" ]]; then
        echo "Requested version $version differs from source version $actual_version." >&2
        exit 1
    fi
    if [[ -n "$(git status --porcelain --untracked-files=normal)" ]]; then
        echo "Commit or preserve local changes before releasing." >&2
        exit 1
    fi
    git fetch origin main --tags
    if ! git merge-base --is-ancestor HEAD origin/main; then
        echo "Release commit must be on origin/main." >&2
        exit 1
    fi
    tag="v$version"
    if git show-ref --verify --quiet "refs/tags/$tag"; then
        echo "Tag $tag already exists; choose an unreleased version." >&2
        exit 1
    fi
    commit="$(git rev-parse HEAD)"
    git tag "$tag" "$commit"
    trap 'git tag -d "$tag" >/dev/null' EXIT
    just goreleaser release --clean --skip=publish
    shopt -s nullglob
    assets=(output/release/tech-radar_*.tar.gz output/release/tech-radar_*.zip output/release/*_checksums.txt)
    if [[ "${#assets[@]}" -ne 7 ]]; then
        echo "Expected six archives and one checksum file." >&2
        exit 1
    fi
    flags=()
    if [[ "$version" == *-* ]]; then
        flags+=(--prerelease)
    fi
    gh release create "$tag" "${assets[@]}" --repo beardedeagle/tech-radar \
        --target "$commit" --title "$tag" --notes-file output/release/CHANGELOG.md "${flags[@]}"

[private]
_actionlint:
    @if [ ! -x "{{ actionlint_dir }}/actionlint{{ exe }}" ]; then mkdir -p "{{ actionlint_dir }}"; GOTOOLCHAIN={{ automation_go }} GOBIN="{{ actionlint_dir }}" go install github.com/rhysd/actionlint/cmd/actionlint@v{{ actionlint_version }}; fi

[private]
_lefthook:
    @if [ ! -x "{{ lefthook_dir }}/lefthook{{ exe }}" ]; then mkdir -p "{{ lefthook_dir }}"; GOTOOLCHAIN={{ automation_go }} GOBIN="{{ lefthook_dir }}" go install github.com/evilmartians/lefthook/v2@v{{ lefthook_version }}; fi

[private]
_goreleaser:
    @if [ ! -x "{{ goreleaser_dir }}/goreleaser{{ exe }}" ]; then mkdir -p "{{ goreleaser_dir }}"; GOTOOLCHAIN={{ automation_go }} GOBIN="{{ goreleaser_dir }}" go install github.com/goreleaser/goreleaser/v2@v{{ goreleaser_version }}; fi

[private]
_hugo:
    @if [ ! -x "{{ hugo_dir }}/hugo{{ exe }}" ]; then mkdir -p "{{ hugo_dir }}"; GOTOOLCHAIN={{ automation_go }} GOBIN="{{ hugo_dir }}" go install github.com/gohugoio/hugo@v{{ hugo_version }}; fi

[private]
_actions-lock:
    @if [ ! -x "{{ actions_lock_dir }}/gh-actions-lock{{ exe }}" ]; then mkdir -p "{{ actions_lock_dir }}"; GOTOOLCHAIN={{ automation_go }} GOBIN="{{ actions_lock_dir }}" go install github.com/github/gh-actions-lock/cmd/gh-actions-lock@{{ actions_lock_revision }}; fi

[private]
_codeql:
    #!/usr/bin/env bash
    set -euo pipefail
    if [[ -x "{{ codeql_dir }}/codeql/codeql{{ exe }}" ]]; then exit 0; fi
    case "$(uname -s)" in
        Linux) platform=linux64; checksum=64518dc413c8e7b5733fee2489e830a899bb1e3f28c8873b5869d5cc90164c5c ;;
        Darwin) platform=osx64; checksum=5a035e81cd8c76622d0e453d1d5197cfee46177f171d623f15fbb560a0b2b215 ;;
        MINGW*) platform=win64; checksum=b1a54af0e0657bc8dd2201474373aae6cee72c20d820a3a89423013f2465533d ;;
        *) echo 'Unsupported CodeQL platform.' >&2; exit 1 ;;
    esac
    temporary="$(mktemp -d)"
    trap 'rm -rf "$temporary"' EXIT
    curl --fail --silent --show-error --location --retry 3 \
        "https://github.com/github/codeql-action/releases/download/codeql-bundle-v{{ codeql_version }}/codeql-bundle-$platform.tar.gz" -o "$temporary/bundle.tar.gz"
    if command -v sha256sum >/dev/null; then
        actual="$(sha256sum "$temporary/bundle.tar.gz" | cut -d ' ' -f 1)"
    else
        actual="$(shasum -a 256 "$temporary/bundle.tar.gz" | cut -d ' ' -f 1)"
    fi
    [[ "$actual" == "$checksum" ]] || { echo 'CodeQL checksum mismatch.' >&2; exit 1; }
    tar -xzf "$temporary/bundle.tar.gz" -C "$temporary"
    mkdir -p "{{ tools_dir }}"
    mv "$temporary/codeql" "{{ codeql_dir }}.tmp"
    mkdir -p "{{ codeql_dir }}"
    mv "{{ codeql_dir }}.tmp" "{{ codeql_dir }}/codeql"
