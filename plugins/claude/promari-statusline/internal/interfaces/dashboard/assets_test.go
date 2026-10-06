package dashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// web is the TypeScript source of the page, relative to this package.
const web = "../../../web"

// sourceHash must match the hash web/build.mjs writes to assets/source.sha256:
// SHA-256 over each input, in sorted order, as "<name>\0<content>\0". The
// inputs are web/src (without its tests), build.mjs, package.json and the lock.
func sourceHash(t *testing.T, dir fs.FS) string {
	t.Helper()
	src, err := fs.ReadDir(dir, "src")
	if err != nil {
		t.Fatal(err)
	}
	inputs := []string{"build.mjs", "package.json", "pnpm-lock.yaml"}
	for _, e := range src {
		if !strings.HasSuffix(e.Name(), ".test.ts") {
			inputs = append(inputs, path.Join("src", e.Name()))
		}
	}
	slices.Sort(inputs)
	h := sha256.New()
	for _, name := range inputs {
		data, err := fs.ReadFile(dir, name)
		if err != nil {
			t.Fatal(err)
		}
		h.Write([]byte(name + "\x00"))
		h.Write(data)
		h.Write([]byte("\x00"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// The release builds the binary with Go alone, so the page it embeds is the
// one committed. A source changed without `pnpm build` (in web/) would ship
// the old page without a word; this test is where that shows.
func TestTheAssetsAreBuiltFromTheSources(t *testing.T) {
	t.Parallel()
	recorded, err := fs.ReadFile(assets, "assets/source.sha256")
	if err != nil {
		t.Fatalf("assets/source.sha256 is missing: run `pnpm build` in web/: %v", err)
	}
	if got := sourceHash(t, os.DirFS(web)); got != strings.TrimSpace(string(recorded)) {
		t.Errorf("the embedded page was built from other sources (%s, the sources hash to %s): run `pnpm build` in web/ and commit the assets", strings.TrimSpace(string(recorded)), got)
	}
	for _, name := range []string{"index.html", "app.js", "app.css"} {
		if _, err := fs.Stat(assets, "assets/"+name); err != nil {
			t.Errorf("assets/%s is missing", name)
		}
	}
}

// The check has teeth: one byte more in a source, or one more source, changes
// the hash; a test file does not.
func TestTheSourceHashSeesEveryInput(t *testing.T) {
	t.Parallel()
	base := fstest.MapFS{
		"build.mjs":      {Data: []byte("build")},
		"package.json":   {Data: []byte("{}")},
		"pnpm-lock.yaml": {Data: []byte("lock")},
		"src/main.ts":    {Data: []byte("main")},
	}
	want := sourceHash(t, base)
	changed := func(edit func(fstest.MapFS)) string {
		c := fstest.MapFS{}
		for k, v := range base {
			c[k] = &fstest.MapFile{Data: slices.Clone(v.Data)}
		}
		edit(c)
		return sourceHash(t, c)
	}
	if changed(func(c fstest.MapFS) { c["src/main.ts"].Data = []byte("main!") }) == want {
		t.Error("an edited source keeps the hash")
	}
	if changed(func(c fstest.MapFS) { c["src/style.css"] = &fstest.MapFile{Data: []byte("")} }) == want {
		t.Error("a new source keeps the hash")
	}
	if changed(func(c fstest.MapFS) { c["pnpm-lock.yaml"].Data = []byte("lock2") }) == want {
		t.Error("a new lock keeps the hash")
	}
	if changed(func(c fstest.MapFS) { c["src/main.test.ts"] = &fstest.MapFile{Data: []byte("test")} }) != want {
		t.Error("a test file changes the hash")
	}
}
