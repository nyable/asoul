package agent

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/tailscale/hujson"
)

// ConfigDoc is a comment- and order-preserving view over a JSONC/JSON
// configuration file. It applies map-based merges back onto the original
// syntax tree so that untouched fields keep their comments, blank lines and
// key order.
type ConfigDoc struct {
	Path string

	root hujson.Value
	raw  []byte
	perm os.FileMode
}

// LoadConfigDoc reads and parses a JSONC/JSON file into a ConfigDoc.
func LoadConfigDoc(path string) (*ConfigDoc, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	root, err := hujson.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return &ConfigDoc{Path: path, root: root, raw: raw, perm: info.Mode().Perm()}, nil
}

// Raw returns the original file bytes as read from disk.
func (d *ConfigDoc) Raw() []byte { return d.raw }

// RootMap decodes the original document into a plain map for computation.
// It operates on a copy because hujson.Standardize mutates its input buffer,
// which the parsed tree aliases.
func (d *ConfigDoc) RootMap() (map[string]any, error) {
	std, err := hujson.Standardize(append([]byte(nil), d.raw...))
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(std, &root); err != nil {
		return nil, err
	}
	return root, nil
}

// Bytes serializes the current syntax tree, preserving comments and order.
func (d *ConfigDoc) Bytes() []byte { return d.root.Pack() }

// Root returns the root value for read-only inspection.
func (d *ConfigDoc) Root() *hujson.Value { return &d.root }

// Find navigates to a nested object member by key path.
func (d *ConfigDoc) Find(keys ...string) *hujson.Value {
	cur := &d.root
	for _, key := range keys {
		cur = objectMember(cur, key)
		if cur == nil {
			return nil
		}
	}
	return cur
}

// Apply reconciles merged into the object located at keys. Existing members are
// updated in place (keeping their comments/whitespace), members absent from
// merged are removed, and new members are appended with formatting matching the
// surrounding object.
func (d *ConfigDoc) Apply(merged map[string]any, keys ...string) error {
	node := d.Find(keys...)
	if node == nil {
		return fmt.Errorf("config path %s not found", strings.Join(keys, "."))
	}
	if _, ok := node.Value.(*hujson.Object); !ok {
		replacement := newHuValue(merged, d.nodeClosingIndent(keys))
		node.Value = replacement.Value
		return nil
	}
	return syncObject(node, merged, "")
}

// HasDiff reports whether the current tree differs from the original raw bytes.
func (d *ConfigDoc) HasDiff() bool {
	return string(d.raw) != string(d.Bytes())
}

// Perm returns the file permission bits read at load time.
func (d *ConfigDoc) Perm() os.FileMode { return d.perm }

// nodeClosingIndent returns the key indentation of the member addressed by
// keys, or "" when it cannot be determined.
func (d *ConfigDoc) nodeClosingIndent(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	parent := &d.root
	for _, k := range keys[:len(keys)-1] {
		parent = objectMember(parent, k)
		if parent == nil {
			return ""
		}
	}
	obj, ok := parent.Value.(*hujson.Object)
	if !ok {
		return ""
	}
	target := keys[len(keys)-1]
	for i := range obj.Members {
		if literalString(obj.Members[i].Name.Value) == target {
			return indentBefore(&obj.Members[i].Name)
		}
	}
	return ""
}

func objectMember(v *hujson.Value, name string) *hujson.Value {
	obj, ok := v.Value.(*hujson.Object)
	if !ok {
		return nil
	}
	for i := range obj.Members {
		if literalString(obj.Members[i].Name.Value) == name {
			return &obj.Members[i].Value
		}
	}
	return nil
}

func literalString(v hujson.ValueTrimmed) string {
	if lit, ok := v.(hujson.Literal); ok {
		return lit.String()
	}
	return ""
}

// syncObject reconciles merged into the given object AST node.
func syncObject(v *hujson.Value, merged map[string]any, fallbackClosingIndent string) error {
	obj, ok := v.Value.(*hujson.Object)
	if !ok {
		return fmt.Errorf("target is not a JSON object")
	}

	memberIndent, closingIndent, multiline := detectIndent(obj)
	if len(obj.Members) == 0 && fallbackClosingIndent != "" {
		closingIndent = fallbackClosingIndent
		memberIndent = closingIndent + "  "
		multiline = true
	}

	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		newVal := merged[key]
		idx := memberIndex(obj, key)
		if idx < 0 {
			nameVal := hujson.Value{Value: hujson.String(key)}
			if multiline {
				nameVal.BeforeExtra = []byte("\n" + memberIndent)
			} else {
				nameVal.BeforeExtra = []byte(" ")
			}
			valNode := newHuValue(newVal, memberIndent)
			valNode.BeforeExtra = []byte(" ")
			obj.Members = append(obj.Members, hujson.ObjectMember{Name: nameVal, Value: valNode})
			if multiline && len(obj.AfterExtra) == 0 {
				obj.AfterExtra = []byte("\n" + closingIndent)
			}
			continue
		}

		member := &obj.Members[idx]
		oldVal := nodeToAny(&member.Value)
		if reflect.DeepEqual(oldVal, newVal) {
			continue
		}
		memberClosing := indentBefore(&member.Name)
		if memberClosing == "" {
			memberClosing = closingIndent
		}
		if childMerged, ok := newVal.(map[string]any); ok {
			if _, isObject := member.Value.Value.(*hujson.Object); isObject {
				if err := syncObject(&member.Value, childMerged, memberClosing); err != nil {
					return err
				}
				continue
			}
		}
		replacement := newHuValue(newVal, memberClosing)
		member.Value.Value = replacement.Value
	}

	var kept []hujson.ObjectMember
	for _, m := range obj.Members {
		if _, ok := merged[literalString(m.Name.Value)]; !ok {
			continue
		}
		kept = append(kept, m)
	}
	obj.Members = kept
	return nil
}

func memberIndex(obj *hujson.Object, name string) int {
	for i := range obj.Members {
		if literalString(obj.Members[i].Name.Value) == name {
			return i
		}
	}
	return -1
}

// detectIndent derives the member indentation, closing-brace indentation and
// whether the object is rendered across multiple lines.
func detectIndent(obj *hujson.Object) (memberIndent, closingIndent string, multiline bool) {
	if len(obj.Members) > 0 {
		memberIndent = indentBefore(&obj.Members[0].Name)
	}
	afterIndent := trailingIndent(obj.AfterExtra)
	if memberIndent != "" || afterIndent != "" || strings.Contains(string(obj.AfterExtra), "\n") {
		multiline = true
	}
	closingIndent = afterIndent
	if closingIndent == "" && memberIndent != "" {
		closingIndent = trimOneLevel(memberIndent)
	}
	return memberIndent, closingIndent, multiline
}

func indentBefore(v *hujson.Value) string {
	return trailingIndent(v.BeforeExtra)
}

// trailingIndent returns the run of spaces/tabs after the last newline.
func trailingIndent(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	s := string(b)
	idx := strings.LastIndexByte(s, '\n')
	if idx < 0 {
		return ""
	}
	seg := s[idx+1:]
	if strings.Trim(seg, " \t") != "" {
		return ""
	}
	return seg
}

func trimOneLevel(indent string) string {
	if strings.HasPrefix(indent, "  ") {
		return indent[2:]
	}
	if strings.HasPrefix(indent, "\t") {
		return indent[1:]
	}
	if strings.HasPrefix(indent, " ") {
		return indent[1:]
	}
	return indent
}

func nodeToAny(v *hujson.Value) any {
	std, err := hujson.Standardize(v.Pack())
	if err != nil {
		return nil
	}
	var out any
	if err := json.Unmarshal(std, &out); err != nil {
		return nil
	}
	return out
}

// newHuValue builds a formatted syntax node from a decoded JSON value.
// closingIndent is the indentation that should precede the closing brace/bracket.
func newHuValue(value any, closingIndent string) hujson.Value {
	switch t := value.(type) {
	case nil:
		return hujson.Value{Value: hujson.Literal("null")}
	case bool:
		return hujson.Value{Value: hujson.Bool(t)}
	case string:
		return hujson.Value{Value: hujson.String(t)}
	case float64:
		return hujson.Value{Value: numberLiteral(t)}
	case float32:
		return hujson.Value{Value: numberLiteral(float64(t))}
	case int:
		return hujson.Value{Value: hujson.Int(int64(t))}
	case int64:
		return hujson.Value{Value: hujson.Int(t)}
	case map[string]any:
		return newHuObject(t, closingIndent)
	case []any:
		return newHuArray(t, closingIndent)
	default:
		b, err := json.Marshal(value)
		if err != nil {
			return hujson.Value{Value: hujson.Literal("null")}
		}
		return hujson.Value{Value: hujson.Literal(b)}
	}
}

func numberLiteral(f float64) hujson.Literal {
	if f == math.Trunc(f) && !math.IsInf(f, 0) && f >= math.MinInt64 && f <= math.MaxInt64 {
		return hujson.Int(int64(f))
	}
	return hujson.Float(f)
}

func newHuObject(m map[string]any, closingIndent string) hujson.Value {
	if len(m) == 0 {
		return hujson.Value{Value: &hujson.Object{}}
	}
	obj := &hujson.Object{}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	memberIndent := closingIndent + "  "
	for _, k := range keys {
		name := hujson.Value{BeforeExtra: []byte("\n" + memberIndent), Value: hujson.String(k)}
		val := newHuValue(m[k], memberIndent)
		val.BeforeExtra = []byte(" ")
		obj.Members = append(obj.Members, hujson.ObjectMember{Name: name, Value: val})
	}
	obj.AfterExtra = []byte("\n" + closingIndent)
	return hujson.Value{Value: obj}
}

func newHuArray(items []any, closingIndent string) hujson.Value {
	if len(items) == 0 {
		return hujson.Value{Value: &hujson.Array{}}
	}
	arr := &hujson.Array{}
	elemIndent := closingIndent + "  "
	for _, item := range items {
		val := newHuValue(item, elemIndent)
		val.BeforeExtra = []byte("\n" + elemIndent)
		arr.Elements = append(arr.Elements, val)
	}
	arr.AfterExtra = []byte("\n" + closingIndent)
	return hujson.Value{Value: arr}
}
