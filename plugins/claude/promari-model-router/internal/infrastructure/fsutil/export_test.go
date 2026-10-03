package fsutil

// Test-only hooks (compiled only with the tests).

// TempFile is the file WriteFileAtomic writes through.
type TempFile = tempFile

// SetCreateTemp replaces the temporary file factory and returns a restore func.
func SetCreateTemp(f func(dir, pattern string) (TempFile, error)) (restore func()) {
	old := createTemp
	createTemp = f
	return func() { createTemp = old }
}
