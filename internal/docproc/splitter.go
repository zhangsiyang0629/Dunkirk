package docproc

import (
	"regexp"
	"strings"
)

type Chapter struct {
	Index   int
	Title   string
	Content string
}

type Segment struct {
	Content string
	Index   int
}

var titleRegex = regexp.MustCompile(`第[一二三四五六七八九十百零\d]+[回章节部集话]`)

func detectParagraphSep(lines []string) func(prevLine, currLine string) bool {
	n := len(lines)
	emptyLineCnt := 0
	indentCnt := 0
	for i := 1; i < n; i++ {
		prev := strings.TrimSpace(lines[i-1])
		curr := lines[i]
		if prev == "" && strings.TrimSpace(curr) != "" {
			emptyLineCnt++
		}
		// 前一行无缩进、当前行有缩进 → 段落开始
		if strings.TrimSpace(prev) != "" &&
			strings.TrimLeft(lines[i-1], " \t") == lines[i-1] &&
			strings.TrimLeft(curr, " \t") != curr {
			indentCnt++
		}
	}
	if indentCnt > emptyLineCnt && indentCnt > 3 {
		return func(prevLine, currLine string) bool {
			prevTrimmed := strings.TrimSpace(prevLine)
			if prevTrimmed == "" {
				return false
			}
			return strings.TrimLeft(currLine, " \t") != currLine &&
				strings.TrimLeft(prevLine, " \t") == prevLine
		}
	}
	return nil
}

func matchTitle(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "# ") {
		return strings.TrimPrefix(trimmed, "# "), true
	}
	if titleRegex.MatchString(trimmed) {
		return trimmed, true
	}
	return "", false
}

func SplitByChapters(content string) []Chapter {
	lines := strings.Split(content, "\n")
	// 扫描所有标题行及其行号
	type marker struct {
		lineNum int
		title   string
	}
	var markers []marker
	for i, line := range lines {
		if title, ok := matchTitle(strings.TrimSpace(line)); ok {
			markers = append(markers, marker{lineNum: i, title: title})
		}
	}
	if len(markers) == 0 {
		return nil
	}

	var deduped []marker
	for _, m := range markers {
		if len(deduped) > 0 && extractChapterNum(deduped[len(deduped)-1].title) == extractChapterNum(m.title) {
			continue
		}
		deduped = append(deduped, m)
	}
	markers = deduped

	// 找到正文开始的第一个标题：相邻标题行之间如有实质内容（非空白、非页码）则判定为正文
	contentStartIdx := 0
	for i := 0; i < len(markers); i++ {
		nextLine := len(lines)
		if i+1 < len(markers) {
			nextLine = markers[i+1].lineNum
		}
		realLines := 0
		for _, l := range lines[markers[i].lineNum:nextLine] {
			t := strings.TrimSpace(l)
			if t == "" {
				continue
			}
			if strings.ContainsAny(t, ".…·") || isPageNumber(t) {
				continue
			}
			realLines++
		}
		if realLines > 2 {
			contentStartIdx = i
			break
		}
	}
	type chapter struct {
		marker
		endLine int
	}
	var chapters []chapter
	for i := contentStartIdx; i < len(markers); i++ {
		endLine := len(lines)
		if i+1 < len(markers) {
			endLine = markers[i+1].lineNum
		}
		chapters = append(chapters, chapter{
			marker:  markers[i],
			endLine: endLine,
		})
	}
	result := make([]Chapter, 0, len(chapters))
	for i, ch := range chapters {
		content := strings.Join(lines[ch.lineNum:ch.endLine], "\n")
		result = append(result, Chapter{
			Index:   i + 1,
			Title:   ch.title,
			Content: strings.TrimSpace(content),
		})
	}
	return result
}

func isPageNumber(s string) bool {
	return regexp.MustCompile(`^\d+$`).MatchString(s)
}

func extractChapterNum(title string) string {
	m := titleRegex.FindString(title)
	return m
}

func SplitByParagraphs(content string, maxChars int) []Segment {
	lines := strings.Split(content, "\n")
	if len(lines) < 5 {
		if strings.TrimSpace(content) != "" {
			return []Segment{{Content: strings.TrimSpace(content), Index: 1}}
		}
		return nil
	}

	sep := detectParagraphSep(lines)
	var paragraphs []string
	if sep == nil {
		paragraphs = splitByEmptyLine(content)
	} else {
		paragraphs = splitByIndent(lines, sep)
	}

	headerRe := regexp.MustCompile(`【第[一二三四五六七八九十百零\d]+[回章节部集话]`)
	numRe := regexp.MustCompile(`^\d+$`)

	var segs []Segment
	var buf strings.Builder
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		if len([]rune(p)) < 30 {
			continue
		}
		if headerRe.MatchString(p) {
			continue
		}
		if numRe.MatchString(p) {
			continue
		}

		if buf.Len() > 0 && len([]rune(buf.String()+p)) > maxChars {
			segs = append(segs, Segment{
				Content: strings.TrimSpace(buf.String()),
				Index:   len(segs) + 1,
			})
			buf.Reset()
		}

		if buf.Len() > 0 {
			buf.WriteString("\n\n")
		}
		buf.WriteString(p)
	}

	if buf.Len() > 0 {
		segs = append(segs, Segment{
			Content: strings.TrimSpace(buf.String()),
			Index:   len(segs) + 1,
		})
	}
	return segs
}

func splitByEmptyLine(content string) []string {
	raw := strings.Split(content, "\n\n")
	var result []string
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func splitByIndent(lines []string, isNewPara func(prevLine, currLine string) bool) []string {
	var paragraphs []string
	var buf strings.Builder
	prevLine := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isNewPara(prevLine, line) && buf.Len() > 0 {
			paragraphs = append(paragraphs, strings.TrimSpace(buf.String()))
			buf.Reset()
		}
		if trimmed != "" {
			buf.WriteString(trimmed + "\n")
		}
		prevLine = line
	}
	if buf.Len() > 0 {
		paragraphs = append(paragraphs, strings.TrimSpace(buf.String()))
	}
	return paragraphs
}
