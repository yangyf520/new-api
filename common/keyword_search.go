package common

import (
	"strings"

	"gorm.io/gorm"
)

// KeywordFuzzyPattern wraps a trimmed keyword as "%keyword%" for LIKE/ILIKE matching.
func KeywordFuzzyPattern(keyword string) (pattern string, ok bool) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return "", false
	}
	return "%" + keyword + "%", true
}

// SQLTextLike returns a column LIKE/ILIKE placeholder clause (case-insensitive on PostgreSQL).
func SQLTextLike(column string) string {
	if UsingPostgreSQL {
		return column + " ILIKE ?"
	}
	return column + " LIKE ?"
}

// SQLCastTextLike returns CAST(column AS TEXT/CHAR) LIKE ? for fuzzy id/numeric search.
func SQLCastTextLike(column string) string {
	if UsingPostgreSQL || UsingSQLite {
		return "CAST(" + column + " AS TEXT) LIKE ?"
	}
	return "CAST(" + column + " AS CHAR) LIKE ?"
}

// SQLUnixTimestampLike returns a formatted unix-epoch column LIKE/ILIKE clause.
func SQLUnixTimestampLike(column string) string {
	if UsingPostgreSQL {
		return "to_char(to_timestamp(" + column + "), 'YYYY-MM-DD HH24:MI:SS') ILIKE ?"
	}
	if UsingSQLite {
		return "strftime('%Y-%m-%d %H:%M:%S', " + column + ", 'unixepoch') LIKE ?"
	}
	return "FROM_UNIXTIME(" + column + ", '%Y-%m-%d %H:%i:%s') LIKE ?"
}

// KeywordOrFilter collects OR-combined LIKE clauses for keyword search.
type KeywordOrFilter struct {
	clauses []string
	counts  []int
}

// NewKeywordOrFilter creates an empty keyword OR filter builder.
func NewKeywordOrFilter() *KeywordOrFilter {
	return &KeywordOrFilter{}
}

// Text adds a text/varchar column fuzzy match.
func (f *KeywordOrFilter) Text(column string) {
	f.clauses = append(f.clauses, SQLTextLike(column))
	f.counts = append(f.counts, 1)
}

// CastText adds a CAST-to-text fuzzy match for numeric columns.
func (f *KeywordOrFilter) CastText(column string) {
	f.clauses = append(f.clauses, SQLCastTextLike(column))
	f.counts = append(f.counts, 1)
}

// UnixTimestamp adds a formatted unix timestamp fuzzy match.
func (f *KeywordOrFilter) UnixTimestamp(column string) {
	f.clauses = append(f.clauses, SQLUnixTimestampLike(column))
	f.counts = append(f.counts, 1)
}

// Expr appends a custom SQL fragment with the given placeholder count.
func (f *KeywordOrFilter) Expr(sql string, placeholderCount int) {
	if strings.TrimSpace(sql) == "" || placeholderCount <= 0 {
		return
	}
	f.clauses = append(f.clauses, sql)
	f.counts = append(f.counts, placeholderCount)
}

// SubqueryIn adds: outerColumn IN (SELECT idColumn FROM table WHERE col1 LIKE ? OR ...).
func (f *KeywordOrFilter) SubqueryIn(outerColumn, table, idColumn string, matchColumns ...string) {
	if len(matchColumns) == 0 {
		return
	}
	parts := make([]string, len(matchColumns))
	for i, col := range matchColumns {
		parts[i] = SQLTextLike(col)
	}
	sql := outerColumn + " IN (SELECT " + idColumn + " FROM " + table + " WHERE " + strings.Join(parts, " OR ") + ")"
	f.clauses = append(f.clauses, sql)
	f.counts = append(f.counts, len(matchColumns))
}

// Empty reports whether the filter has no clauses.
func (f *KeywordOrFilter) Empty() bool {
	return len(f.clauses) == 0
}

// WhereSQL returns "(clause1 OR clause2 OR ...)".
func (f *KeywordOrFilter) WhereSQL() string {
	return "(" + strings.Join(f.clauses, " OR ") + ")"
}

// Args repeats pattern for each placeholder in the filter.
func (f *KeywordOrFilter) Args(pattern string) []interface{} {
	total := 0
	for _, count := range f.counts {
		total += count
	}
	args := make([]interface{}, total)
	for i := range args {
		args[i] = pattern
	}
	return args
}

// ApplyKeywordOrFilter applies a keyword OR filter to a GORM query when keyword is non-empty.
func ApplyKeywordOrFilter(db *gorm.DB, keyword string, configure func(*KeywordOrFilter)) *gorm.DB {
	pattern, ok := KeywordFuzzyPattern(keyword)
	if !ok {
		return db
	}
	filter := NewKeywordOrFilter()
	configure(filter)
	if filter.Empty() {
		return db
	}
	return db.Where(filter.WhereSQL(), filter.Args(pattern)...)
}
