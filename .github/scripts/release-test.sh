#!/usr/bin/env bash
# Exercise release guards with a local remote and intercepted publication.
set -euo pipefail

# Hooks export repository-local settings that must not reach temporary Git repos.
while IFS= read -r variable; do
    unset "$variable"
done < <(git rev-parse --local-env-vars)

scripts="$(cd -- "$(dirname -- "$0")" && pwd -P)"
temporary="$(mktemp -d)"
trap 'rm -rf "$temporary"' EXIT
fixture="$temporary/repo with spaces"
mkdir -p "$fixture" "$temporary/bin"
git init --bare --quiet "$temporary/origin.git"
git init --quiet --initial-branch=main "$fixture"
cd "$fixture"
printf 'output/\ntech-radar\n' > .gitignore
git add .gitignore
git -c user.name=Test -c user.email=test@example.invalid commit --quiet -m fixture
git remote add origin "$temporary/origin.git"
git push --quiet origin main
# The isolation regression only needs to exercise the real fixture setup above.
if [[ "${1:-}" == --git-env-isolation ]]; then exit 0; fi
export RELEASE_TEST_VERSION=0.2.1 RELEASE_TEST_BUILD_EXIT=0 RELEASE_TEST_FAIL_ASSET=''
export RELEASE_TEST_PUBLISH_EXIT=0
export RELEASE_TEST_GH_LOG="$temporary/gh.jsonl"
export GITHUB_ACTIONS=true GITHUB_REF=refs/heads/main GITHUB_OUTPUT="$temporary/outputs"
cat > tech-radar <<'SH'
#!/bin/sh
printf 'tech-radar %s\n' "$RELEASE_TEST_VERSION"
SH
cat > "$temporary/bin/just" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
[[ "$*" == 'goreleaser release --clean --skip=publish' ]]
if [[ "$RELEASE_TEST_BUILD_EXIT" != 0 ]]; then exit "$RELEASE_TEST_BUILD_EXIT"; fi
rm -rf output/release
mkdir -p output/release
for platform in darwin linux windows; do
    extension=tar.gz
    if [[ "$platform" == windows ]]; then extension=zip; fi
    for architecture in amd64 arm64; do
        printf '%s %s\n' "$platform" "$architecture" > "output/release/tech-radar_${RELEASE_TEST_VERSION}_${platform}_${architecture}.$extension"
    done
done
cd output/release
if command -v sha256sum >/dev/null; then
    sha256sum ./*.tar.gz ./*.zip > "tech-radar_${RELEASE_TEST_VERSION}_checksums.txt"
else
    shasum -a 256 ./*.tar.gz ./*.zip > "tech-radar_${RELEASE_TEST_VERSION}_checksums.txt"
fi
printf 'Release notes\n' > CHANGELOG.md
SH
cat > "$temporary/bin/gh" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$@" | jq -Rs 'split("\n")[:-1]' >> "$RELEASE_TEST_GH_LOG"
case "$1 $2" in
    'attestation verify') [[ "$3" != "$RELEASE_TEST_FAIL_ASSET" ]] ;;
    'release create') exit "$RELEASE_TEST_PUBLISH_EXIT" ;;
    *) exit 2 ;;
esac
SH
chmod +x tech-radar "$temporary/bin/just" "$temporary/bin/gh"
export PATH="$temporary/bin:$PATH"
printf 'Test attestation bundle\n' > "$temporary/bundle.json"
bundle="$temporary/bundle.json"

run() {
    local name="$1" expected="$2" status=0 before
    shift 2
    before="$(git tag --list)"
    : > "$RELEASE_TEST_GH_LOG"
    : > "$GITHUB_OUTPUT"
    bash "$scripts/release.sh" "$@" > "$temporary/log" 2>&1 || status=$?
    if [[ "$status" != "$expected" ]]; then
        printf 'FAIL: %s: expected %s, got %s\n' "$name" "$expected" "$status" >&2
        cat "$temporary/log" >&2
        exit 1
    fi
    [[ "$(git tag --list)" == "$before" ]]
    if [[ "$expected" != 0 ]]; then
        jq -es 'all(.[]; .[0:2] != ["release", "create"])' "$RELEASE_TEST_GH_LOG" >/dev/null
    fi
    printf 'ok: %s\n' "$name"
}

run 'prepare exports seven subjects and removes its local tag' 0 prepare v0.2.1 ./tech-radar
[[ ! -s "$RELEASE_TEST_GH_LOG" ]]
[[ "$(wc -l < "$GITHUB_OUTPUT" | tr -d ' ')" == 9 ]]
run 'publish verifies every asset before creating the release' 0 publish 0.2.1 ./tech-radar "$bundle"
jq -es --arg commit "$(git rev-parse HEAD)" --arg bundle "$bundle" '
    length == 8 and .[7][0:2] == ["release", "create"] and
    ([.[0:7][] | .[2]] | unique | length) == 7 and
    all(.[0:7][]; .[0:2] == ["attestation", "verify"] and
        .[index("--bundle") + 1] == $bundle and
        .[index("--repo") + 1] == "beardedeagle/tech-radar" and
        .[index("--cert-identity") + 1] == "https://github.com/beardedeagle/tech-radar/.github/workflows/release.yml@refs/heads/main" and
        .[index("--source-ref") + 1] == "refs/heads/main" and
        .[index("--source-digest") + 1] == $commit and
        .[index("--signer-digest") + 1] == $commit and
        index("--deny-self-hosted-runners") != null)
' "$RELEASE_TEST_GH_LOG" >/dev/null

RELEASE_TEST_FAIL_ASSET=output/release/tech-radar_0.2.1_checksums.txt
run 'failed checksum-manifest attestation blocks publication' 1 publish 0.2.1 ./tech-radar "$bundle"
RELEASE_TEST_FAIL_ASSET=output/release/tech-radar_0.2.1_darwin_arm64.tar.gz
run 'failed archive attestation blocks publication' 1 publish 0.2.1 ./tech-radar "$bundle"
RELEASE_TEST_FAIL_ASSET=''
run 'missing attestation bundle blocks publication' 1 publish 0.2.1 ./tech-radar
printf 'changed\n' >> output/release/tech-radar_0.2.1_linux_arm64.tar.gz
run 'changed archive bytes block publication' 1 publish 0.2.1 ./tech-radar "$bundle"
run 'prepare restores the package set' 0 prepare 0.2.1 ./tech-radar
rm output/release/tech-radar_0.2.1_windows_arm64.zip
run 'missing target blocks publication' 1 publish 0.2.1 ./tech-radar "$bundle"

for version in 01.2.1 0.2.1+build 0.2.1-01; do
    run 'noncanonical versions are rejected' 1 prepare "$version" ./tech-radar
done
run 'source version mismatch is rejected' 1 prepare 0.2.2 ./tech-radar
GITHUB_REF=refs/heads/topic
run 'release outside main is rejected' 1 prepare 0.2.1 ./tech-radar
GITHUB_REF=refs/heads/main
printf 'untracked\n' > dirty.txt
run 'dirty checkout is rejected' 1 prepare 0.2.1 ./tech-radar
rm dirty.txt
git tag v0.2.1
run 'existing tag is rejected' 1 prepare 0.2.1 ./tech-radar
git tag -d v0.2.1 >/dev/null
git -c user.name=Test -c user.email=test@example.invalid commit --quiet --allow-empty -m unpublished
run 'commit outside origin/main is rejected' 1 prepare 0.2.1 ./tech-radar
git push --quiet origin main
RELEASE_TEST_BUILD_EXIT=1
run 'failed packaging removes its temporary tag' 1 prepare 0.2.1 ./tech-radar
RELEASE_TEST_BUILD_EXIT=0
RELEASE_TEST_VERSION=0.2.1-rc.1
run 'prerelease preparation succeeds' 0 prepare v0.2.1-rc.1 ./tech-radar
run 'prerelease publication retains its flag' 0 publish v0.2.1-rc.1 ./tech-radar "$bundle"
jq -es '.[-1] | index("--prerelease") != null' "$RELEASE_TEST_GH_LOG" >/dev/null

# Reproduce a hook's repository-local environment using a disposable worktree.
hook_fixture="$temporary/hook worktree"
git worktree add --quiet -b hook-env "$hook_fixture"
git_directory="$(git -C "$hook_fixture" rev-parse --absolute-git-dir)"
common_directory="$(git rev-parse --path-format=absolute --git-common-dir)"
index="$temporary/hook-index"
cp "$git_directory/index" "$index"
printf 'staged fixture content\n' > "$hook_fixture/staged.txt"
GIT_INDEX_FILE="$index" git -C "$hook_fixture" add staged.txt
cp "$index" "$temporary/index-before"
cp "$git_directory/index" "$temporary/worktree-index-before"
cp "$common_directory/config" "$temporary/config-before"
cp "$hook_fixture/.gitignore" "$temporary/gitignore-before"
git show-ref > "$temporary/refs-before"
before="$(GIT_INDEX_FILE="$index" git -C "$hook_fixture" status --porcelain=v1)"
(
    cd "$hook_fixture"
    export GIT_DIR="$git_directory" GIT_WORK_TREE="$hook_fixture"
    export GIT_COMMON_DIR="$common_directory" GIT_INDEX_FILE="$index" GIT_PREFIX=''
    export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=core.bare GIT_CONFIG_VALUE_0=true
    bash "$scripts/release-test.sh" --git-env-isolation
) > "$temporary/git-env.log" 2>&1 || {
    cat "$temporary/git-env.log" >&2
    exit 1
}
cmp "$index" "$temporary/index-before"
cmp "$git_directory/index" "$temporary/worktree-index-before"
cmp "$common_directory/config" "$temporary/config-before"
cmp "$hook_fixture/.gitignore" "$temporary/gitignore-before"
git show-ref > "$temporary/refs-after"
cmp "$temporary/refs-before" "$temporary/refs-after"
[[ "$(GIT_INDEX_FILE="$index" git -C "$hook_fixture" status --porcelain=v1)" == "$before" ]]
printf 'ok: inherited Git environment preserves the outer worktree, indexes, config and refs\n'
