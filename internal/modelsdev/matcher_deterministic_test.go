package modelsdev

import "testing"

func TestMatcherDuplicateIDsUseStableProviderOrder(t *testing.T) {
	catalog := Catalog{
		"z-provider": {
			ID: "z-provider",
			Models: map[string]ModelData{
				"shared": {ID: "z-provider/shared", Name: "from z"},
			},
		},
		"a-provider": {
			ID: "a-provider",
			Models: map[string]ModelData{
				"shared": {ID: "a-provider/shared", Name: "from a"},
			},
		},
	}

	for i := 0; i < 100; i++ {
		matched, ok := NewMatcher(catalog).FindModel("shared")
		if !ok {
			t.Fatal("expected duplicate model to be matched")
		}
		if matched.ID != "a-provider/shared" {
			t.Fatalf("iteration %d selected unstable provider %q", i, matched.ID)
		}
	}
}
