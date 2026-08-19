package pbai

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	pdf "github.com/ledongthuc/pdf"
)

const (
	maxTextSize    = 300 * 1024 // 300 KB cap for extracted text
	maxPDFPages    = 20
	maxImageSize   = 8 * 1024 * 1024 // 8 MB cap for images (base64)
)

// Ingested holds the result of extracting content from an uploaded file.
type Ingested struct {
	Text    string // extracted text (for text/md/pdf)
	Mime    string // mime type
	IsImage bool   // true when the file should be sent as an image to a vision model
}

// Ingest extracts content from a decoded uploaded file. File content is always
// treated as untrusted data by the caller.
func Ingest(file *FileInput) (*Ingested, error) {
	raw, err := base64.StdEncoding.DecodeString(file.Data)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 file data: %w", err)
	}

	mime := strings.ToLower(file.Mime)
	if mime == "" {
		mime = sniffMime(file.Name)
	}

	switch {
	case strings.HasPrefix(mime, "image/"):
		if len(raw) > maxImageSize {
			return nil, fmt.Errorf("image too large (%d bytes, max %d)", len(raw), maxImageSize)
		}
		return &Ingested{Mime: mime, IsImage: true}, nil

	case strings.HasPrefix(mime, "text/"), strings.HasSuffix(file.Name, ".md"), strings.HasSuffix(file.Name, ".txt"), strings.HasSuffix(file.Name, ".csv"):
		text := decodeText(raw)
		if len(text) > maxTextSize {
			text = text[:maxTextSize]
		}
		if strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("no readable text content found in %q", file.Name)
		}
		return &Ingested{Text: text, Mime: mime}, nil

	case mime == "application/pdf" || strings.HasSuffix(file.Name, ".pdf"):
		text, err := extractPDF(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("failed to read PDF %q: %w", file.Name, err)
		}
		if len(text) > maxTextSize {
			text = text[:maxTextSize]
		}
		if strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("no readable text found in PDF %q (scanned/image PDFs are not supported)", file.Name)
		}
		return &Ingested{Text: text, Mime: "text/plain"}, nil

	default:
		// fall back to text decoding for unknown types
		text := decodeText(raw)
		if strings.TrimSpace(text) != "" {
			if len(text) > maxTextSize {
				text = text[:maxTextSize]
			}
			return &Ingested{Text: text, Mime: mime}, nil
		}
		return nil, fmt.Errorf("unsupported file type %q for %q", mime, file.Name)
	}
}

// sniffMime guesses a mime type from the file extension.
func sniffMime(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.HasSuffix(n, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(n, ".png"):
		return "image/png"
	case strings.HasSuffix(n, ".jpg"), strings.HasSuffix(n, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(n, ".gif"):
		return "image/gif"
	case strings.HasSuffix(n, ".webp"):
		return "image/webp"
	case strings.HasSuffix(n, ".md"):
		return "text/markdown"
	case strings.HasSuffix(n, ".csv"):
		return "text/csv"
	case strings.HasSuffix(n, ".txt"):
		return "text/plain"
	}
	return "application/octet-stream"
}

// decodeText decodes raw bytes as UTF-8, falling back to latin-1 if needed.
func decodeText(raw []byte) string {
	if utf8.Valid(raw) {
		return string(raw)
	}
	// best effort latin-1 → UTF-8
	buf := make([]rune, 0, len(raw))
	for _, b := range raw {
		buf = append(buf, rune(b))
	}
	return string(buf)
}

// extractPDF pulls text out of a PDF, capped at maxPDFPages.
func extractPDF(r io.ReaderAt) (string, error) {
	reader, readerErr := pdf.NewReader(r, 0)
	if readerErr != nil {
		return "", readerErr
	}
	total := reader.NumPage()
	if total > maxPDFPages {
		total = maxPDFPages
	}
	var out strings.Builder
	for i := 1; i <= total; i++ {
		p := reader.Page(i)
		if p.V.IsNull() {
			continue
		}
		text, err := p.GetPlainText(nil)
		if err != nil {
			continue
		}
		out.WriteString(text)
		out.WriteString("\n")
	}
	return out.String(), nil
}