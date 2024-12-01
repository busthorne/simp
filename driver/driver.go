package driver

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/busthorne/simp"
)

var Drivers = []string{"openai", "anthropic", "gemini", "dify", "vertex"}

func ListString() string {
	return strings.Join(Drivers, ", ")
}

func url2image64(ctx context.Context, url string) (mime string, b []byte, err error) {
	resp, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	mime = resp.Header.Get("Content-Type")
	switch mime {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
	default:
		err = simp.ErrUnsupportedMime
		return
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}
	b = data
	return
}
