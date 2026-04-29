package schema

import (
	"maps"
	"slices"
	"testing"

	"github.com/pseudomuto/housekeeper/pkg/parser"
	"github.com/stretchr/testify/require"
)

// TestReconcileClusters verifies the live-vs-source ON CLUSTER reconciliation
// applied uniformly across tables, views, dictionaries, databases, functions,
// roles and grants.
func TestReconcileClusters(t *testing.T) {
	type item struct {
		Cluster string
	}
	get := func(i *item) string { return i.Cluster }
	set := func(i *item, c string) { i.Cluster = c }

	cases := []struct {
		name    string
		current map[string]*item
		target  map[string]*item
		want    map[string]string // expected cluster after reconciliation, by key
	}{
		{
			name:    "macro source rewrites resolved live name",
			current: map[string]*item{"a": {Cluster: "default"}, "b": {Cluster: "default"}},
			target:  map[string]*item{"a": {Cluster: "'{cluster}'"}, "b": {Cluster: "'{cluster}'"}},
			want:    map[string]string{"a": "'{cluster}'", "b": "'{cluster}'"},
		},
		{
			name:    "non-clustered live items are not touched",
			current: map[string]*item{"a": {Cluster: ""}, "b": {Cluster: "default"}},
			target:  map[string]*item{"a": {Cluster: "'{cluster}'"}, "b": {Cluster: "'{cluster}'"}},
			want:    map[string]string{"a": "", "b": "'{cluster}'"},
		},
		{
			name:    "no inferable target cluster is a no-op",
			current: map[string]*item{"a": {Cluster: "x"}, "b": {Cluster: "y"}},
			target:  map[string]*item{"a": {Cluster: "x"}, "b": {Cluster: "y"}},
			want:    map[string]string{"a": "x", "b": "y"},
		},
		{
			name:    "empty target leaves current untouched",
			current: map[string]*item{"a": {Cluster: "default"}},
			target:  map[string]*item{},
			want:    map[string]string{"a": "default"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ReconcileClusters(maps.Values(tc.current), maps.Values(tc.target), get, set)
			for k, want := range tc.want {
				require.Equal(t, want, tc.current[k].Cluster, "key=%s", k)
			}
		})
	}
}

// TestReconcileClusters_Slice exercises the iter.Seq helper against a slice
// (the shape used for grants).
func TestReconcileClusters_Slice(t *testing.T) {
	type item struct{ Cluster string }
	current := []*item{{Cluster: "default"}, {Cluster: ""}, {Cluster: "default"}}
	target := []*item{{Cluster: "'{cluster}'"}}

	ReconcileClusters(slices.Values(current), slices.Values(target),
		func(i *item) string { return i.Cluster },
		func(i *item, c string) { i.Cluster = c },
	)

	require.Equal(t, "'{cluster}'", current[0].Cluster)
	require.Equal(t, "", current[1].Cluster, "non-clustered item must be left alone")
	require.Equal(t, "'{cluster}'", current[2].Cluster)
}

func TestNormalizeDefaultDatabase(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"default", ""},
		{"`default`", ""},
		{"my_db", "my_db"},
		{"`my_db`", "my_db"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			require.Equal(t, tc.want, normalizeDefaultDatabase(tc.in))
		})
	}
}

// TestGetViewTableTargetValue_DefaultDB ensures the implicit `default` database
// is stripped from MV TO clauses so live extraction matches unqualified source.
func TestGetViewTableTargetValue_DefaultDB(t *testing.T) {
	tbl := "market_item_intensity"
	defDB := "default"
	otherDB := "analytics"
	backtickedDef := "`default`"

	cases := []struct {
		name   string
		target *parser.ViewTableTarget
		want   string
	}{
		{"nil target", nil, ""},
		{"unqualified table", &parser.ViewTableTarget{Table: &tbl}, "market_item_intensity"},
		{"default db stripped", &parser.ViewTableTarget{Database: &defDB, Table: &tbl}, "market_item_intensity"},
		{"backticked default db stripped", &parser.ViewTableTarget{Database: &backtickedDef, Table: &tbl}, "market_item_intensity"},
		{"non-default db kept", &parser.ViewTableTarget{Database: &otherDB, Table: &tbl}, "analytics.market_item_intensity"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, getViewTableTargetValue(tc.target))
		})
	}
}
