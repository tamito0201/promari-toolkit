// Package jsonx reads JSON written by other programs, one member at a time.
//
// encoding/json/v2 stops at the first value of an unexpected type, which is
// right for data a program owns. The status line reads what Claude Code, Codex
// and the GitHub CLI print, and their formats change between versions. Here
// every member is decoded on its own, so a member that changed its type is
// lost alone and everything else is kept.
package jsonx

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
)

// Object is a JSON object whose members are still undecoded.
type Object map[string]jsontext.Value

// Parse decodes a JSON object. ok is false for anything else: invalid JSON, an
// array, a number, or nothing at all.
func Parse(data []byte) (obj Object, ok bool) {
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, false
	}
	return obj, true
}

// Get decodes one member as T. ok is false when the member is missing, null,
// or of another type.
func Get[T any](obj Object, name string) (v T, ok bool) {
	raw, found := obj[name]
	if !found || raw.Kind() == jsontext.KindNull {
		return v, false
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		var zero T
		return zero, false
	}
	return v, true
}

// Or decodes one member as T, or returns the zero value.
func Or[T any](obj Object, name string) T {
	v, _ := Get[T](obj, name)
	return v
}

// Child returns a member that is itself an object; a missing member or one of
// another type gives an empty object, so lookups can be chained.
func Child(obj Object, name string) Object {
	child, _ := Get[Object](obj, name)
	return child
}
