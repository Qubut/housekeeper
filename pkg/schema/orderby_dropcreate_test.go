package schema_test

import (
	"strings"
	"testing"

	"github.com/pseudomuto/housekeeper/pkg/parser"
	"github.com/pseudomuto/housekeeper/pkg/schema"
	"github.com/stretchr/testify/require"
)

func TestOrderByChangeUsesDropCreate(t *testing.T) {
	curSQL := `CREATE TABLE listings (
    created_at DateTime64(6),
    category String,
    name String,
    kind String,
    version UInt64
) ENGINE = ReplicatedReplacingMergeTree(version)
ORDER BY (category, name, kind, created_at);`
	tgtSQL := `CREATE TABLE listings (
    created_at DateTime64(6),
    item_id UInt64,
    kind String,
    version UInt64
) ENGINE = ReplicatedReplacingMergeTree(version)
ORDER BY (item_id, kind, created_at);`

	cur, err := parser.ParseString(curSQL)
	require.NoError(t, err)
	tgt, err := parser.ParseString(tgtSQL)
	require.NoError(t, err)

	diff, err := schema.GenerateDiff(cur, tgt)
	require.NoError(t, err)

	var buf strings.Builder
	for _, stmt := range diff.Statements {
		if stmt.DropTable != nil {
			buf.WriteString("DROP;")
		}
		if stmt.CreateTable != nil {
			buf.WriteString("CREATE;")
		}
		if stmt.AlterTable != nil {
			buf.WriteString("ALTER;")
		}
	}
	require.Contains(t, buf.String(), "DROP;", "expected DROP+CREATE, got %s", buf.String())
	require.Contains(t, buf.String(), "CREATE;")
	require.NotContains(t, buf.String(), "ALTER;")
}
