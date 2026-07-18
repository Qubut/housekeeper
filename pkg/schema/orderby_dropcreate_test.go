package schema_test

import (
	"strings"
	"testing"

	"github.com/pseudomuto/housekeeper/pkg/parser"
	"github.com/pseudomuto/housekeeper/pkg/schema"
	"github.com/stretchr/testify/require"
)

func TestOrderByChangeUsesDropCreate(t *testing.T) {
	curSQL := `CREATE TABLE buff163_market_listings (
    created_at DateTime64(6),
    game String,
    market_hash_name String,
    order_type String,
    version UInt64
) ENGINE = ReplicatedReplacingMergeTree(version)
ORDER BY (game, market_hash_name, order_type, created_at);`
	tgtSQL := `CREATE TABLE buff163_market_listings (
    created_at DateTime64(6),
    item_id UInt64,
    order_type String,
    version UInt64
) ENGINE = ReplicatedReplacingMergeTree(version)
ORDER BY (item_id, order_type, created_at);`

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
