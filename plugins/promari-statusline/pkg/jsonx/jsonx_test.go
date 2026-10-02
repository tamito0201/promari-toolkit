package jsonx_test

import (
	"testing"

	"promari-statusline/pkg/jsonx"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data string
		want int
		ok   bool
	}{
		{"an object", `{"a":1,"b":{"c":2}}`, 2, true},
		{"an empty object", `{}`, 0, true},
		{"an array", `[1]`, 0, false},
		{"a number", `1`, 0, false},
		{"null", `null`, 0, true}, // null decodes to an object without members
		{"nothing", ``, 0, false},
		{"invalid JSON", `{"a":`, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if obj, ok := jsonx.Parse([]byte(tt.data)); len(obj) != tt.want || ok != tt.ok {
				t.Errorf("Parse(%s) = %d members, %v; want %d, %v", tt.data, len(obj), ok, tt.want, tt.ok)
			}
		})
	}
}

func TestGet(t *testing.T) {
	t.Parallel()
	obj, _ := jsonx.Parse([]byte(`{"n":0,"s":"text","b":false,"null":null,"o":{"inner":7},"list":[{"x":1},{"x":"two"}]}`))

	// A member of the right type is found even when it is the zero value.
	if v, ok := jsonx.Get[float64](obj, "n"); v != 0 || !ok {
		t.Errorf("Get[float64](n) = %v, %v; zero is a value", v, ok)
	}
	if v, ok := jsonx.Get[bool](obj, "b"); v || !ok {
		t.Errorf("Get[bool](b) = %v, %v; false is a value", v, ok)
	}
	// Missing, null and wrongly typed members are all "not there".
	for _, name := range []string{"missing", "null", "s"} {
		if v, ok := jsonx.Get[float64](obj, name); v != 0 || ok {
			t.Errorf("Get[float64](%s) = %v, %v; want absent", name, v, ok)
		}
	}
	if got := jsonx.Or[string](obj, "s"); got != "text" {
		t.Errorf("Or[string](s) = %q", got)
	}
	if got := jsonx.Or[string](obj, "n"); got != "" {
		t.Errorf("Or[string](n) = %q, want the zero value for another type", got)
	}
	if got := jsonx.Or[float64](jsonx.Child(obj, "o"), "inner"); got != 7 {
		t.Errorf("Child(o).inner = %v", got)
	}
	// A child that is missing or is no object gives an empty object, so that
	// lookups chain without a check at every step.
	for _, name := range []string{"missing", "s", "null"} {
		if got := jsonx.Or[float64](jsonx.Child(jsonx.Child(obj, name), "deeper"), "inner"); got != 0 {
			t.Errorf("a chain through %s = %v", name, got)
		}
	}
	// The items of a list are decoded one by one as well.
	items := jsonx.Or[[]jsonx.Object](obj, "list")
	if len(items) != 2 || jsonx.Or[float64](items[0], "x") != 1 || jsonx.Or[float64](items[1], "x") != 0 {
		t.Errorf("list = %v", items)
	}
}
