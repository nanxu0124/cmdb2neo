package cypher

import (
	"embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed *.cql
var files embed.FS

// MustTemplate 解析指定模板并渲染，失败直接 panic，便于在初始化阶段暴露错误。
func MustTemplate(name string, data any) string {
	tmpl, err := template.New(name).ParseFS(files, name)
	if err != nil {
		panic(fmt.Errorf("parse template %s failed: %w", name, err))
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		panic(fmt.Errorf("execute template %s failed: %w", name, err))
	}
	return sb.String()
}

// MustAsset 返回模板原文。
func MustAsset(name string) string {
	b, err := files.ReadFile(name)
	if err != nil {
		panic(fmt.Errorf("load %s failed: %w", name, err))
	}
	return string(b)
}
