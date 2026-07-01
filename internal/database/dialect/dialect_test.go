package dialect

import "testing"

func TestJSONTextExprByDialect(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
		want    string
	}{
		{name: "postgres", dialect: "postgres", want: "attributes->>'license'"},
		{name: "mysql", dialect: "mysql", want: "JSON_UNQUOTE(JSON_EXTRACT(attributes, '$.license'))"},
		{name: "sqlite", dialect: "sqlite", want: "JSON_EXTRACT(attributes, '$.license')"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := JSONTextExpr(tt.dialect, "attributes", "license"); got != tt.want {
				t.Fatalf("JSONTextExpr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestColumnExistsSQLByDialect(t *testing.T) {
	for _, dialectName := range []string{"sqlite", "postgres", "mysql"} {
		t.Run(dialectName, func(t *testing.T) {
			query, args, err := ColumnExistsQuery(dialectName, "artifacts", "coordinates")
			if err != nil {
				t.Fatalf("ColumnExistsQuery() error = %v", err)
			}
			if query == "" {
				t.Fatal("expected non-empty query")
			}
			if len(args) != 2 {
				t.Fatalf("args length = %d, want 2", len(args))
			}
		})
	}
}

func TestColumnExistsSQLRejectsUnsupportedDialect(t *testing.T) {
	if _, _, err := ColumnExistsQuery("oracle", "artifacts", "coordinates"); err == nil {
		t.Fatal("expected unsupported dialect error")
	}
}

func TestDropColumnSQLByDialect(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
		want    string
	}{
		{name: "sqlite", dialect: "sqlite", want: "ALTER TABLE artifacts DROP COLUMN coordinates"},
		{name: "postgres", dialect: "postgres", want: "ALTER TABLE artifacts DROP COLUMN IF EXISTS coordinates"},
		{name: "mysql", dialect: "mysql", want: "ALTER TABLE artifacts DROP COLUMN coordinates"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DropColumnSQL(tt.dialect, "artifacts", "coordinates")
			if err != nil {
				t.Fatalf("DropColumnSQL() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("DropColumnSQL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDropColumnSQLRejectsUnsupportedDialect(t *testing.T) {
	if _, err := DropColumnSQL("oracle", "artifacts", "coordinates"); err == nil {
		t.Fatal("expected unsupported dialect error")
	}
}

func TestSQLiteDSNWithPragmas(t *testing.T) {
	got, err := SQLiteDSNWithPragmas("./data/registry.db")
	if err != nil {
		t.Fatalf("SQLiteDSNWithPragmas() error = %v", err)
	}
	want := "./data/registry.db?_journal_mode=WAL&_busy_timeout=30000&_txlock=immediate"
	if got != want {
		t.Fatalf("SQLiteDSNWithPragmas() = %q, want %q", got, want)
	}

	got, err = SQLiteDSNWithPragmas("file:test.db?cache=shared")
	if err != nil {
		t.Fatalf("SQLiteDSNWithPragmas() error = %v", err)
	}
	want = "file:test.db?cache=shared&_journal_mode=WAL&_busy_timeout=30000&_txlock=immediate"
	if got != want {
		t.Fatalf("SQLiteDSNWithPragmas() = %q, want %q", got, want)
	}
}
