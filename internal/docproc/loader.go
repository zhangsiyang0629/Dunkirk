package docproc

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	fileLoader "github.com/cloudwego/eino-ext/components/document/loader/file"
	"github.com/cloudwego/eino-ext/components/document/parser/docx"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/document/parser"
	"github.com/cloudwego/eino/schema"
)

func LoadDocument(ctx context.Context, filePath string) (*schema.Document, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".pdf":
		doc, err := convertPDFWithMarker(ctx, filePath)
		if err == nil {
			return doc, nil
		}
		log.Printf("marker failed, fallback to pdftotext: %v", err)
		out, err := exec.CommandContext(ctx, "pdftotext", "-layout", "-nopgbrk", filePath, "-").Output()
		if err != nil {
			return nil, fmt.Errorf("pdftotext: %w", err)
		}
		return &schema.Document{
			ID:      filepath.Base(filePath),
			Content: string(out),
		}, nil
	}
	var p parser.Parser
	switch ext {
	case ".docx":
		dp, err := docx.NewDocxParser(ctx, &docx.Config{})
		if err != nil {
			return nil, fmt.Errorf("new docx parser: %w", err)
		}
		p = dp
	default:
		// .md / .txt → TextParser (默认)
	}
	loader, err := fileLoader.NewFileLoader(ctx, &fileLoader.FileLoaderConfig{
		UseNameAsID: true,
		Parser:      p,
	})
	if err != nil {
		return nil, fmt.Errorf("new file loader: %w", err)
	}
	docs, err := loader.Load(ctx, document.Source{URI: filePath})
	if err != nil {
		return nil, fmt.Errorf("load: %w", err)
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("no content loaded")
	}
	return docs[0], nil
}

// func hasCommand(name string) bool {
// 	_, err := exec.LookPath(name)
// 	return err == nil
// }

func convertPDFWithMarker(ctx context.Context, filePath string) (*schema.Document, error) {
	pythonPath, err := exec.LookPath("python3")
	if err != nil {
		return nil, fmt.Errorf("python3 not found: %w", err)
	}
	script, err := filepath.Abs(filepath.Join("internal", "docproc", "convert_pdf.py"))
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, pythonPath, script, filePath)
	cmd.Env = os.Environ() // 继承当前进程的完整环境变量
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("marker convert: %w\nstderr: %s", err, cmd.Stderr)
	}
	return &schema.Document{
		ID:      filepath.Base(filePath),
		Content: string(out),
	}, nil
}
