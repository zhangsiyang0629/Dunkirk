package pipeline

import (
	"fmt"
	"strings"
)

type SSMLRenderer interface {
	Render(text string) string
}

type EdgeRenderer struct{}

func NewEdgeRenderer() *EdgeRenderer { return &EdgeRenderer{} }

type tagSpan struct {
	openIdx  int
	closeIdx int
	tagName  string // "em" or "slow"
}

func (r *EdgeRenderer) Render(text string) string {
	segments := flattenTags(text)
	var sb strings.Builder
	for _, seg := range segments {
		if isSelfClosing(seg.tag) {
			sb.WriteString(seg.text)
			continue
		}

		switch seg.tag {
		case "em":
			sb.WriteString(fmt.Sprintf(`<prosody pitch="+10%%">%s</prosody>`, seg.text))
		case "slow":
			sb.WriteString(fmt.Sprintf(`<prosody rate="slow">%s</prosody>`, seg.text))
		case "fast":
			sb.WriteString(fmt.Sprintf(`<prosody rate="fast">%s</prosody>`, seg.text))
		default:
			sb.WriteString(seg.text)
		}
	}
	return sb.String()
}

type tagText struct {
	tag  string
	text string
}

type frame struct {
	tag     string
	start   int
	content strings.Builder
}

func flattenTags(text string) []tagText {
	var stack []frame
	var result []tagText
	pos := 0

	for pos < len(text) {
		openTag := findOpenTag(text, pos)
		closeTag := findCloseTag(text, pos)
		if openTag != -1 && (closeTag == -1 || openTag < closeTag) {
			tagName := extractTagName(text, openTag)

			if isSelfClosing(tagName) {
				result = append(result, tagText{tag: tagName, text: ""})
				pos = openTag + len(tagName) + 3
				continue
			}

			if tagName != "" {
				if len(stack) > 0 {
					top := &stack[len(stack)-1]
					prevText := text[top.start:openTag]
					top.content.WriteString(prevText)
				}
				f := frame{tag: tagName, start: openTag + len(tagName) + 2}
				stack = append(stack, f)
				pos = openTag + len(tagName) + 2
				continue
			}
		}
		if closeTag != -1 && (openTag == -1 || closeTag < openTag) {
			tagName := extractCloseTagName(text, closeTag)
			if tagName != "" && len(stack) > 0 {
				top := &stack[len(stack)-1]
				if top.tag == tagName {
					content := text[top.start:closeTag]
					result = append(result, tagText{tag: top.tag, text: content})
					stack = stack[:len(stack)-1]

					if len(stack) > 0 {
						outer := &stack[len(stack)-1]
						outer.start = closeTag + len(tagName) + 3 // `[/em]` = 5, `[/slow]` = 7
					}
					pos = closeTag + len(tagName) + 3
					continue
				}
			}
		}
		pos++
	}

	if len(stack) > 0 {
		var buf strings.Builder
		for _, f := range stack {
			buf.WriteString(text[f.start:])
		}
		if buf.Len() > 0 {
			result = append(result, tagText{tag: "", text: buf.String()})
		}
	}
	return result
}

func findOpenTag(text string, pos int) int {
	idx1 := strings.Index(text[pos:], "[em]")
	idx2 := strings.Index(text[pos:], "[slow]")
	result := -1
	if idx1 != -1 && (result == -1 || pos+idx1 < result) {
		result = pos + idx1
	}
	if idx2 != -1 && (result == -1 || pos+idx2 < result) {
		result = pos + idx2
	}
	return result
}

func findCloseTag(text string, pos int) int {
	idx1 := strings.Index(text[pos:], "[/em]")
	idx2 := strings.Index(text[pos:], "[/slow]")
	result := -1
	if idx1 != -1 && (result == -1 || pos+idx1 < result) {
		result = pos + idx1
	}
	if idx2 != -1 && (result == -1 || pos+idx2 < result) {
		result = pos + idx2
	}
	return result
}

func extractTagName(text string, openPos int) string {
	after := text[openPos:]
	if strings.HasPrefix(after, "[em]") {
		return "em"
	}
	if strings.HasPrefix(after, "[slow]") {
		return "slow"
	}
	return ""
}

func extractCloseTagName(text string, closePos int) string {
	after := text[closePos:]
	if strings.HasPrefix(after, "[/em]") {
		return "em"
	}
	if strings.HasPrefix(after, "[/slow]") {
		return "slow"
	}
	return ""
}

func isSelfClosing(tagName string) bool {
	return strings.Contains(tagName, ":")
}
