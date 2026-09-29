package jsonc

import (
	"encoding/json"
	"testing"
)

func TestStripCommentsAndTrailingCommas(t *testing.T) {
	input := []byte(`{
		// Single line comment
		"model": "gpt-5", /* Inline comment */
		"options": {
			"url": "https://example.com//not-a-comment", // comment with url
			"val": 123, // trailing comma
		},
		/* Multi-line
		   comment here
		*/
		"list": [
			1,
			2,
			3, // trailing comma in list
		],
	}`)

	clean := StripCommentsAndTrailingCommas(input)

	var obj map[string]any
	if err := json.Unmarshal(clean, &obj); err != nil {
		t.Fatalf("failed to unmarshal stripped jsonc: %v\nOutput was:\n%s", err, string(clean))
	}

	if obj["model"] != "gpt-5" {
		t.Errorf("expected model gpt-5, got %v", obj["model"])
	}

	options, ok := obj["options"].(map[string]any)
	if !ok {
		t.Fatalf("options not a map: %v", obj["options"])
	}
	if options["url"] != "https://example.com//not-a-comment" {
		t.Errorf("expected url to retain // inside string, got %v", options["url"])
	}

	list, ok := obj["list"].([]any)
	if !ok || len(list) != 3 {
		t.Errorf("expected list length 3, got %v", list)
	}
}

func TestUnmarshal(t *testing.T) {
	input := []byte(`{
		"agent": "opencode", // test
	}`)
	var res struct {
		Agent string `json:"agent"`
	}
	if err := Unmarshal(input, &res); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if res.Agent != "opencode" {
		t.Fatalf("expected agent opencode, got %s", res.Agent)
	}
}
