package jsonc

import (
	"bytes"
	"encoding/json"
	"unicode"
)

// StripCommentsAndTrailingCommas removes C-style comments (// and /* */)
// and trailing commas before closing braces/brackets from JSONC input,
// producing standard JSON compatible with encoding/json.
func StripCommentsAndTrailingCommas(input []byte) []byte {
	var buf bytes.Buffer
	buf.Grow(len(input))

	inString := false
	escaped := false
	length := len(input)

	// Step 1: Strip single-line and multi-line comments
	for i := 0; i < length; i++ {
		b := input[i]

		if inString {
			buf.WriteByte(b)
			if escaped {
				escaped = false
			} else if b == '\\' {
				escaped = true
			} else if b == '"' {
				inString = false
			}
			continue
		}

		// Check for comment starts when not in string
		if b == '"' {
			inString = true
			buf.WriteByte(b)
			continue
		}

		// Single-line comment
		if b == '/' && i+1 < length && input[i+1] == '/' {
			i += 2
			for i < length && input[i] != '\n' && input[i] != '\r' {
				i++
			}
			if i < length {
				buf.WriteByte(input[i])
			}
			continue
		}

		// Multi-line comment
		if b == '/' && i+1 < length && input[i+1] == '*' {
			i += 2
			for i+1 < length && !(input[i] == '*' && input[i+1] == '/') {
				if input[i] == '\n' {
					buf.WriteByte('\n') // preserve line numbers
				}
				i++
			}
			i++ // skip '/' of closing */
			continue
		}

		buf.WriteByte(b)
	}

	strippedComments := buf.Bytes()

	// Step 2: Strip trailing commas before '}' or ']'
	var out bytes.Buffer
	out.Grow(len(strippedComments))

	inString = false
	escaped = false
	lastCommaIdx := -1
	scLen := len(strippedComments)

	for i := 0; i < scLen; i++ {
		b := strippedComments[i]

		if inString {
			out.WriteByte(b)
			if escaped {
				escaped = false
			} else if b == '\\' {
				escaped = true
			} else if b == '"' {
				inString = false
			}
			continue
		}

		if b == '"' {
			inString = true
			lastCommaIdx = -1
			out.WriteByte(b)
			continue
		}

		if b == ',' {
			lastCommaIdx = out.Len()
			out.WriteByte(b)
			continue
		}

		if b == '}' || b == ']' {
			if lastCommaIdx != -1 {
				// Remove the trailing comma
				raw := out.Bytes()
				copy(raw[lastCommaIdx:], raw[lastCommaIdx+1:])
				out.Truncate(len(raw) - 1)
				lastCommaIdx = -1
			}
			out.WriteByte(b)
			continue
		}

		if !unicode.IsSpace(rune(b)) {
			lastCommaIdx = -1
		}
		out.WriteByte(b)
	}

	return out.Bytes()
}

// Unmarshal parses the JSONC-encoded data and stores the result
// in the value pointed to by v.
func Unmarshal(data []byte, v any) error {
	clean := StripCommentsAndTrailingCommas(data)
	return json.Unmarshal(clean, v)
}
