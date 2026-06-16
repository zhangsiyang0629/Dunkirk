package memory

import (
	"fmt"
	"strings"
)

type MemoryContext struct {
	Summaries         []string
	RecentGenerations []*GenerationRecord
}

func BuildContextPrompt(mc *MemoryContext) string {
	var parts []string

	if len(mc.Summaries) > 0 {
		lines := make([]string, len(mc.Summaries))
		for i, s := range mc.Summaries {
			lines[i] = fmt.Sprintf("阶段%d：%s", i+1, s)
		}
		parts = append(parts, fmt.Sprintf("【对话摘要】\n%s", strings.Join(lines, "\n")))
	}

	if len(mc.RecentGenerations) > 0 {
		var lines []string
		for _, g := range mc.RecentGenerations {
			var chLines []string
			for _, ch := range g.Chapters {
				var segs []string
				for _, seg := range ch.Segments {
					switch seg.Status {
					case SegmentStatusApproved:
						segs = append(segs, fmt.Sprintf("第%d集✓", seg.SegmentIdx+1))
					case SegmentStatusRejected:
						segs = append(segs, fmt.Sprintf("第%d集✗", seg.SegmentIdx+1))
					case SegmentStatusRejectedButSaved:
						segs = append(segs, fmt.Sprintf("第%d集✗(脚本已保留)", seg.SegmentIdx+1))
					}
				}
				chLines = append(chLines, fmt.Sprintf("  %s：%s", ch.Topic, strings.Join(segs, " ")))
			}
			lines = append(lines, fmt.Sprintf("《%s》\n%s", g.BookName, strings.Join(chLines, "\n")))
		}
		parts = append(parts, fmt.Sprintf("【最近生成记录】\n%s", strings.Join(lines, "\n")))
	}
	return strings.Join(parts, "\n\n")
}
