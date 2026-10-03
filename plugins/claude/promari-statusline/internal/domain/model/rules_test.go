package model_test

import (
	"strings"
	"testing"

	"promari-statusline/internal/domain/model"
)

// lint runs the added lines of one file, a "-" line standing for a deleted
// line between them.
func lint(file string, lines ...string) model.Violations {
	var l model.Lint
	l.File(file)
	for _, line := range lines {
		if line == "-" {
			l.Gap()
			continue
		}
		l.Added(line)
	}
	l.Done()
	return l.Found
}

func TestLintFindsTheRules(t *testing.T) {
	t.Parallel()
	long := "        var message = \"" + strings.Repeat("あ", 60) + "\";"
	tests := []struct {
		name  string
		file  string
		lines []string
		rule  model.Rule
		want  int
	}{
		{"SQL joined with a value", "Repo.cs", []string{`var sql = "SELECT * FROM Users WHERE Id = " + id;`}, model.RuleSQLConcat, 1},
		{"SQL interpolated", "Repo.cs", []string{`var sql = $"DELETE FROM Users WHERE Id = {id}";`}, model.RuleSQLConcat, 1},
		{"FromSqlRaw interpolated", "Repo.cs", []string{`db.Users.FromSqlRaw($"SELECT * FROM Users WHERE Name = '{name}'");`}, model.RuleSQLConcat, 1},
		{"SQL in Go joined", "repo.go", []string{`q := "UPDATE users SET name = '" + name + "'"`}, model.RuleSQLConcat, 1},
		{"SQL with a parameter is fine", "Repo.cs", []string{`var sql = "SELECT * FROM Users WHERE Id = @id";`}, model.RuleSQLConcat, 0},
		{"a table name joined to parameterized SQL", "persist.go", []string{`"DELETE FROM "+tblOrders+" WHERE order_id = ? AND company_id = ?",`}, model.RuleSQLConcat, 0},
		{"a vendored library is not judged", "theme/js/libs/imagesloaded.js", []string{`var $ = window.jQuery;`}, model.RuleJSVar, 0},
		{"a parameter beside SQL", "lab.py", []string{`con.execute("insert into ev(v) values (?)", (f"v{i}",))`}, model.RuleSQLConcat, 0},
		{"SQL formatted with %", "lab.py", []string{`cur.execute("SELECT * FROM t WHERE id = %s" % uid)`}, model.RuleSQLConcat, 1},
		{"SQL in a test", "repo_test.go", []string{`exec(t, db, "UPDATE ledger SET "+tt.update+" WHERE id = 2")`}, model.RuleSQLConcat, 0},
		{"a log line is not SQL", "Repo.cs", []string{`Console.WriteLine("[LOG] select " + name);`}, model.RuleSQLConcat, 0},

		{"DELETE without WHERE", "seed.sql", []string{`DELETE FROM employees;`}, model.RuleNoWhere, 1},
		{"UPDATE without WHERE in a string", "Repo.cs", []string{`cmd.CommandText = "UPDATE employees SET active = 0;";`}, model.RuleNoWhere, 1},
		{"UPDATE with WHERE", "seed.sql", []string{`UPDATE employees SET active = 0 WHERE id = 3;`}, model.RuleNoWhere, 0},
		{"UPDATE with WHERE after an escaped newline", "fig.py", []string{`T(F, "UPDATE t SET a = 1\nWHERE id = 8;")`}, model.RuleNoWhere, 0},
		{"a test empties its table", "OrderTests.cs", []string{`await db.Database.ExecuteSqlRawAsync("DELETE FROM Orders;", ct);`}, model.RuleNoWhere, 0},
		{"UPDATE whose WHERE follows", "seed.sql", []string{`UPDATE employees SET active = 0`, `WHERE id = 3;`}, model.RuleNoWhere, 0},

		{"= NULL", "query.sql", []string{`SELECT * FROM t WHERE deleted_at = NULL;`}, model.RuleNullCompare, 1},
		{"IS NULL", "query.sql", []string{`SELECT * FROM t WHERE deleted_at IS NULL;`}, model.RuleNullCompare, 0},
		{"SET = NULL", "query.sql", []string{`UPDATE t SET deleted_at = NULL WHERE id = 1;`}, model.RuleNullCompare, 0},

		{"connection string", "Program.cs", []string{`var cs = "Server=(localdb)\\mssqllocaldb;Database=Shop;Trusted_Connection=True";`}, model.RuleSecret, 1},
		{"password literal", "Seed.cs", []string{`password = "Passw0rd!";`}, model.RuleSecret, 1},
		{"token in a remote", "setup.sh", []string{"git remote add origin https://user:abc123@" + "github.com/o/r.git"}, model.RuleSecret, 1},
		{"a placeholder", "Seed.cs", []string{`password = "<your password>";`}, model.RuleSecret, 0},
		{"a test may hold a password", "Seed_test.go", []string{`password := "Passw0rd!"`}, model.RuleSecret, 0},

		{"throw ex", "Svc.cs", []string{`    throw ex;`}, model.RuleThrowEx, 1},
		{"throw on", "Svc.cs", []string{`    throw;`, `    throw new InvalidOperationException("x");`}, model.RuleThrowEx, 0},

		{"empty catch on one line", "Svc.cs", []string{`catch (IOException) { }`}, model.RuleEmptyCatch, 1},
		{"empty catch over lines", "Svc.cs", []string{`    catch (IOException e)`, `    {`, `    }`}, model.RuleEmptyCatch, 1},
		{"a catch that says why it ignores", "Svc.cs", []string{`    catch (IOException e)`, `    {`, `        // the file may be gone already`, `    }`}, model.RuleEmptyCatch, 0},
		{"a catch named ignored", "Svc.java", []string{`    try { Thread.sleep(ms); } catch (InterruptedException ignored) { }`}, model.RuleEmptyCatch, 0},
		{"any catch of TypeScript takes everything", "api.ts", []string{`  } catch (e) {`, `    console.warn(e);`, `  }`}, model.RuleCatchAll, 0},
		{"empty catch in TypeScript", "api.ts", []string{`  } catch (e) {`, `  }`}, model.RuleEmptyCatch, 1},
		{"a catch that logs", "Svc.cs", []string{`    catch (IOException e)`, `    {`, `        _log.Warn(e);`, `    }`}, model.RuleEmptyCatch, 0},
		{"a catch cut by a deleted line", "Svc.cs", []string{`    catch (IOException e)`, `    {`, "-", `    }`}, model.RuleEmptyCatch, 0},

		{"catch Exception and go on", "Svc.cs", []string{`    catch (Exception e)`, `    {`, `        Console.WriteLine(e);`, `    }`}, model.RuleCatchAll, 1},
		{"catch everything and go on", "Svc.cs", []string{`    catch`, `    {`, `        Console.WriteLine("x");`, `    }`}, model.RuleCatchAll, 1},
		{"catch Exception and throw on", "Svc.cs", []string{`    catch (Exception e)`, `    {`, `        _log.Error(e);`, `        throw;`, `    }`}, model.RuleCatchAll, 0},
		{"catch a specific exception", "Svc.cs", []string{`    catch (FormatException e)`, `    {`, `        return null;`, `    }`}, model.RuleCatchAll, 0},

		{"POST without a token", "HomeController.cs", []string{`    [HttpPost]`, `    public IActionResult Save(Form f)`}, model.RuleNoCSRF, 1},
		{"POST with a token", "HomeController.cs", []string{`    [HttpPost]`, `    [ValidateAntiForgeryToken]`}, model.RuleNoCSRF, 0},
		{"POST of an API", "UsersController.cs", []string{`[ApiController]`, `    [HttpPost]`}, model.RuleNoCSRF, 0},

		{"Html.Raw of a value", "Index.cshtml", []string{`@Html.Raw(Model.Body)`}, model.RuleHTMLRaw, 1},
		{"Html.Raw of a literal", "Index.cshtml", []string{`@Html.Raw("<hr />")`}, model.RuleHTMLRaw, 0},
		{"singleton DbContext", "Program.cs", []string{`builder.Services.AddSingleton<ApplicationDbContext>();`}, model.RuleSingletonDB, 1},
		{"scoped DbContext", "Program.cs", []string{`builder.Services.AddDbContext<ApplicationDbContext>();`}, model.RuleSingletonDB, 0},

		{"JS var", "app.js", []string{`var total = 0;`}, model.RuleJSVar, 1},
		{"C# var is fine", "App.cs", []string{`var total = 0;`}, model.RuleJSVar, 0},
		{"a bundle is not judged", "dist/app.user.js", []string{`var chrome = {};`}, model.RuleJSVar, 0},
		{"JS let", "app.js", []string{`let total = 0;`}, model.RuleJSVar, 0},
		{"<br> twice", "index.html", []string{`<p>a<br /><br />b</p>`}, model.RuleBrRun, 1},
		{"<br> once", "index.html", []string{`<p>a<br />b</p>`}, model.RuleBrRun, 0},

		{"lower-case class", "item.cs", []string{`public class item`}, model.RuleNaming, 1},
		{"interface without I", "Repo.cs", []string{`public interface Repository`}, model.RuleNaming, 1},
		{"exception without its suffix", "Err.cs", []string{`public class StockError : Exception`}, model.RuleNaming, 1},
		{"ApplicationException", "Err.cs", []string{`public class StockException : ApplicationException`}, model.RuleNaming, 1},
		{"a Flg", "A.cs", []string{`    bool closeFlg = false;`}, model.RuleNaming, 1},
		{"a vague local", "A.cs", []string{`    string temp = name;`}, model.RuleNaming, 1},
		{"good names", "A.cs", []string{`public class Item`, `public interface IRepository`, `public class StockException : Exception`, `    bool isClosed = false;`, `    public bool DeleteFlag { get; set; }`}, model.RuleNaming, 0},
		{"a migration is not judged", "Migrations/20260401_Init.cs", []string{`public partial class init : Migration`}, model.RuleNaming, 0},

		{"if without braces", "A.cs", []string{`    if (x > 0) return x;`}, model.RuleNoBraces, 1},
		{"else without braces", "A.cs", []string{`    else count = 0;`}, model.RuleNoBraces, 1},
		{"TypeScript guards are not judged", "a.ts", []string{`    if (!x) return;`}, model.RuleNoBraces, 0},
		{"if with braces", "A.cs", []string{`    if (x > 0) { return x; }`, `    if (x > 0)`, `    else if (y) {`}, model.RuleNoBraces, 0},
		{"a for with its parts", "A.cs", []string{`    for (int i = 0; i < n; i++)`}, model.RuleNoBraces, 0},

		{"a public method without ///", "Svc.cs", []string{`    }`, `    public int Count(string name)`}, model.RuleNoDoc, 1},
		{"a public type with /// before its attribute", "Svc.cs", []string{`/// <summary>Orders.</summary>`, `[ApiController]`, `public class OrdersController : Controller`}, model.RuleNoDoc, 0},
		{"a public property with ///", "Item.cs", []string{`    /// <summary>The name.</summary>`, `    public string Name { get; set; }`}, model.RuleNoDoc, 0},
		{"an override takes its base's documentation", "Item.cs", []string{`    }`, `    public override string ToString()`}, model.RuleNoDoc, 0},
		{"the first line of a hunk is not judged", "Item.cs", []string{`    public int Count { get; }`}, model.RuleNoDoc, 0},
		{"a private member needs none", "Item.cs", []string{`    }`, `    private int count;`}, model.RuleNoDoc, 0},
		{"SQL in a string that goes on to the next line", "Repo.cs", []string{`var sql = @"SELECT name FROM users`}, model.RuleSQLConcat, 0},
		{"a promise's catch is not a block", "api.js", []string{`fetchIt().catch(e => log(e));`}, model.RuleEmptyCatch, 0},
		{"catch-all on one line", "Svc.cs", []string{`    catch (Exception e) { Log(e); }`}, model.RuleCatchAll, 1},
		{"catch-all on one line that throws on", "Svc.cs", []string{`    catch (Exception e) { Log(e); throw; }`}, model.RuleCatchAll, 0},
		{"a block that throws on its first line", "Svc.cs", []string{`    catch (Exception e) { Log(e); throw;`, `    }`}, model.RuleCatchAll, 0},
		{"a catch without braces is not followed", "Svc.swift", []string{`    } catch`, `    log(error)`, `    }`}, model.RuleEmptyCatch, 0},
		{"a long line", "A.cs", []string{long}, model.RuleLongLine, 1},
		{"a long line of Go is not judged", "a.go", []string{long}, model.RuleLongLine, 0},
		{"a tab in C#", "A.cs", []string{"\tint count = 0;"}, model.RuleTabIndent, 1},
		{"a tab in Go", "a.go", []string{"\tcount := 0"}, model.RuleTabIndent, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := lint(tt.file, tt.lines...)[tt.rule]; got != tt.want {
				t.Errorf("rule %d = %d, want %d", tt.rule, got, tt.want)
			}
		})
	}
}

func TestLintKeepsFilesApart(t *testing.T) {
	t.Parallel()
	var l model.Lint
	l.File("A.cs")
	l.Added("    catch (IOException e)")
	l.Added("    {")
	l.File("B.cs")
	l.Added("    }")
	l.Done()
	if got := l.Found.Total(); got != 0 {
		t.Errorf("a catch block carried over to another file: %v", l.Found)
	}
}

func TestLineCells(t *testing.T) {
	t.Parallel()
	if got := model.LineCells("ab漢字\t"); got != 2+4+4 {
		t.Errorf("LineCells = %d", got)
	}
}

// BenchmarkLint checks MaxLintedLines lines of C#, the most a render reads.
func BenchmarkLint(b *testing.B) {
	sample := []string{
		`        public async Task<IActionResult> Index(int? page)`,
		`        {`,
		`            var items = await _context.Items.Where(i => i.Price > 0).ToListAsync();`,
		`            if (items.Count == 0) { return NotFound(); }`,
		`            var sql = "SELECT * FROM Items WHERE Id = @id";`,
		`            try`,
		`            {`,
		`                Console.WriteLine($"[LOG] {items.Count} items");`,
		`            }`,
		`            catch (DbUpdateException e)`,
		`            {`,
		`                _logger.LogError(e, "保存に失敗しました");`,
		`            }`,
		`            return View(items);`,
		`        }`,
	}
	b.ReportAllocs()
	for b.Loop() {
		var l model.Lint
		l.File("Controllers/ItemsController.cs")
		for i := range model.MaxLintedLines {
			l.Added(sample[i%len(sample)])
		}
		l.Done()
	}
}

func TestLintStopsAtItsLimit(t *testing.T) {
	t.Parallel()
	var l model.Lint
	l.File("Big.cs")
	for range model.MaxLintedLines + 3 {
		l.Added("    throw ex;")
	}
	l.Done()
	if l.Found[model.RuleThrowEx] != model.MaxLintedLines || l.Linted != model.MaxLintedLines || l.Skipped != 3 {
		t.Errorf("found %d, linted %d, skipped %d", l.Found[model.RuleThrowEx], l.Linted, l.Skipped)
	}
}
