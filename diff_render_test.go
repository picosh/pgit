package main

import (
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2/styles"
	git "github.com/gogs/git-module"
)

func TestFormatDiffHunkGo(t *testing.T) {
	section := &git.DiffSection{
		Lines: []*git.DiffLine{
			{Type: git.DiffLineSection, Content: "@@ -1,5 +1,6 @@"},
			{Type: git.DiffLinePlain, Content: " package main", LeftLine: 1, RightLine: 1},
			{Type: git.DiffLinePlain, Content: " ", LeftLine: 2, RightLine: 2},
			{Type: git.DiffLineDelete, Content: "-func OldFunction() int {", LeftLine: 3},
			{Type: git.DiffLineAdd, Content: "+func NewFunction() string {", RightLine: 3},
			{Type: git.DiffLineAdd, Content: "+\t// added comment", RightLine: 4},
			{Type: git.DiffLinePlain, Content: " \treturn 42", LeftLine: 4, RightLine: 5},
			{Type: git.DiffLinePlain, Content: " }", LeftLine: 5, RightLine: 6},
		},
	}

	theme := styles.Get("dracula")
	anchor := "diff-main.go-hunk-0"
	htmlOut, err := FormatDiffHunk(theme, "main.go", section, anchor)
	if err != nil {
		t.Fatalf("FormatDiffHunk: %v", err)
	}

	// Verify table structure
	if !strings.Contains(htmlOut, "diff-table") {
		t.Errorf("expected diff-table class in output")
	}
	if !strings.Contains(htmlOut, "diff-line-hunk") {
		t.Errorf("expected diff-line-hunk class in output")
	}

	// Verify line number and diff overlays
	if !strings.Contains(htmlOut, "diff-line-add") {
		t.Errorf("expected diff-line-add class in output")
	}
	if !strings.Contains(htmlOut, "diff-line-delete") {
		t.Errorf("expected diff-line-delete class in output")
	}
	if !strings.Contains(htmlOut, "diff-line-context") {
		t.Errorf("expected diff-line-context class in output")
	}

	// Verify gutter markers
	if !strings.Contains(htmlOut, "<td class=\"diff-gutter\">+</td>") {
		t.Errorf("expected '+' in gutter")
	}
	if !strings.Contains(htmlOut, "<td class=\"diff-gutter\">-</td>") {
		t.Errorf("expected '-' in gutter")
	}

	// Verify anchor links in line numbers
	if !strings.Contains(htmlOut, `id="diff-main.go-hunk-0-L3"`) {
		t.Errorf("expected deleted line anchor id in output, got:\n%s", htmlOut)
	}
	if !strings.Contains(htmlOut, `href="#diff-main.go-hunk-0-L3"`) {
		t.Errorf("expected deleted line anchor href in output, got:\n%s", htmlOut)
	}
	if !strings.Contains(htmlOut, `id="diff-main.go-hunk-0-R3"`) {
		t.Errorf("expected added line anchor id in output, got:\n%s", htmlOut)
	}
	if !strings.Contains(htmlOut, `href="#diff-main.go-hunk-0-R3"`) {
		t.Errorf("expected added line anchor href in output, got:\n%s", htmlOut)
	}

	// Verify syntax highlighting (Chroma classes for Go keywords / comments)
	if !strings.Contains(htmlOut, "class=\"kd\"") && !strings.Contains(htmlOut, "class=\"k\"") {
		t.Errorf("expected keyword chroma class (kd or k) for 'func' or 'package', got:\n%s", htmlOut)
	}
	if !strings.Contains(htmlOut, "class=\"c1\"") && !strings.Contains(htmlOut, "class=\"c\"") {
		t.Errorf("expected comment chroma class (c1 or c) for '// added comment', got:\n%s", htmlOut)
	}
}

func TestFormatDiffHunkZig(t *testing.T) {
	section := &git.DiffSection{
		Lines: []*git.DiffLine{
			{Type: git.DiffLineSection, Content: "@@ -1,4 +1,5 @@"},
			{Type: git.DiffLinePlain, Content: " const std = @import(\"std\");", LeftLine: 1, RightLine: 1},
			{Type: git.DiffLinePlain, Content: " ", LeftLine: 2, RightLine: 2},
			{Type: git.DiffLineDelete, Content: "-pub fn main() void {", LeftLine: 3},
			{Type: git.DiffLineAdd, Content: "+pub fn main() !void {", RightLine: 3},
			{Type: git.DiffLineAdd, Content: "+    // zig comment", RightLine: 4},
			{Type: git.DiffLinePlain, Content: " }", LeftLine: 4, RightLine: 5},
		},
	}

	htmlOut, err := FormatDiffHunk(nil, "src/main.zig", section, "diff-main.zig-hunk-0")
	if err != nil {
		t.Fatalf("FormatDiffHunk: %v", err)
	}

	// Verify Zig syntax highlighting tokens
	if !strings.Contains(htmlOut, "class=\"k\"") && !strings.Contains(htmlOut, "class=\"kd\"") {
		t.Errorf("expected keyword class for 'pub' or 'fn' or 'const', got:\n%s", htmlOut)
	}
	if !strings.Contains(htmlOut, "diff-line-add") {
		t.Errorf("expected diff-line-add in zig diff")
	}
	if !strings.Contains(htmlOut, `href="#diff-main.zig-hunk-0-R3"`) {
		t.Errorf("expected anchor link in zig diff, got:\n%s", htmlOut)
	}
}

func TestFormatDiffHunkFallback(t *testing.T) {
	section := &git.DiffSection{
		Lines: []*git.DiffLine{
			{Type: git.DiffLineSection, Content: "@@ -1,2 +1,2 @@"},
			{Type: git.DiffLineDelete, Content: "-foo", LeftLine: 1},
			{Type: git.DiffLineAdd, Content: "+bar", RightLine: 1},
		},
	}

	// Unknown extension should not error and still render diff table correctly
	htmlOut, err := FormatDiffHunk(nil, "unknown.xyz", section, "")
	if err != nil {
		t.Fatalf("FormatDiffHunk: %v", err)
	}

	if !strings.Contains(htmlOut, "diff-table") {
		t.Errorf("expected diff-table class in output")
	}
	if !strings.Contains(htmlOut, "diff-line-add") {
		t.Errorf("expected diff-line-add in unknown file diff")
	}
}

func TestFormatDiffFile(t *testing.T) {
	file := &git.DiffFile{
		Name: "test.go",
		Sections: []*git.DiffSection{
			{
				Lines: []*git.DiffLine{
					{Type: git.DiffLineSection, Content: "@@ -1,2 +1,2 @@"},
					{Type: git.DiffLineDelete, Content: "-package old", LeftLine: 1},
					{Type: git.DiffLineAdd, Content: "+package new", RightLine: 1},
				},
			},
		},
	}

	htmlOut, err := FormatDiffFile(nil, file)
	if err != nil {
		t.Fatalf("FormatDiffFile: %v", err)
	}

	if !strings.Contains(htmlOut, "diff-test.go-hunk-0") {
		t.Errorf("expected hunk anchor in file diff")
	}
	if !strings.Contains(htmlOut, "diff-line-add") {
		t.Errorf("expected diff-line-add")
	}
}
