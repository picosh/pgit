package main

import (
	"strings"
	"testing"
)

func TestIsMarkdown(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		{"README.md", true},
		{"readme.markdown", true},
		{"doc.MD", true},
		{"main.go", false},
		{"Makefile", false},
		{"file.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			if got := isMarkdown(tt.filename); got != tt.expected {
				t.Errorf("isMarkdown(%q) = %v, expected %v", tt.filename, got, tt.expected)
			}
		})
	}
}

func TestParseMarkdown(t *testing.T) {
	input := `# Hello World

This is a paragraph with **bold** and *italic* text.

## Features

- Item 1
- Item 2

### Code block

` + "```go\nfunc main() {\n\tprintln(\"hello\")\n}\n```" + `

### Table

| Name | Role |
|------|------|
| Alice | Admin |
`

	html, err := ParseMarkdown(input)
	if err != nil {
		t.Fatalf("ParseMarkdown returned error: %v", err)
	}

	// Heading should not be deleted from HTML
	if !strings.Contains(html, `<h1 id="hello-world">Hello World`) {
		t.Errorf("expected HTML to contain rendered <h1>, got:\n%s", html)
	}

	// Heading should have anchor
	if !strings.Contains(html, `<a class="anchor" href="#hello-world"`) {
		t.Errorf("expected anchor on heading, got:\n%s", html)
	}

	// Code block should have chroma classes
	if !strings.Contains(html, `class="chroma"`) {
		t.Errorf("expected code block to have chroma class, got:\n%s", html)
	}

	// Table should be rendered
	if !strings.Contains(html, `<table>`) {
		t.Errorf("expected table to be rendered, got:\n%s", html)
	}
}
