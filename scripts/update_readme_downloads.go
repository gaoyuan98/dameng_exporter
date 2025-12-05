package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

const (
	versionConstPattern = `const\s+Version\s+=\s+"([^"]+)"`

	readmeMarkerStart = "<!-- AUTO_UPDATING_DOWNLOADS_START -->"
	readmeMarkerEnd   = "<!-- AUTO_UPDATING_DOWNLOADS_END -->"

	snippetTemplate = "#### 方式一：下载预编译版本（推荐）\n\n" +
		"访问 [Releases](https://github.com/gaoyuan98/dameng_exporter/releases) 页面下载对应平台的版本：\n\n" +
		"```bash\n" +
		"# Linux AMD64\n" +
		"wget https://github.com/gaoyuan98/dameng_exporter/releases/download/{{.Version}}/dameng_exporter_{{.Version}}_linux_amd64.tar.gz\n" +
		"tar -xzf dameng_exporter_{{.Version}}_linux_amd64.tar.gz\n\n" +
		"# Linux ARM64\n" +
		"wget https://github.com/gaoyuan98/dameng_exporter/releases/download/{{.Version}}/dameng_exporter_{{.Version}}_linux_arm64.tar.gz\n" +
		"tar -xzf dameng_exporter_{{.Version}}_linux_arm64.tar.gz\n\n" +
		"# Windows AMD64\n" +
		"# 下载 dameng_exporter_{{.Version}}_windows_amd64.tar.gz 并解压\n" +
		"```"
)

func main() {
	repo := flag.String("repo", ".", "项目根目录（默认当前目录）")
	flag.Parse()

	root, err := filepath.Abs(*repo)
	checkErr(err)

	version, err := extractVersion(filepath.Join(root, "dameng_exporter.go"))
	checkErr(err)

	snippet, err := renderSnippet(version)
	checkErr(err)

	err = updateReadme(filepath.Join(root, "README.md"), snippet)
	checkErr(err)

	fmt.Printf("README 下载说明已同步版本：%s\n", version)
}

func extractVersion(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取 %s 失败: %w", path, err)
	}

	re := regexp.MustCompile(versionConstPattern)
	match := re.FindStringSubmatch(string(data))
	if len(match) != 2 {
		return "", errors.New("未找到 Version 常量")
	}
	return match[1], nil
}

func renderSnippet(version string) (string, error) {
	tpl, err := template.New("download").Parse(snippetTemplate)
	if err != nil {
		return "", fmt.Errorf("解析模板失败: %w", err)
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, struct{ Version string }{Version: version}); err != nil {
		return "", fmt.Errorf("渲染模板失败: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

func updateReadme(path, snippet string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取 README 失败: %w", err)
	}
	content := string(data)

	startIdx := strings.Index(content, readmeMarkerStart)
	if startIdx == -1 {
		return errors.New("README 缺少开始标记")
	}

	afterStart := startIdx + len(readmeMarkerStart)
	endIdx := strings.Index(content[afterStart:], readmeMarkerEnd)
	if endIdx == -1 {
		return errors.New("README 缺少结束标记")
	}
	absoluteEnd := afterStart + endIdx

	replacement := fmt.Sprintf("%s\n%s\n%s", readmeMarkerStart, snippet, readmeMarkerEnd)
	newContent := content[:startIdx] + replacement + content[absoluteEnd+len(readmeMarkerEnd):]

	return os.WriteFile(path, []byte(newContent), 0o644)
}

func checkErr(err error) {
	if err != nil {
		fmt.Println("错误：", err)
		os.Exit(1)
	}
}
