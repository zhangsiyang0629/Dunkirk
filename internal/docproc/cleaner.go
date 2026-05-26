package docproc

import (
	"regexp"
	"strings"
)

func CleanMarkitdownOutput(content string) string {
	lines := strings.Split(content, "\n")

	startIdx := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		_, ok := matchTitle(trimmed)
		if ok && !strings.Contains(trimmed, "..") &&
			!strings.HasPrefix(strings.TrimLeft(line, " "), "【") {
			startIdx = i
			break
		}
	}
	lines = lines[startIdx:]

	pageHeaderRe := regexp.MustCompile(`^\s*【.+】`)
	numRe := regexp.MustCompile(`^\d+$`)
	tocLineRe := regexp.MustCompile(`第[一二三四五六七八九十百零\d]+[回章节部集话].+\.{2,}\d+\s*$`)
	clean := make(map[string]struct{})
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "\f" {
			continue
		}
		if pageHeaderRe.MatchString(trimmed) {
			continue
		}
		if numRe.MatchString(trimmed) {
			continue
		}
		if tocLineRe.MatchString(trimmed) {
			continue
		}
		clean[line] = struct{}{} // 保留空白行和内容行
	}

	var paragraphs []string
	var buf strings.Builder
	for _, line := range lines {
		_, ok := clean[line]
		if !ok {
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// 章节标题 → 独立一段
		if _, ok := matchTitle(trimmed); ok {
			if buf.Len() > 0 {
				paragraphs = append(paragraphs, strings.TrimSpace(buf.String()))
				buf.Reset()
			}
			paragraphs = append(paragraphs, trimmed)
			continue
		}
		// 行尾判定：句号/感叹号/问号结尾 → 段落结束
		runes := []rune(trimmed)
		isSentenceEnd := len(runes) > 0 && strings.Contains("。！？", string(runes[len(runes)-1]))
		if buf.Len() > 0 {
			buf.WriteString(" ")
		}
		buf.WriteString(trimmed)
		if isSentenceEnd {
			paragraphs = append(paragraphs, strings.TrimSpace(buf.String()))
			buf.Reset()
		}
	}
	if buf.Len() > 0 {
		paragraphs = append(paragraphs, strings.TrimSpace(buf.String()))
	}
	return strings.Join(paragraphs, "\n\n")
}
