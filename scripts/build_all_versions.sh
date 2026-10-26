#!/usr/bin/env bash
set -euo pipefail

# 兼容 macOS 自带 Bash 3.2；从任意工作目录调用均可定位项目。
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
cd "$repo_root"

for tool in go tar; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        printf 'Required command not found: %s\n' "$tool" >&2
        exit 1
    fi
done

program_name=dameng_exporter
version="$(sed -nE 's/^const Version = "([^"]+)"$/\1/p' dameng_exporter.go)"
if [[ -z "$version" ]]; then
    printf 'Cannot find Version in dameng_exporter.go\n' >&2
    exit 1
fi

git_revision="$(git rev-parse HEAD 2>/dev/null || printf 'unknown')"
git_branch="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || printf 'unknown')"
ldflags="-s -w -X dameng_exporter/collector.GitRevision=$git_revision -X dameng_exporter/collector.GitBranch=$git_branch"

host_os="$(go env GOHOSTOS)"
host_arch="$(go env GOHOSTARCH)"
GOOS="$host_os" GOARCH="$host_arch" go run -mod=readonly scripts/update_readme_downloads.go

output_dir="$repo_root/dist"
mkdir -p "$output_dir"
staging_dir="$(mktemp -d "$output_dir/.build.XXXXXX")"
trap 'rm -rf "$staging_dir"' EXIT

printf 'Building %s %s (%s, %s)\n' "$program_name" "$version" "$git_branch" "$git_revision"
for target in windows/amd64 linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
    target_os="${target%/*}"
    target_arch="${target#*/}"
    package_name="${program_name}_${version}_${target_os}_${target_arch}"
    package_dir="$staging_dir/$package_name"
    binary_name="$program_name"
    if [[ "$target_os" == windows ]]; then
        binary_name="$binary_name.exe"
    fi

    printf 'Building %s...\n' "$target"
    mkdir -p "$package_dir"
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" \
        go build -mod=readonly -trimpath -ldflags "$ldflags" -o "$package_dir/$binary_name" .
    cp docs/config/dameng_exporter.toml docs/config/custom_queries.metrics LICENSE "$package_dir/"
    cp -R docs/dashboards "$package_dir/dashboards"
    # 防止 macOS tar 将 AppleDouble 元数据一起放入跨平台安装包。
    COPYFILE_DISABLE=1 tar -czf "$staging_dir/$package_name.tar.gz" -C "$staging_dir" "$package_name"
done

# 所有目标均成功后再输出安装包。
for archive in "$staging_dir"/*.tar.gz; do
    mv -f "$archive" "$output_dir/"
done
printf 'All 5 packages are ready in %s\n' "$output_dir"
