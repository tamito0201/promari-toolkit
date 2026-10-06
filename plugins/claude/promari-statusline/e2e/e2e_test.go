// Package e2e_test は、ビルドした psl を Claude Code と利用者が使うのと同じ形——
// 専用のホームディレクトリを持つ1プロセス——で動かす E2E テストである。
//
// シナリオは books/*.yml の runn ランブックに書き、期待値は runn の compare で検証する。
// この Go ファイルが受け持つのは、ランブックの外にしか置けない準備だけである:
// psl のビルド、ホームの用意（ネットワークへ出ないよう HTTP の答えを新鮮なキャッシュとして置く）、
// ダッシュボード用の空きポートの確保。
//
// runn は依存が大きいので、配布する psl のモジュールではなく、このテスト専用のモジュールに置く。
package e2e_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/k1LoW/runn"

	"promari-statusline/internal/domain/service"
)

// binary はこのテスト実行のためにビルドした psl。
var binary string

func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "psl-e2e")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }()
	binary = filepath.Join(dir, "psl")
	args := []string{"build", "-o", binary}
	// task coverage では、バイナリが自分の文を単体テストと同じディレクトリへ数える（Taskfile.yml）。
	if os.Getenv("PSL_E2E_COVERDIR") != "" {
		args = append(args, "-cover", "-coverpkg=promari-statusline/...")
	}
	build := exec.CommandContext(context.Background(), "go", append(args, "./cmd/psl")...)
	build.Dir = ".." // 配布するモジュールの根
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build psl: %v\n%s", err, out)
		return 1
	}
	return m.Run()
}

// TestBooks は books/ の各ランブックを、それぞれ新しいホームで並行に走らせる。
func TestBooks(t *testing.T) {
	t.Parallel()
	books, err := filepath.Glob("books/*.yml")
	if err != nil || len(books) == 0 {
		t.Fatalf("ランブックが見つからない: %v", err)
	}
	testdata, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, book := range books {
		t.Run(strings.TrimSuffix(filepath.Base(book), ".yml"), func(t *testing.T) {
			t.Parallel()
			home := newHome(t)
			port := freePort(t)
			op, err := runn.New(
				runn.Book(book),
				runn.T(t),
				// 実行ファイルを起動し、テスト用ホーム（ランブックの外）のファイルを読む権限だけを与える。
				runn.Scopes("run:exec", "read:parent"),
				runn.Var("psl", binary),
				runn.Var("home", home),
				runn.Var("env", sandboxEnv(home)),
				runn.Var("testdata", testdata),
				runn.Var("port", port),
				runn.Runner("dashboard", "http://127.0.0.1:"+port),
				runn.Func("plain", plain),
				runn.Func("widest", widest),
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := op.Run(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// newHome は1本のランブック用のホームを作る。2つの HTTP エンドポイントの答えを
// 新鮮なキャッシュとして置き、描画がネットワークへ問い合わせないようにする。
func newHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	now := time.Now().Format(time.RFC3339)
	for name, content := range map[string]string{
		".cache/promari-statusline/incident.json": `{"at":"` + now + `","found":false,"value":{"indicator":""}}`,
		".cache/promari-statusline/latest.json":   `{"at":"` + now + `","found":true,"value":"99.0.0"}`,
	} {
		path := filepath.Join(home, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

// sandboxEnv は psl に渡す環境の全部を `env -i` の引数として返す。利用者の環境変数を
// テストへ持ち込まない。coverage の実行中だけ GOCOVERDIR を足す。
func sandboxEnv(home string) string {
	vars := []string{"env", "-i", "HOME=" + strconv.Quote(home), "PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	if dir := os.Getenv("PSL_E2E_COVERDIR"); dir != "" {
		vars = append(vars, "GOCOVERDIR="+strconv.Quote(dir))
	}
	return strings.Join(vars, " ")
}

// freePort は空いている TCP ポートを返す。閉じてから psl が開くまでの間に
// 他のプロセスが取る可能性は残るが、ループバックの短い間隔なので実用上は衝突しない。
func freePort(t *testing.T) string {
	t.Helper()
	l, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
}

var sgr = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// plain は端末の色指定（SGR）を取り除いた文字列を返す。
func plain(s string) string { return sgr.ReplaceAllString(s, "") }

// widest は各行の表示セル幅の最大値を返す。文字数ではなく psl と同じセル幅で数える。
func widest(s string) int {
	widest := 0
	for line := range strings.Lines(plain(s)) {
		widest = max(widest, service.Cells(strings.TrimSuffix(line, "\n")))
	}
	return widest
}
