package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/template"

	"honnef.co/go/tools/analysis/lint"
	"honnef.co/go/tools/staticcheck"
)

var ignoredLinters = map[string]string{
	"SAXXXX": "I am an exmaple for a linter that should be ignored",
}
var analyzers []*lint.Analyzer = sortedAnalyzers()

var BazelBuildTemplate = `# GENERATED FILE - DO NOT EDIT
# This file was generated by running go generate on dev/linters/staticcheck
#
# If you want to ignore an analyzer add it to the ignore list in dev/linters/staticcheck/cmd/gen.go,
# and re-run go generate

load("@io_bazel_rules_go//go:def.bzl", "go_library")
{{ range .Analyzers}}
go_library(
    name = "{{.Analyzer.Name}}",
    srcs = ["staticcheck.go"],
    importpath = "github.com/sourcegraph/sourcegraph/dev/linters/staticcheck/{{.Analyzer.Name}}",
    visibility = ["//visibility:public"],
    x_defs = {"AnalyzerName": "{{.Analyzer.Name}}"},
    deps = [
        "//dev/linters/nolint",
        "@co_honnef_go_tools//analysis/lint",
        "@co_honnef_go_tools//staticcheck",
        "@org_golang_x_tools//go/analysis",
    ],
)
{{ end}}
go_library(
    name = "staticcheck",
    srcs = ["staticcheck.go"],
    importpath = "github.com/sourcegraph/sourcegraph/dev/linters/staticcheck",
    visibility = ["//visibility:public"],
    deps = [
        "//dev/linters/nolint",
        "@co_honnef_go_tools//analysis/lint",
        "@co_honnef_go_tools//staticcheck",
        "@org_golang_x_tools//go/analysis",
    ],
)
`

var BazelDefTemplate = `# DO NOT EDIT - this file was generated by running go generate on dev/linters/staticcheck
#
# If you want to ignore an analyzer add it to the ignore list in dev/linters/staticcheck/cmd/gen.go,
# and re-run go generate

STATIC_CHECK_ANALYZERS = [
{{- range .Analyzers}}
	"//dev/linters/staticcheck:{{.Analyzer.Name}}",
{{- end}}
]
`

func sortedAnalyzers() []*lint.Analyzer {
	linters := make([]*lint.Analyzer, 0)
	// remove ignored linters first
	for _, linter := range staticcheck.Analyzers {
		if _, shouldIgnore := ignoredLinters[linter.Analyzer.Name]; !shouldIgnore {
			linters = append(linters, linter)
		}
	}
	// now sort them
	sort.SliceStable(linters, func(i, j int) bool {
		return strings.Compare(linters[i].Analyzer.Name, linters[j].Analyzer.Name) < 0
	})
	return linters
}

func writeTemplate(targetFile, templateDef string) error {
	name := targetFile
	tmpl := template.Must(template.New(name).Parse(templateDef))

	f, err := os.OpenFile(targetFile, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	err = tmpl.Execute(f, struct {
		Analyzers []*lint.Analyzer
	}{
		Analyzers: analyzers,
	})
	if err != nil {
		return err
	}

	return nil
}

// We support two position arguments:
// 1: buildfile path - file where the analyzer targets should be generated to
// 2: analyzer definition path - file where a convienience analyzer array is generated that contains all the targets
func main() {
	targetFile := "BUILD.bazel"
	if len(os.Args) > 1 {
		targetFile = os.Args[1]
	}

	// Generate targets for all the analyzers
	if err := writeTemplate(targetFile, BazelBuildTemplate); err != nil {
		fmt.Fprintln(os.Stderr, "failed to render Bazel buildfile template")
		panic(err)
	}

	// Generate a file where we can import the list of analyzers into our bazel scripts
	targetFile = "analyzers.bzl"
	if len(os.Args) > 2 {
		targetFile = os.Args[2]
	}
	if err := writeTemplate(targetFile, BazelDefTemplate); err != nil {
		fmt.Fprintln(os.Stderr, "failed to render Anazlyers definiton template")
		panic(err)
	}

}
