package agentmodel

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DeepMerge recursively merges overlay into base, returning a new map.
// If a key exists in both base and overlay:
// - If both values are map[string]any, they are recursively merged.
// - Otherwise, the overlay value overwrites the base value.
func DeepMerge(base, overlay map[string]any) map[string]any {
	result := make(map[string]any)

	// Copy all entries from base
	for k, v := range base {
		if subMap, ok := v.(map[string]any); ok {
			result[k] = cloneMap(subMap)
		} else {
			result[k] = v
		}
	}

	// Merge entries from overlay
	for k, v := range overlay {
		if baseVal, exists := result[k]; exists {
			baseMap, baseIsMap := baseVal.(map[string]any)
			overlayMap, overlayIsMap := v.(map[string]any)

			if baseIsMap && overlayIsMap {
				result[k] = DeepMerge(baseMap, overlayMap)
				continue
			}
		}
		if subMap, ok := v.(map[string]any); ok {
			result[k] = cloneMap(subMap)
		} else {
			result[k] = v
		}
	}

	return result
}

// FillMissing recursively merges source into base only for keys that do NOT exist in base.
// If a key exists in both and both are maps, FillMissing is applied recursively.
func FillMissing(base, source map[string]any) map[string]any {
	result := make(map[string]any)

	for k, v := range base {
		if subMap, ok := v.(map[string]any); ok {
			result[k] = cloneMap(subMap)
		} else {
			result[k] = v
		}
	}

	for k, v := range source {
		if baseVal, exists := result[k]; exists {
			baseMap, baseIsMap := baseVal.(map[string]any)
			srcMap, srcIsMap := v.(map[string]any)
			if baseIsMap && srcIsMap {
				result[k] = FillMissing(baseMap, srcMap)
			}
			continue
		}

		if subMap, ok := v.(map[string]any); ok {
			result[k] = cloneMap(subMap)
		} else {
			result[k] = v
		}
	}

	return result
}
func cloneMap(m map[string]any) map[string]any {
	res := make(map[string]any, len(m))
	for k, v := range m {
		if sub, ok := v.(map[string]any); ok {
			res[k] = cloneMap(sub)
		} else {
			res[k] = v
		}
	}
	return res
}

// MergeUnion recursively merges overlay into base. Unlike DeepMerge, arrays are
// combined as an order-preserving union with duplicate elements removed.
func MergeUnion(base, overlay map[string]any) map[string]any {
	result := make(map[string]any, len(base)+len(overlay))
	for k, v := range base {
		if sub, ok := v.(map[string]any); ok {
			result[k] = cloneMap(sub)
		} else {
			result[k] = v
		}
	}
	for k, v := range overlay {
		baseVal, exists := result[k]
		if !exists {
			result[k] = cloneAny(v)
			continue
		}
		baseMap, baseIsMap := baseVal.(map[string]any)
		overlayMap, overlayIsMap := v.(map[string]any)
		if baseIsMap && overlayIsMap {
			result[k] = MergeUnion(baseMap, overlayMap)
			continue
		}
		baseArr, baseIsArr := baseVal.([]any)
		overlayArr, overlayIsArr := v.([]any)
		if baseIsArr && overlayIsArr {
			result[k] = unionSlice(baseArr, overlayArr)
			continue
		}
		result[k] = cloneAny(v)
	}
	return result
}

func cloneAny(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return cloneMap(t)
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = cloneAny(e)
		}
		return out
	default:
		return v
	}
}

func unionSlice(base, overlay []any) []any {
	out := make([]any, 0, len(base)+len(overlay))
	seen := make(map[string]bool, len(base)+len(overlay))
	appendUnique := func(items []any) {
		for _, item := range items {
			key := canonicalKey(item)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, cloneAny(item))
		}
	}
	appendUnique(base)
	appendUnique(overlay)
	return out
}

func canonicalKey(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// RemovePath deletes the value at a dot-separated path from m.
// Missing paths are ignored.
func RemovePath(m map[string]any, path string) {
	parts := strings.Split(path, ".")
	var cur any = m
	for i, part := range parts {
		obj, ok := cur.(map[string]any)
		if !ok {
			return
		}
		if i == len(parts)-1 {
			delete(obj, part)
			return
		}
		cur, ok = obj[part]
		if !ok {
			return
		}
	}
}
