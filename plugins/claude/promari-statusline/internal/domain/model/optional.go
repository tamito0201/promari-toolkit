// Package model holds the status line's entities and value objects: what a
// session reports, what the machine around it looks like, and the chips, groups
// and lines those facts are shown as. Nothing here reads a file, starts a
// process or knows a colour code.
package model

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
)

// Optional is a value that may be absent. On a status line the two are
// different facts: a cost that was never reported hides its chip, a cost of
// zero shows "$0.00". The zero Optional is absent.
type Optional[T any] struct {
	value T
	ok    bool
}

// Some returns an Optional holding v.
func Some[T any](v T) Optional[T] { return Optional[T]{value: v, ok: true} }

// Get returns the value and whether it is present.
func (o Optional[T]) Get() (T, bool) { return o.value, o.ok }

// Present reports whether a value is held.
func (o Optional[T]) Present() bool { return o.ok }

// Or returns the value, or fallback when it is absent.
func (o Optional[T]) Or(fallback T) T {
	if o.ok {
		return o.value
	}
	return fallback
}

// MarshalJSONTo writes the value, or null when it is absent.
func (o Optional[T]) MarshalJSONTo(enc *jsontext.Encoder) error {
	if !o.ok {
		return enc.WriteToken(jsontext.Null)
	}
	return json.MarshalEncode(enc, &o.value)
}

// UnmarshalJSONFrom reads a value; null leaves the Optional absent.
func (o *Optional[T]) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	if dec.PeekKind() == jsontext.KindNull {
		*o = Optional[T]{}
		return dec.SkipValue()
	}
	var v T
	if err := json.UnmarshalDecode(dec, &v); err != nil {
		return err
	}
	*o = Some(v)
	return nil
}
