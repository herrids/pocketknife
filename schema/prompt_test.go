package schema

import (
	"reflect"
	"testing"
)

func TestScanPrompt(t *testing.T) {
	cases := []struct {
		name          string
		prompt        string
		wantParams    []string
		wantMalformed []int
	}{
		{"static prompt, no placeholders", "Summarize the day.", nil, nil},
		{"single param", "Summarize: {{text}}", []string{"text"}, nil},
		{"whitespace inside braces", "{{ text }} and {{\ttone }}", []string{"text", "tone"}, nil},
		{"repeat dedupes, first-appearance order", "{{b}} {{a}} {{b}}", []string{"b", "a"}, nil},
		{"adjacent placeholders", "{{a}}{{b}}", []string{"a", "b"}, nil},
		{"uppercase not a param", "{{Text}}", nil, []int{0}},
		{"empty braces malformed", "{{}}", nil, []int{0}},
		{"unclosed malformed", "hello {{name", nil, []int{6}},
		{"triple brace reports once", "{{{a}}", []string{"a"}, []int{0}},
		{"digit-leading name malformed", "{{1st}}", nil, []int{0}},
		{"mixed valid and malformed", "{{a}} then {{b c}}", []string{"a"}, []int{11}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params, malformed := ScanPrompt(tc.prompt)
			if !reflect.DeepEqual(params, tc.wantParams) {
				t.Errorf("params = %v, want %v", params, tc.wantParams)
			}
			if !reflect.DeepEqual(malformed, tc.wantMalformed) {
				t.Errorf("malformed = %v, want %v", malformed, tc.wantMalformed)
			}
		})
	}
}

func TestPromptParamsAndRender(t *testing.T) {
	fn := &Function{
		ID:     "fn_summarize",
		Name:   "summarize",
		Prompt: "Summarize {{text}} in a {{tone}} tone. Text again: {{text}}",
	}
	if !fn.IsPrompt() {
		t.Fatal("IsPrompt() = false for a prompt function")
	}
	if got, want := fn.PromptParams(), []string{"text", "tone"}; !reflect.DeepEqual(got, want) {
		t.Errorf("PromptParams() = %v, want %v", got, want)
	}

	out := RenderPrompt(fn.Prompt, map[string]string{"text": "the notes", "tone": "cheerful"})
	want := "Summarize the notes in a cheerful tone. Text again: the notes"
	if out != want {
		t.Errorf("RenderPrompt = %q, want %q", out, want)
	}

	wasm := &Function{ID: "fn_x", Name: "x", Entry: "functions/x.wasm"}
	if wasm.IsPrompt() {
		t.Error("IsPrompt() = true for a wasm function")
	}
	if params := wasm.PromptParams(); params != nil {
		t.Errorf("PromptParams() = %v for a wasm function, want nil", params)
	}
}
