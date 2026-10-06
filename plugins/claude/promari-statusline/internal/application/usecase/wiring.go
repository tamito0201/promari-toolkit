package usecase

import (
	"fmt"
	"reflect"
)

// mustBeWired panics when a field of deps that holds an interface is nil,
// naming the field, and looks into the fields that are structs of their own
// (RenderDeps.Sources). The composition root wires every use case when the
// program starts, so a dependency left out stops the program at once with its
// name (fail fast) instead of panicking in the middle of a render or, worse,
// leaving a chip silently absent.
func mustBeWired(name string, deps any) {
	if missing := unwired(name, reflect.ValueOf(deps)); missing != "" {
		panic(fmt.Sprintf("usecase: %s is not wired", missing))
	}
}

func unwired(path string, v reflect.Value) string {
	for i := range v.NumField() {
		field, name := v.Field(i), path+"."+v.Type().Field(i).Name
		switch field.Kind() {
		case reflect.Interface, reflect.Func:
			if field.IsNil() {
				return name
			}
		case reflect.Struct:
			if missing := unwired(name, field); missing != "" {
				return missing
			}
		default:
		}
	}
	return ""
}
