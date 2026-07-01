package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeywordFuzzyPattern(t *testing.T) {
	pattern, ok := KeywordFuzzyPattern("  abc  ")
	require.True(t, ok)
	require.Equal(t, "%abc%", pattern)

	_, ok = KeywordFuzzyPattern("   ")
	require.False(t, ok)
}

func TestKeywordOrFilter_Build(t *testing.T) {
	UsingPostgreSQL = false
	UsingSQLite = true

	filter := NewKeywordOrFilter()
	filter.Text("work_no")
	filter.CastText("id")
	filter.SubqueryIn("user_id", "users", "id", "email", "username")

	require.Equal(t, "(work_no LIKE ? OR CAST(id AS TEXT) LIKE ? OR user_id IN (SELECT id FROM users WHERE email LIKE ? OR username LIKE ?))", filter.WhereSQL())
	require.Equal(t, []interface{}{"%E10086%", "%E10086%", "%E10086%", "%E10086%"}, filter.Args("%E10086%"))
}

func TestKeywordOrFilter_PostgreSQLTextLike(t *testing.T) {
	UsingPostgreSQL = true
	UsingSQLite = false
	require.Equal(t, "work_no ILIKE ?", SQLTextLike("work_no"))
}
