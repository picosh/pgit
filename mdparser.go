package main

import (
	"bytes"

	"github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	ghtml "github.com/yuin/goldmark/renderer/html"
	gtext "github.com/yuin/goldmark/text"
	"go.abhg.dev/goldmark/anchor"
	"go.abhg.dev/goldmark/hashtag"
)

func CreateGoldmark(extenders ...goldmark.Extender) goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extenders...,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			ghtml.WithUnsafe(),
		),
	)
}

func ParseMarkdown(text string) (string, error) {
	hili := highlighting.NewHighlighting(
		highlighting.WithFormatOptions(
			html.WithClasses(true),
		),
	)
	extenders := []goldmark.Extender{
		extension.GFM,
		extension.Footnote,
		meta.Meta,
		&hashtag.Extender{},
		hili,
		&anchor.Extender{
			Position: anchor.After,
			Texter:   anchor.Text("#"),
		},
	}
	md := CreateGoldmark(extenders...)
	context := parser.NewContext()
	btext := []byte(text)
	doc := md.Parser().Parse(gtext.NewReader(btext), parser.WithContext(context))

	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, btext, doc); err != nil {
		return "", err
	}
	return buf.String(), nil
}
