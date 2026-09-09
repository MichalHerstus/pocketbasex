package pbai

import (
	"reflect"
	"testing"
)

func TestExtractFormFillBare(t *testing.T) {
	fill, clean := extractFormFill(`{"formFill": {"name": "Widget", "price": 9.5}}`)
	want := map[string]any{"name": "Widget", "price": 9.5}
	if !reflect.DeepEqual(fill, want) {
		t.Fatalf("unexpected fill: %#v", fill)
	}
	if clean != "" {
		t.Fatalf("expected empty remaining text, got %q", clean)
	}
}

func TestExtractFormFillInProse(t *testing.T) {
	fill, clean := extractFormFill(`Done. {"formFill": {"name": "Widget"}} Here you go.`)
	want := map[string]any{"name": "Widget"}
	if !reflect.DeepEqual(fill, want) {
		t.Fatalf("unexpected fill: %#v", fill)
	}
	if clean != "Done. Here you go." {
		t.Fatalf("unexpected cleaned text: %q", clean)
	}
}

func TestExtractFormFillFenced(t *testing.T) {
	fill, clean := extractFormFill("```json\n{\"formFill\": {\"qty\": 3}}\n```")
	want := map[string]any{"qty": float64(3)}
	if !reflect.DeepEqual(fill, want) {
		t.Fatalf("unexpected fill: %#v", fill)
	}
	if clean != "" {
		t.Fatalf("expected empty remaining text, got %q", clean)
	}
}

func TestExtractFormFillNone(t *testing.T) {
	fill, clean := extractFormFill("Just a normal reply, no form data.")
	if fill != nil {
		t.Fatalf("expected nil fill, got %#v", fill)
	}
	if clean != "Just a normal reply, no form data." {
		t.Fatalf("unexpected cleaned text: %q", clean)
	}
}

func TestExtractFormFillFalsePositiveKey(t *testing.T) {
	// text mentioning "formFill" without a parseable object must not break
	fill, clean := extractFormFill("The formFill feature is off.")
	if fill != nil || clean != "The formFill feature is off." {
		t.Fatalf("unexpected result: %#v %q", fill, clean)
	}
}

func TestExtractFormFillNestedAndEscapedStrings(t *testing.T) {
	fill, clean := extractFormFill(`{"formFill": {"note": "say \"hi\" {x}", "n": 1}}`)
	want := map[string]any{"note": "say \"hi\" {x}", "n": float64(1)}
	if !reflect.DeepEqual(fill, want) {
		t.Fatalf("unexpected fill: %#v", fill)
	}
	if clean != "" {
		t.Fatalf("expected empty remaining text, got %q", clean)
	}
}

func TestRunStreamExtractsFormFillLastTurnOnly(t *testing.T) {
	// The literal assistant text with an embedded formFill object must not be
	// exposed as FinalText when the extraction runs on it.
	fill, clean := extractFormFill(`{"formFill": {"name": "Widget"}}`)
	if len(fill) != 1 || clean != "" {
		t.Fatalf("unexpected extraction: %#v %q", fill, clean)
	}
}