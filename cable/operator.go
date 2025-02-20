package cable

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
)

type OperatorType string

const (
	OperatorAlternate OperatorType = "#"
	OperatorAttach    OperatorType = "+"
)

var (
	ErrAttachmentMissing = errors.New("cable: attachment has no path or url")
	ErrUnknownOperator   = errors.New("cable: unknown operator")
)

func (op OperatorType) Ord() int {
	switch op {
	case OperatorAlternate:
		return 1
	case OperatorAttach:
		return 2
	default:
		return 0
	}
}

type Operator interface {
	Op() OperatorType
	String() string
}

func ParseOperator(s string) (Operator, error) {
	switch opt, s := OperatorType(s[0]), s[1:]; opt {
	case OperatorAlternate:
		return NewAlternate(s)
	case OperatorAttach:
		return NewAttachment(s)
	default:
		return nil, fmt.Errorf("%w %q", ErrUnknownOperator, opt)
	}
}

type Operators []Operator

func (ops Operators) Len() int {
	return len(ops)
}

func (ops Operators) Less(i, j int) bool {
	return ops[i].Op().Ord() < ops[j].Op().Ord()
}

func (ops Operators) Swap(i, j int) {
	ops[i], ops[j] = ops[j], ops[i]
}

type Alternate struct {
	Index int
}

func NewAlternate(s string) (*Alternate, error) {
	index, err := strconv.Atoi(s)
	if err != nil {
		return nil, fmt.Errorf("cable: alternate index %q is not a number: %w", s, err)
	}
	return &Alternate{Index: index}, nil
}

func (a *Alternate) Op() OperatorType { return OperatorAlternate }

func (a *Alternate) String() string { return string(OperatorAlternate) + strconv.Itoa(a.Index) }

type Attachment struct {
	Path string
	Mime string
	URL  *url.URL

	Client *http.Client
}

func NewAttachment(path string) (*Attachment, error) {
	a := &Attachment{}
	if _, err := os.Stat(path); err != nil {
		u, err := url.Parse(path)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrAttachmentMissing, err)
		}
		a.URL = u
	} else {
		a.Path = path
	}
	return a, nil
}

func (a *Attachment) Op() OperatorType { return OperatorAttach }

func (a *Attachment) String() (s string) {
	s = string(OperatorAttach)
	switch {
	case a.Path != "":
		s += a.Path
	case a.URL != nil:
		s += a.URL.String()
	}
	return
}

func MimeType(fpath string) string {
	switch ext := strings.Trim(path.Ext(fpath), "."); ext {
	case "":
		return ""
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "svg":
		return "image/svg+xml"
	case "pdf":
		return "application/pdf"
	case "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "m4a":
		return "audio/mp4"
	case "ogg":
		return "audio/ogg"
	case "mp4", "m4v":
		return "video/mp4"
	case "webm":
		return "video/webm"
	case "html":
		return "text/html"
	case "md":
		return "text/markdown"
	default:
		return "text/plain"
	}
}

func (a *Attachment) Open(ctx context.Context) (io.ReadCloser, error) {
	switch {
	case a.Path != "":
		f, err := os.Open(a.Path)
		if err != nil {
			return nil, fmt.Errorf("cable: could not open attachment: %w", err)
		}
		a.Mime = MimeType(a.Path)
		return f, nil
	case a.URL != nil:
		client := a.Client
		if client == nil {
			client = http.DefaultClient
		}
		req, err := http.NewRequestWithContext(ctx, "GET", a.URL.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("cable: could not create attachment request: %w", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("cable: could not get attachment: %w", err)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "" {
			a.Mime = ct
		} else {
			a.Mime = MimeType(a.URL.Path)
		}
		return resp.Body, nil
	default:
		return nil, ErrAttachmentMissing
	}
}
