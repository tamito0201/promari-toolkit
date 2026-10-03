// Package fp holds small functional building blocks over iter.Seq.
//
// The standard library already provides slices.Collect, slices.Values,
// maps.Keys and friends; this package adds the lazy transformations the
// router needs (Map, Filter, Reduce, CountBy, GroupBy) and an Option type,
// all as pure functions without hidden state.
package fp

import (
	"cmp"
	"iter"
	"maps"
	"slices"
)

// Map lazily applies f to every element.
func Map[T, U any](seq iter.Seq[T], f func(T) U) iter.Seq[U] {
	return func(yield func(U) bool) {
		for v := range seq {
			if !yield(f(v)) {
				return
			}
		}
	}
}

// Filter lazily keeps the elements for which keep returns true.
func Filter[T any](seq iter.Seq[T], keep func(T) bool) iter.Seq[T] {
	return func(yield func(T) bool) {
		for v := range seq {
			if keep(v) && !yield(v) {
				return
			}
		}
	}
}

// Reduce folds the sequence into a single value.
func Reduce[T, A any](seq iter.Seq[T], init A, f func(A, T) A) A {
	acc := init
	for v := range seq {
		acc = f(acc, v)
	}
	return acc
}

// Count returns the number of elements that satisfy pred.
func Count[T any](seq iter.Seq[T], pred func(T) bool) int {
	return Reduce(seq, 0, func(n int, v T) int {
		if pred(v) {
			return n + 1
		}
		return n
	})
}

// SumBy adds up f over the sequence.
func SumBy[T any, N cmp.Ordered](seq iter.Seq[T], f func(T) N) N {
	var zero N
	return Reduce(seq, zero, func(acc N, v T) N { return acc + f(v) })
}

// CountBy counts elements per key.
func CountBy[T any, K comparable](seq iter.Seq[T], key func(T) K) map[K]int {
	return Reduce(seq, map[K]int{}, func(m map[K]int, v T) map[K]int {
		m[key(v)]++
		return m
	})
}

// GroupBy groups elements per key, preserving order within a group.
func GroupBy[T any, K comparable](seq iter.Seq[T], key func(T) K) map[K][]T {
	return Reduce(seq, map[K][]T{}, func(m map[K][]T, v T) map[K][]T {
		k := key(v)
		m[k] = append(m[k], v)
		return m
	})
}

// IndexBy builds a lookup table; a later element with the same key wins.
func IndexBy[T any, K comparable](seq iter.Seq[T], key func(T) K) map[K]T {
	return Reduce(seq, map[K]T{}, func(m map[K]T, v T) map[K]T {
		m[key(v)] = v
		return m
	})
}

// Take yields at most n elements.
func Take[T any](seq iter.Seq[T], n int) iter.Seq[T] {
	return func(yield func(T) bool) {
		if n <= 0 {
			return
		}
		taken := 0
		for v := range seq {
			if !yield(v) {
				return
			}
			taken++
			if taken >= n {
				return
			}
		}
	}
}

// SortedKeys returns the keys of m in ascending order.
func SortedKeys[K cmp.Ordered, V any](m map[K]V) []K {
	return slices.Sorted(maps.Keys(m))
}

// Option is a value that may be absent, without resorting to pointers.
type Option[T any] struct {
	value T
	ok    bool
}

// Some wraps a present value.
func Some[T any](v T) Option[T] { return Option[T]{value: v, ok: true} }

// None is the absent value.
func None[T any]() Option[T] { return Option[T]{} }

// OptionFrom converts the common (value, ok) pair.
func OptionFrom[T any](v T, ok bool) Option[T] { return Option[T]{value: v, ok: ok} }

// Get returns the value and whether it is present.
func (o Option[T]) Get() (T, bool) { return o.value, o.ok }

// IsSome reports whether the value is present.
func (o Option[T]) IsSome() bool { return o.ok }

// OrElse returns the value or the fallback.
func (o Option[T]) OrElse(fallback T) T {
	if o.ok {
		return o.value
	}
	return fallback
}

// MapOption transforms a present value.
func MapOption[T, U any](o Option[T], f func(T) U) Option[U] {
	if v, ok := o.Get(); ok {
		return Some(f(v))
	}
	return None[U]()
}

// FirstSome returns the first present option produced by the thunks, evaluated lazily.
func FirstSome[T any](thunks ...func() Option[T]) Option[T] {
	for _, thunk := range thunks {
		if o := thunk(); o.IsSome() {
			return o
		}
	}
	return None[T]()
}

// Apply binds one argument to every function, producing thunks for FirstSome.
func Apply[A, T any](fs []func(A) Option[T], arg A) []func() Option[T] {
	return slices.Collect(Map(slices.Values(fs), func(f func(A) Option[T]) func() Option[T] {
		return func() Option[T] { return f(arg) }
	}))
}
