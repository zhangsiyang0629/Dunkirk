package docproc

import (
	"context"
	"os"
	"testing"
)

func TestSplitter(t *testing.T) {
	ctx := context.Background()
	filePath := "/Users/zhangsiyang/Downloads/三国演义.pdf"
	doc, err := LoadDocument(ctx, filePath)
	if err != nil {
		t.Fatal(err)
	}

	chapters := SplitByChapters(doc.Content)
	t.Logf("chapter len:%d", len(chapters))
	for _, ch := range chapters {
		t.Logf("chapter %d: %s (%d chars)", ch.Index, ch.Title, len(ch.Content))
	}
}

func TestConvertPDFWithMarker(t *testing.T) {
	ctx := context.Background()
	filePath := "/Users/zhangsiyang/Downloads/三国演义.pdf"
	doc, err := convertPDFWithMarker(ctx, filePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("converted document: %s, content length: %d", doc.ID, len(doc.Content))
	t.Logf("content preview: %s", doc.Content[:1000])

	res := CleanMarkitdownOutput(doc.Content)
	outputFile, err := os.Create("./三国演义_output.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer outputFile.Close()

	_, err = outputFile.WriteString(res)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("成功将文档写入: %s", outputFile.Name())
}
