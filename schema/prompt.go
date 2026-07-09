package schema

import "regexp"

// placeholderPattern matches one well-formed {{param}} placeholder in a prompt
// template. Param names follow the manifest's machineName rule. This is the
// single definition of the template syntax: the validator, the runtime
// renderer and the client generator all derive from it (the agent's TypeScript
// seams keep a hand-synced mirror).
var placeholderPattern = regexp.MustCompile(`\{\{\s*([a-z][a-z0-9_]*)\s*\}\}`)

// PromptParams returns the prompt template's placeholder names in order of
// first appearance, deduplicated. The order is deterministic and drives the
// generated client's params interface. It returns nil for a wasm function.
func (f *Function) PromptParams() []string {
	params, _ := ScanPrompt(f.Prompt)
	return params
}

// ScanPrompt scans a prompt template and returns its well-formed placeholder
// names (first-appearance order, deduplicated) plus the byte offset of every
// "{{" that does not begin a well-formed placeholder. There is no escaping
// mechanism: a literal "{{" that isn't a placeholder is a validation error,
// keeping the template contract closed and unambiguous.
func ScanPrompt(prompt string) (params []string, malformed []int) {
	matches := placeholderPattern.FindAllStringSubmatchIndex(prompt, -1)
	starts := make(map[int]bool, len(matches))
	seen := make(map[string]bool, len(matches))
	for _, m := range matches {
		starts[m[0]] = true
		name := prompt[m[2]:m[3]]
		if !seen[name] {
			seen[name] = true
			params = append(params, name)
		}
	}
	for i := 0; i+1 < len(prompt); i++ {
		if prompt[i] != '{' || prompt[i+1] != '{' {
			continue
		}
		if starts[i] {
			// A well-formed placeholder's interior is only \s and machineName
			// characters, so no other "{{" can occur inside it — advancing one
			// byte at a time is safe.
			continue
		}
		malformed = append(malformed, i)
		i++ // consume both braces so "{{{" reports once, not twice
	}
	return params, malformed
}

// RenderPrompt substitutes values into the prompt template. Every placeholder
// must have a value present; the caller (funcrun) validates that against
// PromptParams before rendering, so a missing key here simply substitutes the
// empty string rather than erroring.
func RenderPrompt(prompt string, values map[string]string) string {
	return placeholderPattern.ReplaceAllStringFunc(prompt, func(m string) string {
		sub := placeholderPattern.FindStringSubmatch(m)
		return values[sub[1]]
	})
}
