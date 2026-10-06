# E2E テスト（runn）

ビルドした `psl` を、Claude Code と利用者が使うのと同じ形——専用のホームディレクトリを持つ1プロセス——で動かして確かめる。
シナリオは `books/*.yml` の [runn](https://github.com/k1LoW/runn) ランブックに書き、期待値は runn の組み込み関数 `compare` で検証する。

runn は依存が大きい（約180パッケージ）ので、配布する `psl` のモジュールには入れず、このディレクトリを別モジュール
`promari-statusline/e2e` にしている。`./...` は入れ子のモジュールに入らないため、Taskfile の `test`・`race`・`coverage`・
`lint-code`・`security` は E2E を明示して走らせる。

## 実行

```sh
go -C e2e test ./...                          # 全ランブック
go -C e2e test -run 'TestBooks/research' ./... # 1本だけ
sh tools/run.sh task coverage                 # 単体＋E2E バイナリを合算したカバレッジ
```

## 役割の分け方

| 置き場所 | 受け持つこと |
|---|---|
| `e2e_test.go` | psl のビルド（coverage 時は `-cover`）、ランブックごとの新しいホーム、空きポート、runn への変数と関数の受け渡し |
| `books/*.yml` | シナリオの全部（実行・HTTP 要求・期待値・`compare`） |
| `testdata/` | 標準入力に渡す報告 JSON と会話記録。期待値は手計算できる形にし、その計算をランブックのコメントに書く |

ランブックから使える変数は `vars.psl`（バイナリ）・`vars.env`（`env -i HOME=… PATH=…`。利用者の環境を持ち込まない）・
`vars.home`・`vars.testdata`・`vars.port`、関数は `plain(s)`（色指定を除く）と `widest(s)`（各行の表示セル幅の最大）。
HTTP ランナー `dashboard` は `http://127.0.0.1:<port>` を指す。

## 書き方の規約（すべて実測で踏んだ落とし穴）

1. **期待値の検証にステップの `loop:` を使わない。** runn のステップループは、各回の `test:` のうち**最後の回の結果しか判定しない**
   （runn v1.11.1 で確認。1回目が偽でも最後が真ならステップは成功する）。表の各行を確かめるときは、
   - 出力の検査だけなら、`map` で「行 → 結果」の対応表を作り1回の `compare` で比べる
   - 行ごとに実行が要るなら、ループで実行して `bind: {'results[]': …}` に集め、ループの外で `compare(results, 期待の表)` する

   こうすると失敗の差分に行の名前が出る。`loop:` は `until:` 付きの待ち合わせ（サーバの起動・終了）にだけ使う。
2. **標準入力に渡す JSON はファイルにしてリダイレクトする。** `stdin: "{{ vars.x }}"` のようにテンプレートだけの値は、
   展開結果が `{}`・`[1]`・空文字だと map・配列・null に変換されて「invalid stdin」になる。
3. **`exec.command` は1行で書く。** ブロック（`|`）で書いた複数行は改行が `\n` の文字列に置き換わり、引数が壊れる
   （`render` が `rendern` になった）。
4. **ランブックの外のファイルを `file()` で読むには `read:parent` スコープが要る。** ハーネスは `run:exec` と `read:parent` だけを与えている。
5. **対照実験は先頭・途中・末尾の行で行う。** 末尾の行だけ壊して落ちることを確かめても、1. の空振りは見つからない。
   新しいランブックは、期待値を一時的に壊して、表のどの位置でも失敗することを確かめてから入れる。
6. **バックグラウンドのサーバは自分で止める。** `background: true` の子はランブックの終了で強制終了されるので、
   Ctrl-C での正常終了を確かめるには PID と終了コードをホームへ書き出し、`kill -INT` してから終了コードを `compare` する
   （`dashboard.yml` と `research.yml`）。

---

**作成者**: Takaomi Murasaki
