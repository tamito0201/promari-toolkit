package persistence_test

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"gorm.io/gorm"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/persistence"
)

var t0 = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func openAt(t *testing.T, path string, batch int) *persistence.DB {
	t.Helper()
	db, err := persistence.Open(path, persistence.Options{BusyTimeoutMS: 1000, Batch: batch})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func open(t *testing.T) *persistence.DB {
	t.Helper()
	return openAt(t, filepath.Join(t.TempDir(), "ledger.db"), 2)
}

func exec(t *testing.T, db *persistence.DB, sql string) {
	t.Helper()
	if err := db.Gorm().Exec(sql).Error; err != nil {
		t.Fatal(err)
	}
}

// seedPrompt is the brief of the seeded rows (stored only as its digest).
const seedPrompt = "abc"

// seed appends a prompt, a subagent and a subagent result, one second apart.
func seed(t *testing.T, repo *persistence.LedgerRepo) {
	t.Helper()
	for i, ev := range []model.EventKind{model.EventPrompt, model.EventSubagent, model.EventSubagentResult} {
		e := model.NewEntry(t0.Add(time.Duration(i)*time.Second), ev,
			model.WithSession("s", model.SessionModel{}), model.WithToolUse("t", false), model.WithPrompt(seedPrompt))
		if err := repo.Append(t.Context(), e); err != nil {
			t.Fatal(err)
		}
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestOpen(t *testing.T) {
	tests := []struct {
		name    string
		path    func(t *testing.T, dir string) string
		wantErr string // a substring of the expected error, empty for success
	}{
		{
			name: "creates the database 0600 in a new directory",
			path: func(_ *testing.T, dir string) string { return filepath.Join(dir, "sub", "ledger.db") },
		},
		{
			name: "reopens an existing database",
			path: func(t *testing.T, dir string) string {
				t.Helper()
				path := filepath.Join(dir, "ledger.db")
				if err := openAt(t, path, 1).Close(); err != nil {
					t.Fatal(err)
				}
				return path
			},
		},
		{
			name: "parent directory cannot be created", wantErr: "not a directory",
			path: func(t *testing.T, dir string) string {
				t.Helper()
				file := filepath.Join(dir, "file")
				if err := os.WriteFile(file, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				return filepath.Join(file, "ledger.db")
			},
		},
		{
			name: "a directory is not a database", wantErr: "open ledger database",
			path: func(_ *testing.T, dir string) string { return dir },
		},
		{
			name: "a view in place of the ledger table fails the migration", wantErr: "migrate ledger database",
			path: func(t *testing.T, dir string) string {
				t.Helper()
				path := filepath.Join(dir, "ledger.db")
				db := openAt(t, path, 1)
				exec(t, db, "DROP TABLE ledger")
				exec(t, db, "CREATE VIEW ledger AS SELECT 1 AS id")
				return path
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.path(t, t.TempDir())
			db, err := persistence.Open(path, persistence.Options{BusyTimeoutMS: 1000})
			if tt.wantErr != "" {
				if !strings.Contains(errString(err), tt.wantErr) {
					t.Fatalf("Open() error = %v, want it to mention %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(os.FileMode(0o600), info.Mode().Perm()); diff != "" {
				t.Errorf("mode mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestOpenReadOnly(t *testing.T) {
	tests := []struct {
		name     string
		existing bool
		wantErr  string
	}{
		{name: "an existing ledger opens and refuses writes", existing: true},
		{name: "a missing ledger is an error", wantErr: "open ledger read-only"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ledger.db")
			if tt.existing {
				seed(t, persistence.NewLedgerRepo(openAt(t, path, 1)))
			}
			db, err := persistence.OpenReadOnly(path, persistence.Options{BusyTimeoutMS: 1000})
			if tt.wantErr != "" {
				if !strings.Contains(errString(err), tt.wantErr) {
					t.Fatalf("OpenReadOnly() error = %v, want it to mention %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			// The second guard: even a statement that bypasses ValidateQuery cannot write.
			if err := db.Gorm().Exec("DELETE FROM ledger").Error; err == nil {
				t.Error("a write through the read-only connection succeeded")
			}
			_, rows, err := db.Query(t.Context(), "SELECT count(*) AS n FROM ledger", 10)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff([]map[string]any{{"n": int64(3)}}, rows); diff != "" {
				t.Errorf("rows mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestClose(t *testing.T) {
	tests := []struct {
		name    string
		db      func(t *testing.T) *persistence.DB
		close   func(*persistence.DB) error
		wantErr error
	}{
		{name: "Close closes an open database", db: open, close: (*persistence.DB).Close},
		{name: "Shutdown closes it for the DI container", db: open, close: (*persistence.DB).Shutdown},
		{
			name:    "a handle without a connection pool is an error",
			db:      func(*testing.T) *persistence.DB { return persistence.WrapGorm(&gorm.DB{Config: &gorm.Config{}}) },
			close:   (*persistence.DB).Close,
			wantErr: gorm.ErrInvalidDB,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.close(tt.db(t)); !errors.Is(err, tt.wantErr) {
				t.Errorf("close error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestLedgerAppend(t *testing.T) {
	tests := []struct {
		name    string
		breakDB func(t *testing.T, db *persistence.DB)
		wantErr bool
	}{
		{name: "rows are appended and chained"},
		{
			name: "reading the latest row fails", wantErr: true,
			breakDB: func(t *testing.T, db *persistence.DB) {
				t.Helper()
				exec(t, db, "DROP TABLE ledger")
			},
		},
		{
			name: "the insert fails", wantErr: true,
			breakDB: func(t *testing.T, db *persistence.DB) {
				t.Helper()
				exec(t, db, "CREATE TRIGGER no_insert BEFORE INSERT ON ledger BEGIN SELECT RAISE(ABORT, 'refused'); END")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := open(t)
			repo := persistence.NewLedgerRepo(db)
			if tt.breakDB != nil {
				tt.breakDB(t, db)
			}
			err := repo.Append(t.Context(), model.NewEntry(t0, model.EventPrompt))
			if (err != nil) != tt.wantErr {
				t.Fatalf("Append() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if err := repo.Append(t.Context(), model.NewEntry(t0.Add(time.Second), model.EventSubagent)); err != nil {
				t.Fatal(err)
			}
			var rows []persistence.EntryRow
			if err := db.Gorm().Order("id").Find(&rows).Error; err != nil {
				t.Fatal(err)
			}
			got := [][2]string{{rows[0].PrevHash, rows[0].Hash}, {rows[1].PrevHash, rows[1].Hash}}
			want := [][2]string{{"", persistence.RowHash("", rows[0])}, {rows[0].Hash, persistence.RowHash(rows[0].Hash, rows[1])}}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("hash chain mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLedgerSince(t *testing.T) {
	all := []model.EventKind{model.EventPrompt, model.EventSubagent, model.EventSubagentResult}
	tests := []struct {
		name    string
		batch   int
		from    time.Time
		take    int // stop after this many (0 = all)
		breakDB bool
		want    []model.EventKind
		wantErr bool
	}{
		{name: "from the second row, across two batches", batch: 2, from: t0.Add(time.Second), want: all[1:]},
		{name: "everything in batches of one", batch: 1, from: t0, want: all},
		{name: "a batch of zero is treated as one", batch: 0, from: t0, want: all},
		{name: "a batch larger than the table", batch: 10, from: time.Time{}, want: all},
		{name: "nothing after the last row", batch: 2, from: t0.Add(time.Hour), want: nil},
		{name: "the consumer may stop early", batch: 2, from: t0, take: 1, want: all[:1]},
		{name: "a query error is yielded", batch: 2, from: t0, breakDB: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openAt(t, filepath.Join(t.TempDir(), "ledger.db"), tt.batch)
			repo := persistence.NewLedgerRepo(db)
			seed(t, repo)
			if tt.breakDB {
				exec(t, db, "DROP TABLE ledger")
			}
			var (
				got    []model.EventKind
				gotErr error
			)
			for e, err := range repo.Since(t.Context(), tt.from) {
				if err != nil {
					gotErr = err
					break
				}
				got = append(got, e.Event)
				if tt.take > 0 && len(got) == tt.take {
					break
				}
			}
			if (gotErr != nil) != tt.wantErr {
				t.Fatalf("Since() error = %v, wantErr %v", gotErr, tt.wantErr)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("events mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLedgerVerify(t *testing.T) {
	type result struct {
		Checked int
		Broken  uint
	}
	tests := []struct {
		name    string
		seed    bool
		tamper  func(t *testing.T, db *persistence.DB, repo *persistence.LedgerRepo)
		want    result
		wantErr bool
	}{
		{name: "an empty ledger is intact", want: result{}},
		{name: "an untouched chain is intact", seed: true, want: result{Checked: 3}},
		{
			name: "an edited row breaks the chain exactly there", seed: true, want: result{Checked: 1, Broken: 2},
			tamper: func(t *testing.T, db *persistence.DB, _ *persistence.LedgerRepo) {
				t.Helper()
				exec(t, db, "UPDATE ledger SET target = 'opus' WHERE id = 2")
			},
		},
		{
			name: "a deleted row breaks the link of the next one", seed: true, want: result{Checked: 1, Broken: 3},
			tamper: func(t *testing.T, db *persistence.DB, _ *persistence.LedgerRepo) {
				t.Helper()
				exec(t, db, "DELETE FROM ledger WHERE id = 2")
			},
		},
		{
			name: "pruning restarts the chain at the oldest kept row", seed: true, want: result{Checked: 2},
			tamper: func(t *testing.T, _ *persistence.DB, repo *persistence.LedgerRepo) {
				t.Helper()
				if _, err := repo.PruneOlderThan(t.Context(), t0.Add(time.Second)); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "a query error is returned", seed: true, wantErr: true,
			tamper: func(t *testing.T, db *persistence.DB, _ *persistence.LedgerRepo) {
				t.Helper()
				exec(t, db, "DROP TABLE ledger")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := open(t)
			repo := persistence.NewLedgerRepo(db)
			if tt.seed {
				seed(t, repo)
			}
			if tt.tamper != nil {
				tt.tamper(t, db, repo)
			}
			checked, broken, err := repo.Verify(t.Context())
			if (err != nil) != tt.wantErr {
				t.Fatalf("Verify() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.want, result{checked, broken}); diff != "" {
				t.Errorf("Verify() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLedgerSeenPrompt(t *testing.T) {
	tests := []struct {
		name, session, sha string
		orphan             string // the brief of a subagent row without a session
		breakDB            bool
		want               bool
		wantErr            bool
	}{
		{name: "the subagent row of the session is found", session: "s", sha: model.PromptDigest(seedPrompt), want: true},
		{name: "another session has not seen it", session: "other", sha: model.PromptDigest(seedPrompt)},
		{name: "another brief has not been seen", session: "s", sha: "zzz"},
		{name: "an empty session id is never seen", session: "", sha: model.PromptDigest(seedPrompt)},
		{name: "an empty session id does not match a row without a session", session: "", sha: model.PromptDigest("orphan"), orphan: "orphan"},
		{name: "an empty hash is never seen", session: "s", sha: ""},
		{name: "a query error is returned", session: "s", sha: model.PromptDigest(seedPrompt), breakDB: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := open(t)
			repo := persistence.NewLedgerRepo(db)
			seed(t, repo)
			if tt.orphan != "" {
				if err := repo.Append(t.Context(), model.NewEntry(t0, model.EventSubagent, model.WithPrompt(tt.orphan))); err != nil {
					t.Fatal(err)
				}
			}
			if tt.breakDB {
				exec(t, db, "DROP TABLE ledger")
			}
			got, err := repo.SeenPrompt(t.Context(), tt.session, tt.sha)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SeenPrompt() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("SeenPrompt() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLedgerPruneOlderThan(t *testing.T) {
	tests := []struct {
		name     string
		cutoff   time.Time
		breakDB  bool
		want     int64
		wantLeft []model.EventKind
		wantErr  bool
	}{
		{name: "nothing is older than the first row", cutoff: t0, want: 0, wantLeft: []model.EventKind{model.EventPrompt, model.EventSubagent, model.EventSubagentResult}},
		{name: "rows strictly before the cutoff are deleted", cutoff: t0.Add(2 * time.Second), want: 2, wantLeft: []model.EventKind{model.EventSubagentResult}},
		{name: "a delete error is returned", cutoff: t0, breakDB: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := open(t)
			repo := persistence.NewLedgerRepo(db)
			seed(t, repo)
			if tt.breakDB {
				exec(t, db, "DROP TABLE ledger")
			}
			n, err := repo.PruneOlderThan(t.Context(), tt.cutoff)
			if (err != nil) != tt.wantErr {
				t.Fatalf("PruneOlderThan() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			var left []model.EventKind
			for e, err := range repo.Since(t.Context(), time.Time{}) {
				if err != nil {
					t.Fatal(err)
				}
				left = append(left, e.Event)
			}
			if diff := cmp.Diff([]any{tt.want, tt.wantLeft}, []any{n, left}); diff != "" {
				t.Errorf("PruneOlderThan() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSessionRepo(t *testing.T) {
	at := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	save := func(id, m string, when time.Time) func(t *testing.T, r *persistence.SessionRepo) {
		return func(t *testing.T, r *persistence.SessionRepo) {
			t.Helper()
			if err := r.Save(t.Context(), model.Session{ID: id, Model: m, Source: model.SourceSessionState, UpdatedAt: when}); err != nil {
				t.Fatal(err)
			}
		}
	}
	type found struct {
		Session model.Session
		OK      bool
	}
	tests := []struct {
		name      string
		setup     []func(t *testing.T, r *persistence.SessionRepo)
		pruneAt   time.Time // zero: no prune
		breakDB   bool
		find      string
		want      found
		wantPrune int64
		wantErr   bool
	}{
		{
			name:  "save upserts: the second model wins and the injected time is kept",
			setup: []func(*testing.T, *persistence.SessionRepo){save("s", "claude-opus-5-5", at), save("s", "haiku", at)},
			find:  "s", want: found{model.Session{ID: "s", Model: "haiku", Source: model.SourceSessionState, UpdatedAt: at}, true},
		},
		{name: "an unknown session is not found", find: "missing"},
		{
			name:    "prune deletes sessions not updated since the cutoff",
			setup:   []func(*testing.T, *persistence.SessionRepo){save("old", "haiku", at), save("new", "sonnet", at.Add(time.Hour))},
			pruneAt: at.Add(time.Minute), wantPrune: 1, find: "old",
		},
		{
			name:    "prune keeps sessions updated since the cutoff",
			setup:   []func(*testing.T, *persistence.SessionRepo){save("old", "haiku", at), save("new", "sonnet", at.Add(time.Hour))},
			pruneAt: at.Add(time.Minute), wantPrune: 1, find: "new",
			want: found{model.Session{ID: "new", Model: "sonnet", Source: model.SourceSessionState, UpdatedAt: at.Add(time.Hour)}, true},
		},
		{name: "a find error is returned", breakDB: true, find: "s", wantErr: true},
		{name: "a prune error is returned", breakDB: true, pruneAt: at, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := open(t)
			repo := persistence.NewSessionRepo(db)
			for _, f := range tt.setup {
				f(t, repo)
			}
			if tt.breakDB {
				exec(t, db, "DROP TABLE sessions")
			}
			if !tt.pruneAt.IsZero() {
				n, err := repo.PruneOlderThan(t.Context(), tt.pruneAt)
				if tt.wantErr {
					if err == nil {
						t.Fatal("PruneOlderThan() succeeded on a broken database")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if diff := cmp.Diff(tt.wantPrune, n); diff != "" {
					t.Errorf("pruned mismatch (-want +got):\n%s", diff)
				}
			}
			s, ok, err := repo.Find(t.Context(), tt.find)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Find() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.want, found{s, ok}); diff != "" {
				t.Errorf("Find() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValidateQuery(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		ok   bool
	}{
		{name: "plain select", sql: "SELECT * FROM ledger", ok: true},
		{name: "with clause and trailing semicolon", sql: "  with x as (select 1) select * from x;", ok: true},
		{name: "delete", sql: "DELETE FROM ledger"},
		{name: "second statement", sql: "SELECT 1; DROP TABLE ledger"},
		{name: "pragma", sql: "PRAGMA table_info(ledger)"},
		{name: "attach after a select", sql: "select * from ledger where reason = 'x'; attach database 'y' as z"},
		{name: "write hidden in a with clause", sql: "WITH a AS (SELECT 1) UPDATE x SET y=1"},
		{name: "empty", sql: ""},
		{name: "two selects", sql: "select 1; select 2"},
		{name: "attach hidden in a with clause", sql: "WITH a AS (SELECT 1) ATTACH DATABASE 'y' AS z"},
		{name: "a forbidden word inside a column name is not a write", sql: "SELECT updated_at FROM sessions", ok: true},
		{name: "select not at the start", sql: "EXPLAIN QUERY PLAN SELECT 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := persistence.ValidateQuery(tt.sql)
			if (err == nil) != tt.ok {
				t.Errorf("ValidateQuery(%q) = %v, want ok=%v", tt.sql, err, tt.ok)
			}
			if err != nil && !errors.Is(err, persistence.ErrNotReadOnly) {
				t.Errorf("unexpected error type: %v", err)
			}
		})
	}
}

func TestQuery(t *testing.T) {
	type result struct {
		Cols []string
		Rows []map[string]any
	}
	tests := []struct {
		name    string
		sql     string
		limit   int
		want    result
		wantErr error // nil with wantAny = any error
		wantAny bool
	}{
		{
			name: "rows come back as maps", sql: "SELECT id, event FROM ledger ORDER BY id", limit: 10,
			want: result{[]string{"id", "event"}, []map[string]any{
				{"id": int64(1), "event": "prompt"}, {"id": int64(2), "event": "subagent"}, {"id": int64(3), "event": "subagent_result"},
			}},
		},
		{
			name: "the limit caps the rows", sql: "SELECT id FROM ledger ORDER BY id", limit: 1,
			want: result{[]string{"id"}, []map[string]any{{"id": int64(1)}}},
		},
		{
			name: "blobs become strings", sql: "SELECT x'6869' AS b", limit: 10,
			want: result{[]string{"b"}, []map[string]any{{"b": "hi"}}},
		},
		{name: "a write is rejected before it reaches the database", sql: "DELETE FROM ledger", limit: 10, wantErr: persistence.ErrNotReadOnly},
		{name: "a database error is returned", sql: "SELECT * FROM nope", limit: 10, wantAny: true},
	}
	db := open(t)
	seed(t, persistence.NewLedgerRepo(db))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cols, rows, err := db.Query(t.Context(), tt.sql, tt.limit)
			switch {
			case tt.wantAny:
				if err == nil {
					t.Fatal("Query() succeeded, want an error")
				}
				return
			case !errors.Is(err, tt.wantErr):
				t.Fatalf("Query() error = %v, want %v", err, tt.wantErr)
			case err != nil:
				return
			}
			if diff := cmp.Diff(tt.want, result{cols, rows}); diff != "" {
				t.Errorf("Query() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// fakeRows is a cursor that fails where told to.
type fakeRows struct {
	cols            []string
	rows            [][]any
	colErr, scanErr error
	iterErr         error
	next            int
}

func (f *fakeRows) Columns() ([]string, error) { return f.cols, f.colErr }
func (f *fakeRows) Next() bool                 { f.next++; return f.next <= len(f.rows) }
func (f *fakeRows) Err() error                 { return f.iterErr }
func (f *fakeRows) Scan(dest ...any) error {
	if f.scanErr != nil {
		return f.scanErr
	}
	for i, v := range f.rows[f.next-1] {
		*dest[i].(*any) = v
	}
	return nil
}

func TestCollect(t *testing.T) {
	boom := errors.New("boom")
	type result struct {
		Cols []string
		Rows []map[string]any
	}
	tests := []struct {
		name    string
		rows    *fakeRows
		want    result
		wantErr error
	}{
		{
			name: "values are copied and bytes become strings",
			rows: &fakeRows{cols: []string{"a", "b"}, rows: [][]any{{int64(1), []byte("x")}, {nil, "y"}}},
			want: result{[]string{"a", "b"}, []map[string]any{{"a": int64(1), "b": "x"}, {"a": nil, "b": "y"}}},
		},
		{name: "a columns error stops before reading", rows: &fakeRows{colErr: boom}, wantErr: boom},
		{name: "a scan error is returned", rows: &fakeRows{cols: []string{"a"}, rows: [][]any{{1}}, scanErr: boom}, wantErr: boom},
		{
			name: "an iteration error is returned with the rows read so far",
			rows: &fakeRows{cols: []string{"a"}, rows: [][]any{{"v"}}, iterErr: boom}, wantErr: boom,
			want: result{[]string{"a"}, []map[string]any{{"a": "v"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cols, rows, err := persistence.Collect(tt.rows, 10)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Collect() error = %v, want %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.want, result{cols, rows}); diff != "" {
				t.Errorf("Collect() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSchema(t *testing.T) {
	tests := []struct {
		name    string
		closed  bool
		want    []string // table names in order
		wantErr bool
	}{
		{name: "both tables, ordered by name", want: []string{"ledger", "sessions"}},
		{name: "a closed database is an error", closed: true, wantErr: true},
	}
	tableName := regexp.MustCompile("(?i)^CREATE TABLE [`\"]?(\\w+)")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := open(t)
			if tt.closed {
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
			}
			stmts, err := db.Schema(t.Context())
			if (err != nil) != tt.wantErr {
				t.Fatalf("Schema() error = %v, wantErr %v", err, tt.wantErr)
			}
			var names []string
			for s := range slices.Values(stmts) {
				if m := tableName.FindStringSubmatch(s); m != nil {
					names = append(names, m[1])
				}
			}
			if diff := cmp.Diff(tt.want, names); diff != "" {
				t.Errorf("Schema() tables mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func FuzzValidateQueryNeverAcceptsWrites(f *testing.F) {
	for _, seed := range []string{"SELECT 1", "DELETE FROM ledger", "select 1;delete from ledger", "WITH a AS (SELECT 1) UPDATE x SET y=1"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, sql string) {
		if persistence.ValidateQuery(sql) == nil {
			for _, bad := range []string{"delete", "update", "insert", "drop", "attach", "pragma"} {
				if containsWord(sql, bad) {
					t.Fatalf("accepted a statement containing %q: %q", bad, sql)
				}
			}
		}
	})
}

func containsWord(s, w string) bool {
	return regexp.MustCompile(`(?i)\b` + w + `\b`).MatchString(s)
}

func TestReadOnlyLedger(t *testing.T) {
	type result struct {
		Columns []string
		Rows    []map[string]any
		Schema  int // number of CREATE statements
	}
	tests := []struct {
		name     string
		existing bool
		sql      string
		want     result
		wantErr  string // substring of both errors; "" = success
	}{
		{
			name: "an existing ledger answers queries and its schema", existing: true, sql: "SELECT count(*) AS n FROM ledger",
			want: result{Columns: []string{"n"}, Rows: []map[string]any{{"n": int64(3)}}, Schema: 2},
		},
		{name: "a missing ledger is not created", sql: "SELECT 1", wantErr: "open ledger read-only"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ledger.db")
			if tt.existing {
				seed(t, persistence.NewLedgerRepo(openAt(t, path, 1)))
			}
			l := persistence.ReadOnlyLedger{Path: path, Options: persistence.Options{BusyTimeoutMS: 1000}}
			res, qErr := l.Query(t.Context(), tt.sql, 10)
			stmts, sErr := l.Schema(t.Context())
			if tt.wantErr != "" {
				for _, err := range []error{qErr, sErr} {
					if !strings.Contains(errString(err), tt.wantErr) {
						t.Errorf("error = %v, want it to mention %q", err, tt.wantErr)
					}
				}
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("the ledger was created: %v", err)
				}
				return
			}
			if qErr != nil || sErr != nil {
				t.Fatal(qErr, sErr)
			}
			if diff := cmp.Diff(tt.want, result{res.Columns, res.Rows, len(stmts)}); diff != "" {
				t.Errorf("result (-want +got):\n%s", diff)
			}
		})
	}
}
