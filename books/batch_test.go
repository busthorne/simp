package books

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"io"
	"testing"

	"github.com/google/go-cmp/cmp"
	openai "github.com/sashabaranov/go-openai"
)

//go:embed testdata/input.jsonl
var inputJSONL []byte

//go:embed testdata/output.jsonl
var outputJSONL []byte

func TestBatchInput(t *testing.T) {
	r := json.NewDecoder(bytes.NewReader(inputJSONL))

	var inputs []openai.BatchInput
	for {
		var input openai.BatchInput
		switch err := r.Decode(&input); err {
		case nil:
			inputs = append(inputs, input)
		case io.EOF:
			goto eof
		default:
			t.Fatal(err)
		}
	}
eof:
	var got bytes.Buffer
	w := json.NewEncoder(&got)
	for _, input := range inputs {
		w.Encode(input)
	}
	if diff := cmp.Diff(inputJSONL, got.Bytes()); diff != "" {
		t.Fatal(diff)
	}
}

func TestBatchOutput(t *testing.T) {
	r := json.NewDecoder(bytes.NewReader(outputJSONL))

	var outputs []openai.BatchOutput
	for {
		var output openai.BatchOutput
		switch err := r.Decode(&output); err {
		case nil:
			outputs = append(outputs, output)
		case io.EOF:
			goto eof
		default:
			t.Fatal(err)
		}
	}
eof:
	var got bytes.Buffer
	w := json.NewEncoder(&got)
	for _, output := range outputs {
		if err := w.Encode(output); err != nil {
			t.Fatal(err)
		}
	}
	if diff := cmp.Diff(outputJSONL, got.Bytes()); diff != "" {
		t.Fatal(diff)
	}
}
