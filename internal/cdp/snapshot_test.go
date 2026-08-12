package cdp

import (
	"reflect"
	"testing"
)

func TestExtractText(t *testing.T) {
	nodes := []AXNode{
		{Role: AXValue{Value: "StaticText"}, Name: AXValue{Value: "Alice"}},
		{Role: AXValue{Value: "InlineTextBox"}, Name: AXValue{Value: "Alice"}},
		{Role: AXValue{Value: "StaticText"}, Name: AXValue{Value: "  hello   world  "}},
		{Role: AXValue{Value: "button"}, Name: AXValue{Value: "Send"}},
		{Role: AXValue{Value: "generic"}, Name: AXValue{Value: "not visible text"}},
		{Ignored: true, Role: AXValue{Value: "StaticText"}, Name: AXValue{Value: "ignored"}},
		{Role: AXValue{Value: "StaticText"}, Name: AXValue{Value: "Alice"}},
	}

	got := ExtractText(nodes)
	want := []string{"Alice", "hello world", "Send", "Alice"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractText() = %#v, want %#v", got, want)
	}
}

func TestSplitVisibleText(t *testing.T) {
	got := splitVisibleText(" first \r\n\n second   line ")
	want := []string{"first", "second line"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitVisibleText() = %#v, want %#v", got, want)
	}
}

func TestDecodeDOMCapture(t *testing.T) {
	got, err := decodeDOMCapture(`{"mode":"messages","lines":["# Room","[2026-07-19 16:32] Alice: hello"]}`)
	if err != nil {
		t.Fatal(err)
	}
	want := domCaptureResult{
		Mode:  "messages",
		Lines: []string{"# Room", "[2026-07-19 16:32] Alice: hello"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decodeDOMCapture() = %#v, want %#v", got, want)
	}
}
