package model

import (
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// The coding rules a training course for new engineers teaches, checked on the
// lines a change adds. The course teaches C#, ASP.NET Core MVC, SQL Server,
// JavaScript and HTML, so most rules apply to those files only; the rules about
// SQL built from strings and secrets written into code apply to every source
// file. Only rules that one added line (or a catch block within one hunk) can
// decide are checked, and only those the course states as a rule, not the
// habits its own samples happen to show.
const (
	// MaxLineCells is the longest line the course allows: 120 columns, a wide
	// character counting as two.
	MaxLineCells = 120
	// MaxLintedLines bounds the added lines checked per render: a generated
	// file of tens of thousands of lines would otherwise slow every render.
	MaxLintedLines = 5000
)

// Rule is one rule of the course that an added line can break.
type Rule int

// The rules, the most serious first.
const (
	RuleSQLConcat   Rule = iota // SQL built by joining or interpolating strings
	RuleNoWhere                 // UPDATE or DELETE without WHERE
	RuleSecret                  // a password, connection string or token in code
	RuleThrowEx                 // "throw ex;", which loses the stack trace
	RuleEmptyCatch              // a catch block with nothing in it
	RuleCatchAll                // catching Exception without throwing it on
	RuleNoCSRF                  // a POST action without an anti-forgery token
	RuleHTMLRaw                 // Html.Raw of a value, an opening for XSS
	RuleSingletonDB             // a DbContext registered as a singleton
	RuleNullCompare             // "= NULL" in SQL, which is never true
	RuleJSVar                   // var in JavaScript
	RuleBrRun                   // <br> repeated to make space
	RuleNaming                  // a name the naming rules forbid
	RuleNoBraces                // if, for or while without braces
	RuleNoDoc                   // a public C# member without an XML doc comment
	RuleLongLine                // a line longer than 120 columns
	RuleTabIndent               // a tab used to indent C#
	// RuleCount is the number of rules: a table indexed by Rule has this length,
	// so a rule added without its entry is caught by the table's test.
	RuleCount
)

// Violations counts the broken rules of the added lines.
type Violations [RuleCount]int

// Total returns the number of broken rules.
func (v Violations) Total() int {
	total := 0
	for _, n := range v {
		total += n
	}
	return total
}

// The kinds of file the rules tell apart.
var (
	csharpFiles = []string{".cs"}
	razorFiles  = []string{".cshtml", ".razor"}
	jsFiles     = []string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx"}
	markupFiles = []string{".html", ".htm", ".cshtml", ".razor", ".vue", ".jsx", ".tsx"}
	sqlFiles    = []string{".sql"}
	// catchFiles are the languages with try and catch blocks; typedCatchFiles
	// those whose catch names the exceptions it takes, so that catching
	// everything is a choice.
	catchFiles      = []string{".cs", ".java", ".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".kt", ".scala", ".php", ".swift"}
	typedCatchFiles = []string{".cs", ".java", ".kt", ".scala", ".php"}
	// courseFiles are the languages the course teaches, whose layout rules
	// (line length, braces) apply.
	courseFiles = []string{".cs", ".cshtml", ".razor", ".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".html", ".htm", ".css", ".sql"}
)

// generatedPath matches files a tool writes: bundles, vendored code, EF Core
// migrations and designer files. Their lines are no one's to fix.
var generatedPath = regexp.MustCompile(`(?:^|/)(?:dist|build|out|vendor|node_modules|generated|Migrations|libs|third[_-]?party)/|\.min\.(?:js|css)$|\.(?:g|g\.i|designer|Designer|generated)\.cs$`)

func hasExt(file string, exts []string) bool {
	ext := strings.ToLower(path.Ext(file))
	return slices.Contains(exts, ext)
}

var (
	// sqlString matches a string literal that starts an SQL statement.
	sqlString = regexp.MustCompile(`(?i)["'` + "`" + `]\s*(?:SELECT\s.+\sFROM|INSERT\s+INTO|UPDATE\s+\w[\w.\[\]]*\s+SET|DELETE\s+FROM)\b`)
	// placeholder marks a parameter in SQL: ?, @name or $1. A statement that
	// takes its values as parameters joins only names, such as a table's.
	sqlParameter = regexp.MustCompile(`\?|@[A-Za-z_]\w*|\$\d+`)
	sqlRawCalls  = regexp.MustCompile(`\b(?:FromSqlRaw|ExecuteSqlRaw|ExecuteSqlRawAsync|SqlQueryRaw)\s*\(\s*\$`)

	// noWhere matches an UPDATE or DELETE statement that ends on its own line.
	noWhere = regexp.MustCompile(`(?i)\b(?:UPDATE\s+\w[\w.\[\]]*\s+SET\s|DELETE\s+FROM\s+\w[\w.\[\]]*)[^;]*;\s*["'` + "`" + `]?\s*[),;]*\s*$`)
	// where also finds a WHERE after an escaped newline ("...\nWHERE").
	where = regexp.MustCompile(`(?i)WHERE\b`)

	// nullCompare matches "= NULL" or "<> NULL" in a condition.
	nullCompare = regexp.MustCompile(`(?i)\b(?:WHERE|AND|OR|ON)\b.*?(?:[^=!<>]=|<>|!=)\s*NULL\b`)

	// secret matches a connection string, a password or a token written into
	// code.
	secret = regexp.MustCompile(`(?i)\b(?:Server|Data Source)\s*=[^;"]+;.*\b(?:Database|Initial Catalog)\s*=|\b(?:password|passwd|pwd)\s*[:=]\s*["'][^"'\s]{3,}["']|https://[^/@\s:"']+:[^@\s"']+@github\.com|\bgh[ps]_[A-Za-z0-9]{30,}|\bgithub_pat_[A-Za-z0-9_]{30,}`)
	// placeholder matches a value that only stands for a secret.
	placeholder = regexp.MustCompile(`(?i)["'](?:\*+|x+|\.+|<[^>]*>|\$\{[^}]*\}|%[^%]*%|changeme|password|your[_-]?\w*|dummy|example\w*|sample\w*|test\w*)["']`)

	throwEx = regexp.MustCompile(`^\s*throw\s+[a-z]\w*\s*;`)

	// catchHead matches the start of a catch block and tells whether it catches
	// everything: no type, Exception or SystemException.
	catchHead = regexp.MustCompile(`\bcatch\b\s*(?:\(\s*([\w.]*)\s*(\w*)[^)]*\))?\s*(\{.*)?$`)
	// ignoredName is a caught exception named to say it is ignored on purpose.
	ignoredName = regexp.MustCompile(`^(?:_|ignored?|unused|expected)$`)
	catchAll    = map[string]bool{"": true, "Exception": true, "System.Exception": true, "SystemException": true, "System.SystemException": true, "Throwable": true}
	emptyBody   = regexp.MustCompile(`^\{\s*\}`)

	httpPost      = regexp.MustCompile(`\[\s*HttpPost\b`)
	antiForgery   = regexp.MustCompile(`\b(?:Validate|AutoValidate)AntiForgeryToken\b|\bIgnoreAntiforgeryToken\b`)
	apiController = regexp.MustCompile(`\[\s*ApiController\b`)
	htmlRaw       = regexp.MustCompile(`@?Html\.Raw\(\s*[^"\s)]`)
	singletonDB   = regexp.MustCompile(`\bAddSingleton\s*<\s*\w*(?:DbContext|Context)\s*[,>]`)
	// publicMember matches the declaration of a public type or member;
	// overrides take the documentation of what they override.
	publicMember   = regexp.MustCompile(`^\s*public\s+(?:(?:static|sealed|abstract|partial|virtual|async|readonly|new|required|unsafe|extern|const)\s+)*(?:class|interface|record|struct|enum|[\w<>\[\],?.]+\s+\w+\s*(?:\(|\{|=>|=|;|$))`)
	jsVar          = regexp.MustCompile(`^\s*var\s+[A-Za-z_$]`)
	brRun          = regexp.MustCompile(`(?i)<br\s*/?>\s*<br\s*/?>`)
	noBraces       = regexp.MustCompile(`^\s*(?:(?:else\s+)?if|for|foreach|while)\s*\(.*\)\s*[^\s{;/].*;\s*$|^\s*else\s+(?:[^\s{i/]|i[^f]).*;\s*$`)
	lowerType      = regexp.MustCompile(`\b(?:class|struct|record|enum|interface)\s+[a-z_]\w*`)
	badInterface   = regexp.MustCompile(`\binterface\s+(?:[^I\s]\w*|I[^A-Z\s]\w*)\b`)
	exceptionClass = regexp.MustCompile(`\bclass\s+(\w+)\s*:\s*(?:System\.)?(?:Application)?Exception\b`)
	appException   = regexp.MustCompile(`:\s*(?:System\.)?ApplicationException\b`)
	flagName       = regexp.MustCompile(`\bbool\??\s+\w+Flg\b`)
	vagueLocal     = regexp.MustCompile(`^\s*(?:var|int|long|double|decimal|string|bool|object)\s+(?:temp|tmp|data|info|str|buf)\s*[=;]`)
)

// Lint checks the added lines of a change, one file after another. A catch
// block and the POST actions of a controller span several lines, so the state
// of the current file is kept between lines.
type Lint struct {
	Found Violations
	// Linted are the added lines checked; Skipped those left unchecked past
	// MaxLintedLines.
	Linted, Skipped int

	file string
	// What the file is, decided once per file.
	judged, test, csharp, js, markup, razor, sql, course, catches, typed bool
	// The open catch block: its indentation and whether a line inside it has
	// said or thrown anything yet.
	catchOpen   bool
	catchAll    bool
	catchIndent string
	catchBody   bool
	catchThrows bool
	catchBraced bool
	// The last added line of C# that is not an attribute or blank, and whether
	// it is known: a hunk's first line has no known line before it.
	prevCode  string
	prevKnown bool
	// The POST actions, anti-forgery tokens and API controller marks the file
	// adds.
	posts, tokens int
	api           bool
}

// File starts the added lines of another file.
func (l *Lint) File(file string) {
	l.flushFile()
	l.file = file
	l.prevKnown = false
	l.course = hasExt(file, courseFiles)
	l.judged = (IsSourcePath(file) || l.course) && !generatedPath.MatchString(file)
	l.test = IsTestPath(file)
	l.csharp, l.js, l.markup = hasExt(file, csharpFiles), hasExt(file, jsFiles), hasExt(file, markupFiles)
	l.razor, l.sql = hasExt(file, razorFiles), hasExt(file, sqlFiles)
	l.catches, l.typed = hasExt(file, catchFiles), hasExt(file, typedCatchFiles)
}

// Gap marks lines between hunks: a block open across them cannot be judged.
func (l *Lint) Gap() { l.catchOpen, l.prevKnown = false, false }

// Done ends the change.
func (l *Lint) Done() { l.flushFile() }

func (l *Lint) flushFile() {
	if !l.api && l.posts > l.tokens {
		l.Found[RuleNoCSRF] += l.posts - l.tokens
	}
	l.posts, l.tokens, l.api = 0, 0, false
	l.catchOpen = false
}

// Added checks one added line.
func (l *Lint) Added(line string) {
	if !l.judged {
		return
	}
	if l.Linted >= MaxLintedLines {
		l.Skipped++
		return
	}
	l.Linted++
	lower := strings.ToLower(line)
	l.checkCatch(line)
	if strings.Contains(lower, "select") || strings.Contains(lower, "insert") || strings.Contains(lower, "update") ||
		strings.Contains(lower, "delete") || strings.Contains(lower, "null") || strings.Contains(lower, "sqlraw") {
		l.checkSQL(line)
	}
	if !l.test && mayHoldSecret(lower) && secret.MatchString(line) && !placeholder.MatchString(line) {
		l.Found[RuleSecret]++
	}
	if l.course && len(line) > MaxLineCells/2 && LineCells(line) > MaxLineCells {
		l.Found[RuleLongLine]++
	}
	switch {
	case l.csharp:
		l.checkCSharp(line)
	case l.js && strings.Contains(line, "var ") && jsVar.MatchString(line):
		l.Found[RuleJSVar]++
	}
	if l.markup && strings.Contains(lower, "<br") && brRun.MatchString(line) {
		l.Found[RuleBrRun]++
	}
	if l.razor && strings.Contains(line, "Html.Raw") && htmlRaw.MatchString(line) {
		l.Found[RuleHTMLRaw]++
	}
}

// mayHoldSecret is a cheap test that lets most lines skip the secret pattern.
func mayHoldSecret(lower string) bool {
	return strings.Contains(lower, "server") || strings.Contains(lower, "data source") || strings.Contains(lower, "pass") ||
		strings.Contains(lower, "pwd") || strings.Contains(lower, "github")
}

func (l *Lint) checkSQL(line string) {
	// A test builds SQL from its own values and empties its own tables.
	if l.test {
		return
	}
	if sqlBuilt(line) || sqlRawCalls.MatchString(line) {
		l.Found[RuleSQLConcat]++
	}
	sql := l.sql || sqlString.MatchString(line)
	if sql && noWhere.MatchString(line) && !where.MatchString(line) {
		l.Found[RuleNoWhere]++
	}
	if l.sql && nullCompare.MatchString(line) {
		l.Found[RuleNullCompare]++
	}
}

// sqlBuilt reports whether the string literal that holds an SQL statement is
// itself built from values: interpolated ($"...{x}", f"...{x}", `...${x}`),
// joined with + or formatted with % or .format. A value passed beside the
// statement as a parameter is not part of it.
func sqlBuilt(line string) bool {
	loc := sqlString.FindStringIndex(line)
	if loc == nil {
		return false
	}
	open := loc[0]
	quote := line[open]
	end := closingQuote(line, open)
	body := line[open+1 : end]
	before := strings.TrimRight(line[:open], " ")
	after := strings.TrimLeft(line[min(end+1, len(line)):], " ")
	interpolated := strings.HasSuffix(before, "$") || strings.HasSuffix(before, "$@") || strings.HasSuffix(before, "@$") ||
		strings.HasSuffix(before, "f") || strings.HasSuffix(before, "F") || quote == '`'
	switch {
	case interpolated && strings.Contains(body, "{"):
		return true
	case strings.HasSuffix(before, "+") || strings.HasPrefix(after, "+"):
		return !sqlParameter.MatchString(line)
	case strings.HasPrefix(after, "% ") || strings.HasPrefix(after, "%(") || strings.HasPrefix(after, ".format("):
		return true
	}
	return false
}

// closingQuote returns the index of the quote that closes the string opened at
// open, or the end of the line when it continues.
func closingQuote(line string, open int) int {
	quote := line[open]
	for i := open + 1; i < len(line); i++ {
		switch line[i] {
		case '\\':
			i++
		case quote:
			return i
		}
	}
	return len(line)
}

func (l *Lint) checkCSharp(line string) {
	if strings.HasPrefix(line, "\t") {
		l.Found[RuleTabIndent]++
	}
	code := stripComment(line)
	head := strings.TrimLeft(code, " \t")
	if hasAnyPrefix(head, "if", "else", "for", "while") && noBraces.MatchString(code) {
		l.Found[RuleNoBraces]++
	}
	if strings.HasPrefix(head, "throw") && throwEx.MatchString(code) {
		l.Found[RuleThrowEx]++
	}
	if strings.HasPrefix(head, "[") {
		if httpPost.MatchString(code) {
			l.posts++
		}
		if antiForgery.MatchString(code) {
			l.tokens++
		}
		if apiController.MatchString(code) {
			l.api = true
		}
	}
	if strings.Contains(code, "AddSingleton") && singletonDB.MatchString(code) {
		l.Found[RuleSingletonDB]++
	}
	l.checkDoc(strings.TrimSpace(line))
	if namingBroken(code) {
		l.Found[RuleNaming]++
	}
}

// checkDoc counts a public type or member whose line before, attributes and
// blank lines aside, is not an XML documentation comment ("///"). The first
// line of a hunk is not judged: the line before it was not changed and is not
// in the diff.
func (l *Lint) checkDoc(head string) {
	if head == "" || strings.HasPrefix(head, "[") {
		return
	}
	if l.prevKnown && strings.HasPrefix(head, "public ") && !strings.Contains(head, " override ") &&
		publicMember.MatchString(head) && !strings.HasPrefix(l.prevCode, "///") {
		l.Found[RuleNoDoc]++
	}
	l.prevCode, l.prevKnown = head, true
}

// namingBroken reports whether a C# line declares a name the naming rules
// forbid: a type in lower case, an interface without the I prefix, an
// exception without the Exception suffix or derived from ApplicationException,
// a bool named "...Flg", or a local named temp, data, info, str or buf.
func namingBroken(code string) bool {
	declares := strings.Contains(code, "class ") || strings.Contains(code, "interface ") || strings.Contains(code, "struct ") ||
		strings.Contains(code, "record ") || strings.Contains(code, "enum ") || strings.Contains(code, "bool")
	vague := strings.Contains(code, " temp") || strings.Contains(code, " tmp") || strings.Contains(code, " data") ||
		strings.Contains(code, " info") || strings.Contains(code, " str") || strings.Contains(code, " buf")
	if !declares && !vague {
		return false
	}
	if m := exceptionClass.FindStringSubmatch(code); len(m) > 1 && !strings.HasSuffix(m[1], "Exception") {
		return true
	}
	return lowerType.MatchString(code) || badInterface.MatchString(code) || appException.MatchString(code) ||
		flagName.MatchString(code) || vagueLocal.MatchString(code)
}

// checkCatch follows a catch block over the lines of a hunk. A block is empty
// when it closes before a line with anything in it; it swallows everything
// when it catches Exception (or anything) and closes without throwing on.
func (l *Lint) checkCatch(line string) {
	if !l.catches {
		return
	}
	code := strings.TrimSpace(stripComment(line))
	if l.catchOpen {
		l.followCatch(line, code)
		return
	}
	if !strings.Contains(code, "catch") {
		return
	}
	m := catchHead.FindStringSubmatch(code)
	if m == nil || strings.Contains(code, "=>") {
		return
	}
	// Every catch of a language without typed catches takes everything.
	all := catchAll[m[1]] && l.typed
	ignored := ignoredName.MatchString(m[2]) || strings.Contains(line, "//") || strings.Contains(line, "/*")
	rest := strings.TrimSpace(m[3])
	switch {
	case emptyBody.MatchString(rest) && !ignored:
		l.Found[RuleEmptyCatch]++
	case emptyBody.MatchString(rest):
	case rest != "" && strings.HasSuffix(rest, "}"):
		// The whole block is on one line.
		if all && !strings.Contains(rest, "throw") {
			l.Found[RuleCatchAll]++
		}
	default:
		l.catchOpen, l.catchAll, l.catchBody, l.catchThrows = true, all, ignored || rest != "{" && rest != "", false
		l.catchBraced = rest != ""
		l.catchIndent = indentOf(line)
		if strings.Contains(rest, "throw") {
			l.catchThrows = true
		}
	}
}

func (l *Lint) followCatch(line, code string) {
	switch {
	case !l.catchBraced && code == "{":
		l.catchBraced = true
	case !l.catchBraced:
		// A catch without braces is not C-like; stop following it.
		l.catchOpen = false
	case code == "}" && indentOf(line) == l.catchIndent:
		l.catchOpen = false
		switch {
		case !l.catchBody:
			l.Found[RuleEmptyCatch]++
		case l.catchAll && !l.catchThrows:
			l.Found[RuleCatchAll]++
		}
	case code == "" && strings.TrimSpace(line) != "":
		// A comment says why the exception is ignored: the block is not
		// empty, though it still swallows what it catches.
		l.catchBody = true
	case code != "":
		l.catchBody = true
		if strings.Contains(code, "throw") {
			l.catchThrows = true
		}
	}
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// stripComment drops a trailing // comment that is not inside a string.
func stripComment(line string) string {
	inString := byte(0)
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case inString != 0:
			switch c {
			case '\\':
				i++
			case inString:
				inString = 0
			}
		case c == '"' || c == '\'' || c == '`':
			inString = c
		case c == '/' && i+1 < len(line) && line[i+1] == '/':
			return line[:i]
		}
	}
	return line
}

func indentOf(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// LineCells returns the columns a line takes, a wide character counting as
// two and a tab as four.
func LineCells(line string) int {
	cells := 0
	for _, r := range line {
		switch {
		case r == '\t':
			cells += 4
		case isWide(r):
			cells += 2
		default:
			cells++
		}
	}
	return cells
}

func isWide(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) ||
		(r >= 0xFF01 && r <= 0xFF60) || (r >= 0xFFE0 && r <= 0xFFE6) || (r >= 0x3000 && r <= 0x303F)
}
