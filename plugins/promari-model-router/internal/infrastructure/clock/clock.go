// Package clock provides the system clock and a fixed clock for tests.
package clock

import "time"

// System is the wall clock.
type System struct{}

// Now returns time.Now.
func (System) Now() time.Time { return time.Now() }

// Fixed always returns the same instant.
type Fixed struct{ At time.Time }

// Now returns the fixed instant.
func (f Fixed) Now() time.Time { return f.At }
