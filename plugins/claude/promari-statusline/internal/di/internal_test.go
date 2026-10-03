package di

import "testing"

func TestExecutableSuffix(t *testing.T) {
	t.Parallel()
	if executableSuffix("windows") != ".exe" || executableSuffix("darwin") != "" || executableSuffix("linux") != "" {
		t.Error("only Windows names its binaries with .exe")
	}
}
