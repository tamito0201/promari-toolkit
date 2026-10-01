package persistence_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/infrastructure/persistence"
)

// TestLedgerConcurrentAppend: hooks run as separate processes, each with its
// own connection. A deferred transaction reads the chain head and then asks
// for the write lock; two of them deadlock and SQLite answers SQLITE_BUSY at
// once (the busy timeout does not apply to a lock upgrade), so rows were lost.
func TestLedgerConcurrentAppend(t *testing.T) {
	type result struct {
		Failed, Stored, Checked int
		Broken                  uint
	}
	tests := []struct {
		name       string
		conns, per int
	}{
		{name: "two connections", conns: 2, per: 20},
		{name: "four connections", conns: 4, per: 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ledger.db")
			repos := make([]*persistence.LedgerRepo, tt.conns)
			for i := range repos {
				repos[i] = persistence.NewLedgerRepo(openAt(t, path, 100))
			}
			var (
				mu     sync.Mutex
				failed int
				wg     sync.WaitGroup
			)
			start := make(chan struct{})
			for _, repo := range repos {
				wg.Go(func() {
					<-start
					for range tt.per {
						if err := repo.Append(t.Context(), model.NewEntry(time.Now(), model.EventSubagent)); err != nil {
							mu.Lock()
							failed++
							mu.Unlock()
						}
					}
				})
			}
			close(start)
			wg.Wait()
			stored := 0
			for _, err := range repos[0].Since(t.Context(), time.Time{}) {
				if err != nil {
					t.Fatal(err)
				}
				stored++
			}
			checked, broken, err := repos[0].Verify(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			want := result{Stored: tt.conns * tt.per, Checked: tt.conns * tt.per}
			if diff := cmp.Diff(want, result{failed, stored, checked, broken}); diff != "" {
				t.Errorf("concurrent append (-want +got):\n%s", diff)
			}
		})
	}
}

// TestLedgerVerifyCoversEveryColumn: the hash used to cover only some columns,
// so editing the outcome of a subagent (status, tokens) or its safety flag
// went unnoticed by `pmr verify`.
func TestLedgerVerifyCoversEveryColumn(t *testing.T) {
	tests := []struct {
		name   string
		update string
	}{
		{name: "status", update: "status = 'failed'"},
		{name: "total tokens", update: "total_tokens = total_tokens + 1"},
		{name: "input tokens", update: "input_tokens = 99"},
		{name: "output tokens", update: "output_tokens = 99"},
		{name: "cache read tokens", update: "cache_read_tokens = 99"},
		{name: "duration", update: "duration_ms = 99"},
		{name: "danger", update: "danger = NOT danger"},
		{name: "detail", update: "detail = 'edited'"},
		{name: "session model", update: "session_model = 'claude-haiku-4-5'"},
		{name: "session source", update: "session_source = 'settings'"},
		{name: "confidence", update: "confidence = 99"},
		{name: "margin", update: "margin = 99"},
		{name: "codex", update: "codex = 'task'"},
		{name: "continuation", update: "continuation = 1"},
		{name: "language", update: "lang = 'en'"},
		{name: "advised", update: "advised = 1"},
		{name: "pressure", update: "pressure_high = 1"},
		{name: "subagent type", update: "subagent_type = 'Plan'"},
		{name: "mismatch", update: "mismatch = NULL"},
		{name: "nested", update: "nested = 1"},
		{name: "previous hash", update: "prev_hash = 'x'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := open(t)
			repo := persistence.NewLedgerRepo(db)
			o := model.SubagentOutcome{
				ToolUseID: "t", SubagentType: "general-purpose", Requested: "haiku", Resolved: "claude-haiku-4-5", Status: "completed",
				TotalTokens: 10, InputTokens: 1, OutputTokens: 2, CacheReadTokens: 3, DurationMS: 4,
			}
			for i, e := range []model.Entry{
				model.NewEntry(t0, model.EventPrompt),
				model.NewEntry(t0.Add(time.Second), model.EventSubagentResult, model.WithOutcome(o), model.WithDetail("d")),
			} {
				if err := repo.Append(t.Context(), e); err != nil {
					t.Fatalf("append %d: %v", i, err)
				}
			}
			exec(t, db, "UPDATE ledger SET "+tt.update+" WHERE id = 2")
			_, broken, err := repo.Verify(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(uint(2), broken); diff != "" {
				t.Errorf("broken row (-want +got):\n%s", diff)
			}
		})
	}
}

// TestOpenEscapesThePath: the path was pasted into a `file:` URI, so a
// directory name with `?` or `#` opened (and created) another file.
func TestOpenEscapesThePath(t *testing.T) {
	tests := []struct {
		name string
		dir  string
	}{
		{name: "question mark", dir: "a?mode=ro"},
		{name: "hash and percent", dir: "b#c %41"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := filepath.Join(t.TempDir(), tt.dir)
			path := filepath.Join(parent, "ledger.db")
			repo := persistence.NewLedgerRepo(openAt(t, path, 10))
			if err := repo.Append(t.Context(), model.NewEntry(t0, model.EventPrompt)); err != nil {
				t.Fatal(err)
			}
			n := 0
			for _, err := range persistence.NewLedgerRepo(openAt(t, path, 10)).Since(t.Context(), time.Time{}) {
				if err != nil {
					t.Fatal(err)
				}
				n++
			}
			var names []string
			entries, err := os.ReadDir(filepath.Dir(parent))
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range entries {
				names = append(names, e.Name())
			}
			if diff := cmp.Diff([]any{1, []string{tt.dir}}, []any{n, names}); diff != "" {
				t.Errorf("rows and files beside the data directory (-want +got):\n%s", diff)
			}
		})
	}
}

// legacyLedger writes a ledger the way a release before hash forms did: rows
// hashed in HashV1 and no hash_version column. It returns the path.
func legacyLedger(t *testing.T, rows int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ledger.db")
	db, err := persistence.Open(path, persistence.Options{BusyTimeoutMS: 1000, Batch: 10})
	if err != nil {
		t.Fatal(err)
	}
	repo := persistence.NewLedgerRepo(db)
	for i := range rows {
		if err := repo.Append(t.Context(), model.NewEntry(t0.Add(time.Duration(i)*time.Second), model.EventPrompt, model.WithDetail("a|b"))); err != nil {
			t.Fatal(err)
		}
	}
	var all []persistence.EntryRow
	if err := db.Gorm().Order("id").Find(&all).Error; err != nil {
		t.Fatal(err)
	}
	prev := ""
	for i := range all {
		r := &all[i]
		r.PrevHash, r.Hash = prev, persistence.LegacyRowHash(prev, *r)
		prev = r.Hash
		if err := db.Gorm().Model(r).Updates(map[string]any{"prev_hash": r.PrevHash, "hash": r.Hash}).Error; err != nil {
			t.Fatal(err)
		}
	}
	exec(t, db, "ALTER TABLE ledger DROP COLUMN hash_version")
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestLedgerHashVersions: an upgraded binary keeps verifying the rows an older
// release wrote, in the form they were written in.
func TestLedgerHashVersions(t *testing.T) {
	type result struct {
		Checked int
		Broken  uint
	}
	tests := []struct {
		name   string
		tamper string // SQL run after the upgrade and one new row
		rehash bool   // recompute the last row's hash in its (tampered) form
		second bool   // append a second new row
		want   result
	}{
		{name: "old rows verify in the old form and new rows chain onto them", want: result{Checked: 3}},
		{name: "an edit an old row's form covers is detected", tamper: "UPDATE ledger SET detail = 'x' WHERE id = 1", want: result{Broken: 1}},
		{name: "an edit to an old row's tokens is detected", tamper: "UPDATE ledger SET total_tokens = 99 WHERE id = 1", want: result{Broken: 1}},
		{
			// The downgraded row hashes correctly in HashV1 (which does not
			// cover status): only a HashV1 row after a HashV2 row gives the
			// edit away. (The first HashV2 row follows HashV1 rows, so its
			// downgrade is not detectable this way.)
			name: "a downgraded row is detected even with a matching old hash", rehash: true, second: true,
			tamper: "UPDATE ledger SET hash_version = 1, status = 'failed' WHERE id = 4",
			want:   result{Checked: 3, Broken: 4},
		},
		{
			name: "a new row cannot be downgraded to the old form", tamper: "UPDATE ledger SET hash_version = 1, status = 'failed' WHERE id = 3",
			want: result{Checked: 2, Broken: 3},
		},
		{name: "an unknown form breaks the chain", tamper: "UPDATE ledger SET hash_version = 9 WHERE id = 3", want: result{Checked: 2, Broken: 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := legacyLedger(t, 2)
			db := openAt(t, path, 10)
			repo := persistence.NewLedgerRepo(db)
			wantVersions := []int{persistence.HashV1, persistence.HashV1, persistence.HashV2}
			if err := repo.Append(t.Context(), model.NewEntry(t0.Add(time.Minute), model.EventPrompt)); err != nil {
				t.Fatal(err)
			}
			if tt.second {
				if err := repo.Append(t.Context(), model.NewEntry(t0.Add(2*time.Minute), model.EventPrompt)); err != nil {
					t.Fatal(err)
				}
				wantVersions = append(wantVersions, persistence.HashV2)
			}
			var versions []int
			if err := db.Gorm().Model(&persistence.EntryRow{}).Order("id").Pluck("hash_version", &versions).Error; err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(wantVersions, versions); diff != "" {
				t.Errorf("hash forms (-want +got):\n%s", diff)
			}
			if tt.tamper != "" {
				exec(t, db, tt.tamper)
			}
			if tt.rehash {
				var last persistence.EntryRow
				if err := db.Gorm().Order("id desc").First(&last).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.Gorm().Model(&last).Update("hash", persistence.LegacyRowHash(last.PrevHash, last)).Error; err != nil {
					t.Fatal(err)
				}
			}
			checked, broken, err := repo.Verify(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.want, result{checked, broken}); diff != "" {
				t.Errorf("Verify() (-want +got):\n%s", diff)
			}
		})
	}
}

// TestLedgerFeed: positions, not timestamps, say what is new. The SSE feed
// used to resume from "time of the last entry + 1ns", and the stored text
// ("…00.12345Z") compares above that bound ("…00.123450001Z"), so the same
// rows came back on every poll.
func TestLedgerFeed(t *testing.T) {
	trailingZero := time.Date(2026, 10, 1, 12, 0, 0, 123450000, time.UTC)
	tests := []struct {
		name     string
		at       []time.Time
		batch    int
		pos      uint
		from     time.Time
		take     int // stop after this many (0 = all)
		breakDB  bool
		wantHead uint
		want     []uint
		wantErr  bool
	}{
		{name: "an empty ledger has head 0", wantHead: 0},
		{
			name: "after the last position nothing repeats, whatever the timestamps", at: []time.Time{trailingZero, trailingZero.Truncate(time.Second)},
			batch: 1, pos: 2, wantHead: 2,
		},
		{
			name: "insertion order, across batches, even when timestamps go back", at: []time.Time{t0.Add(time.Hour), t0, t0.Add(time.Minute)},
			batch: 2, wantHead: 3, want: []uint{1, 2, 3},
		},
		{name: "the time bound filters", at: []time.Time{t0, t0.Add(time.Hour)}, batch: 5, from: t0.Add(time.Minute), wantHead: 2, want: []uint{2}},
		{name: "the consumer may stop early", at: []time.Time{t0, t0}, batch: 5, take: 1, wantHead: 2, want: []uint{1}},
		{name: "a query error is yielded", at: []time.Time{t0}, batch: 5, breakDB: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openAt(t, filepath.Join(t.TempDir(), "ledger.db"), tt.batch)
			repo := persistence.NewLedgerRepo(db)
			for _, at := range tt.at {
				if err := repo.Append(t.Context(), model.NewEntry(at, model.EventPrompt)); err != nil {
					t.Fatal(err)
				}
			}
			if tt.breakDB {
				exec(t, db, "DROP TABLE ledger")
			}
			head, headErr := repo.Head(t.Context())
			var (
				got    []uint
				gotErr error
			)
			for p, err := range repo.After(t.Context(), tt.pos, tt.from) {
				if err != nil {
					gotErr = err
					break
				}
				if !p.Entry.At.Equal(tt.at[p.Pos-1]) {
					t.Errorf("entry %d at %v, want %v", p.Pos, p.Entry.At, tt.at[p.Pos-1])
				}
				got = append(got, p.Pos)
				if tt.take > 0 && len(got) == tt.take {
					break
				}
			}
			if (gotErr != nil) != tt.wantErr || (headErr != nil) != tt.wantErr {
				t.Fatalf("errors = %v, %v; wantErr %v", headErr, gotErr, tt.wantErr)
			}
			if diff := cmp.Diff([]any{tt.wantHead, tt.want}, []any{head, got}); diff != "" {
				t.Errorf("head and positions (-want +got):\n%s", diff)
			}
		})
	}
}

// TestLedgerMismatchRoundTrip: the tri-state is stored as a nullable boolean
// and reads back as the same value (never a pointer shared with the entry).
func TestLedgerMismatchRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		o    model.SubagentOutcome
		want model.Mismatch
	}{
		{name: "unknown", o: model.SubagentOutcome{Requested: "haiku"}, want: model.MismatchUnknown},
		{name: "no", o: model.SubagentOutcome{Requested: "haiku", Resolved: "claude-haiku-4-5"}, want: model.MismatchNo},
		{name: "yes", o: model.SubagentOutcome{Requested: "haiku", Resolved: "claude-opus-5-5"}, want: model.MismatchYes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := persistence.NewLedgerRepo(open(t))
			if err := repo.Append(t.Context(), model.NewEntry(t0, model.EventSubagentResult, model.WithOutcome(tt.o))); err != nil {
				t.Fatal(err)
			}
			var got []model.Mismatch
			for e, err := range repo.Since(t.Context(), time.Time{}) {
				if err != nil {
					t.Fatal(err)
				}
				got = append(got, e.Mismatch)
			}
			if diff := cmp.Diff([]model.Mismatch{tt.want}, got); diff != "" {
				t.Errorf("(-want +got):\n%s", diff)
			}
		})
	}
}
