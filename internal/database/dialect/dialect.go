package dialect

import (
	"fmt"
	"strings"
)

const SQLitePragmas = "_journal_mode=WAL&_busy_timeout=30000&_txlock=immediate"

func JSONTextExpr(dialectName, column, key string) string {
	switch dialectName {
	case "postgres":
		return column + "->>'" + key + "'"
	case "mysql":
		return "JSON_UNQUOTE(JSON_EXTRACT(" + column + ", '$." + key + "'))"
	default:
		return "JSON_EXTRACT(" + column + ", '$." + key + "')"
	}
}

func ColumnExistsQuery(dialectName, tableName, columnName string) (string, []interface{}, error) {
	switch dialectName {
	case "sqlite":
		return "SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", []interface{}{tableName, columnName}, nil
	case "postgres":
		return `
				SELECT COUNT(*)
				FROM information_schema.columns
				WHERE table_schema = current_schema()
				  AND table_name = ?
				  AND column_name = ?
			`, []interface{}{tableName, columnName}, nil
	case "mysql":
		return `
				SELECT COUNT(*)
				FROM information_schema.columns
				WHERE table_schema = DATABASE()
				  AND table_name = ?
				  AND column_name = ?
			`, []interface{}{tableName, columnName}, nil
	default:
		return "", nil, fmt.Errorf("unsupported database dialect: %s", dialectName)
	}
}

func DropColumnSQL(dialectName, tableName, columnName string) (string, error) {
	switch dialectName {
	case "sqlite", "mysql":
		return fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", tableName, columnName), nil
	case "postgres":
		return fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s", tableName, columnName), nil
	default:
		return "", fmt.Errorf("unsupported database dialect: %s", dialectName)
	}
}

func SQLiteDSNWithPragmas(dsn string) (string, error) {
	if strings.Contains(dsn, "?") {
		return dsn + "&" + SQLitePragmas, nil
	}
	return dsn + "?" + SQLitePragmas, nil
}
