package main

import (
	"bytes"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	formatterHtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	git "github.com/gogs/git-module"
)

var inlineChromaFormatter = formatterHtml.New(
	formatterHtml.WithClasses(true),
	formatterHtml.PreventSurroundingPre(true),
)

func resolveLexer(fileName string) chroma.Lexer {
	lexer := lexers.Match(fileName)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	return chroma.Coalesce(lexer)
}

// FormatDiffHunk formats a git.DiffSection as a syntax-highlighted HTML table
// with diff overlay classes and clickable anchor links on line numbers.
func FormatDiffHunk(theme *chroma.Style, fileName string, section *git.DiffSection, hunkAnchor string) (string, error) {
	if section == nil {
		return "", nil
	}
	if theme == nil {
		theme = styles.Fallback
	}
	lexer := resolveLexer(fileName)

	var buf bytes.Buffer
	buf.WriteString("<div class=\"chroma diff-container\"><table class=\"diff-table\"><tbody class=\"diff-hunk\">")

	var hunkHeader string
	var diffLines []*git.DiffLine

	if len(section.Lines) > 0 && section.Lines[0].Type == git.DiffLineSection {
		hunkHeader = strings.TrimSpace(section.Lines[0].Content)
		diffLines = section.Lines[1:]
	} else {
		diffLines = section.Lines
	}

	if hunkHeader != "" {
		buf.WriteString("<tr class=\"diff-line diff-line-hunk\">")
		buf.WriteString("<td class=\"diff-num diff-num-old\">...</td>")
		buf.WriteString("<td class=\"diff-num diff-num-new\">...</td>")
		buf.WriteString("<td class=\"diff-gutter\"></td>")
		buf.WriteString("<td class=\"diff-code\"><span class=\"gu\">")
		buf.WriteString(html.EscapeString(hunkHeader))
		buf.WriteString("</span></td></tr>\n")
	}

	for _, line := range diffLines {
		var oldNumStr, newNumStr, gutter, rowClass string
		var oldAnchorID, newAnchorID string

		switch line.Type {
		case git.DiffLinePlain:
			if line.LeftLine > 0 {
				oldNumStr = strconv.Itoa(line.LeftLine)
			}
			if line.RightLine > 0 {
				newNumStr = strconv.Itoa(line.RightLine)
			}
			if hunkAnchor != "" {
				if oldNumStr != "" {
					oldAnchorID = fmt.Sprintf("%s-L%s", hunkAnchor, oldNumStr)
				}
				if newNumStr != "" {
					newAnchorID = fmt.Sprintf("%s-R%s", hunkAnchor, newNumStr)
				}
			}
			gutter = " "
			rowClass = "diff-line diff-line-context"
		case git.DiffLineDelete:
			if line.LeftLine > 0 {
				oldNumStr = strconv.Itoa(line.LeftLine)
			}
			newNumStr = ""
			if hunkAnchor != "" && oldNumStr != "" {
				oldAnchorID = fmt.Sprintf("%s-L%s", hunkAnchor, oldNumStr)
			}
			gutter = "-"
			rowClass = "diff-line diff-line-delete"
		case git.DiffLineAdd:
			oldNumStr = ""
			if line.RightLine > 0 {
				newNumStr = strconv.Itoa(line.RightLine)
			}
			if hunkAnchor != "" && newNumStr != "" {
				newAnchorID = fmt.Sprintf("%s-R%s", hunkAnchor, newNumStr)
			}
			gutter = "+"
			rowClass = "diff-line diff-line-add"
		default:
			gutter = " "
			rowClass = "diff-line diff-line-context"
		}

		buf.WriteString("<tr class=\"")
		buf.WriteString(rowClass)
		buf.WriteString("\">")

		// Old line number column with anchor link
		buf.WriteString("<td class=\"diff-num diff-num-old\"")
		if oldAnchorID != "" {
			buf.WriteString(" id=\"")
			buf.WriteString(html.EscapeString(oldAnchorID))
			buf.WriteString("\"")
		}
		if oldNumStr != "" {
			buf.WriteString(" data-line-number=\"")
			buf.WriteString(oldNumStr)
			buf.WriteString("\">")
			if oldAnchorID != "" {
				buf.WriteString("<a href=\"#")
				buf.WriteString(html.EscapeString(oldAnchorID))
				buf.WriteString("\">")
				buf.WriteString(oldNumStr)
				buf.WriteString("</a>")
			} else {
				buf.WriteString(oldNumStr)
			}
		} else {
			buf.WriteString(">")
		}
		buf.WriteString("</td>")

		// New line number column with anchor link
		buf.WriteString("<td class=\"diff-num diff-num-new\"")
		if newAnchorID != "" {
			buf.WriteString(" id=\"")
			buf.WriteString(html.EscapeString(newAnchorID))
			buf.WriteString("\"")
		}
		if newNumStr != "" {
			buf.WriteString(" data-line-number=\"")
			buf.WriteString(newNumStr)
			buf.WriteString("\">")
			if newAnchorID != "" {
				buf.WriteString("<a href=\"#")
				buf.WriteString(html.EscapeString(newAnchorID))
				buf.WriteString("\">")
				buf.WriteString(newNumStr)
				buf.WriteString("</a>")
			} else {
				buf.WriteString(newNumStr)
			}
		} else {
			buf.WriteString(">")
		}
		buf.WriteString("</td>")

		// Gutter column
		buf.WriteString("<td class=\"diff-gutter\">")
		buf.WriteString(html.EscapeString(gutter))
		buf.WriteString("</td>")

		// Code cell
		buf.WriteString("<td class=\"diff-code\">")
		rawLine := line.Content
		var lineContent string
		if len(rawLine) > 0 && (rawLine[0] == ' ' || rawLine[0] == '+' || rawLine[0] == '-') {
			lineContent = rawLine[1:]
		} else {
			lineContent = rawLine
		}
		lineContent = strings.TrimRight(lineContent, "\r\n")
		if lineContent == "" {
			buf.WriteString("\n")
		} else {
			it, err := lexer.Tokenise(nil, lineContent)
			if err != nil {
				buf.WriteString(html.EscapeString(lineContent))
			} else {
				if err := inlineChromaFormatter.Format(&buf, theme, it); err != nil {
					buf.WriteString(html.EscapeString(lineContent))
				}
			}
		}
		buf.WriteString("</td></tr>\n")
	}

	buf.WriteString("</tbody></table></div>")
	return buf.String(), nil
}

// FormatDiffFile formats all hunks in a git.DiffFile into syntax-highlighted HTML.
func FormatDiffFile(theme *chroma.Style, file *git.DiffFile) (string, error) {
	if file == nil {
		return "", nil
	}
	if file.IsBinary() {
		return "<div class=\"chroma diff-container\"><pre style=\"padding: var(--grid-height);\">Binaries are not rendered as diffs.</pre></div>", nil
	}
	if len(file.Sections) == 0 {
		return "", nil
	}

	fileName := file.Name
	if fileName == "" {
		fileName = file.OldName()
	}

	var buf bytes.Buffer
	for hunkIdx, section := range file.Sections {
		hunkAnchor := fmt.Sprintf("diff-%s-hunk-%d", fileName, hunkIdx)
		hunkHtml, err := FormatDiffHunk(theme, fileName, section, hunkAnchor)
		if err != nil {
			return "", err
		}
		buf.WriteString(hunkHtml)
	}
	return buf.String(), nil
}
