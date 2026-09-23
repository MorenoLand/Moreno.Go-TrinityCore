package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	columnTypePattern   = regexp.MustCompile(`(?is)^([a-z]+)(\s*\([^)]*\))?`)
	routinePattern      = regexp.MustCompile(`(?is)^CREATE\s+(?:OR\s+REPLACE\s+)?(?:DEFINER\s*=\s*\S+\s+)?(?:PROCEDURE|FUNCTION|TRIGGER|EVENT)\b`)
	noisePattern        = regexp.MustCompile(`(?i)\b(UNSIGNED|ZEROFILL|AUTO_INCREMENT)\b`)
	charsetPattern      = regexp.MustCompile(`(?i)\b(?:CHARACTER\s+SET|CHARSET)\s+[a-zA-Z0-9_]+`)
	collationPattern    = regexp.MustCompile(`(?i)\bCOLLATE\s+[a-zA-Z0-9_]+`)
	commentPattern      = regexp.MustCompile(`(?is)\bCOMMENT\s+'(?:''|\\.|[^'])*'`)
	onUpdatePattern     = regexp.MustCompile(`(?i)\bON\s+UPDATE\s+(?:CURRENT_TIMESTAMP(?:\(\))?|[A-Z_]+)`)
	usingPattern        = regexp.MustCompile(`(?i)\bUSING\s+(?:BTREE|HASH)`)
	bitPattern          = regexp.MustCompile(`(?i)\bb'([01]+)'`)
	viewPrefixPattern   = regexp.MustCompile(`(?is)^CREATE\s+(?:ALGORITHM\s*=\s*[^\s]+\s+)?(?:DEFINER\s*=\s*[^\s]+\s+)?(?:SQL\s+SECURITY\s+(?:DEFINER|INVOKER)\s+)?VIEW\s+`)
	viewModifierPattern = regexp.MustCompile(`(?is)\s+(?:ALGORITHM\s*=\s*[^\s]+|DEFINER\s*=\s*[^\s]+|SQL\s+SECURITY\s+(?:DEFINER|INVOKER))`)
)

func NormalizeSQLiteCreateTable(statement string) ([]string, error) {
	open := findOpeningParen(statement)
	if open < 0 {
		return nil, errors.New("CREATE TABLE has no definition")
	}
	close := matchingParen(statement, open)
	if close < 0 {
		return nil, errors.New("CREATE TABLE has an unterminated definition")
	}
	match := tableNamePattern.FindStringSubmatch(statement)
	if len(match) != 2 {
		return nil, errors.New("CREATE TABLE has no table name")
	}
	table := match[1]
	logicalTable := strings.Trim(table, "`\"")
	definitions := splitTopLevel(statement[open+1 : close])
	if len(definitions) == 0 {
		return nil, fmt.Errorf("table %s has no columns", table)
	}
	columns := make([]string, 0, len(definitions))
	indexes := make([]string, 0)
	for _, definition := range definitions {
		definition = strings.TrimSpace(definition)
		upper := strings.ToUpper(definition)
		switch {
		case strings.HasPrefix(upper, "KEY "), strings.HasPrefix(upper, "INDEX "), strings.HasPrefix(upper, "UNIQUE KEY "), strings.HasPrefix(upper, "UNIQUE INDEX "):
			index, err := normalizeIndex(definition, table)
			if err != nil {
				return nil, err
			}
			indexes = append(indexes, index)
		case strings.HasPrefix(upper, "FULLTEXT "), strings.HasPrefix(upper, "SPATIAL "):
			definition = regexp.MustCompile(`(?is)^(?:FULLTEXT|SPATIAL)\s+`).ReplaceAllString(definition, "")
			index, err := normalizeIndex(definition, table)
			if err != nil {
				return nil, err
			}
			indexes = append(indexes, index)
		case strings.HasPrefix(upper, "PRIMARY KEY"), strings.HasPrefix(upper, "CONSTRAINT "):
			columns = append(columns, normalizeConstraint(definition))
		case strings.HasPrefix(upper, "CHECK "):
			columns = append(columns, normalizeConstraint(definition))
		default:
			if logicalTable == "character_aura" || logicalTable == "pet_aura" {
				name, rest := firstToken(definition)
				if strings.EqualFold(strings.Trim(name, "`\""), "casterGuid") {
					if typeMatch := columnTypePattern.FindStringSubmatch(strings.TrimSpace(rest)); len(typeMatch) > 0 {
						definition = name + " TEXT " + strings.TrimSpace(rest[len(typeMatch[0]):])
					}
				}
			}
			column, err := normalizeColumn(definition)
			if err != nil {
				return nil, fmt.Errorf("table %s: %w", table, err)
			}
			columns = append(columns, column)
		}
	}
	var b strings.Builder
	b.WriteString("CREATE TABLE IF NOT EXISTS ")
	b.WriteString(table)
	b.WriteString(" (\n")
	b.WriteString(strings.Join(columns, ",\n"))
	b.WriteString("\n)")
	result := []string{b.String()}
	result = append(result, indexes...)
	return result, nil
}

func NormalizeSQLiteAlterTable(statement string) []string {
	upper := strings.ToUpper(statement)
	if strings.Contains(upper, "DISABLE KEYS") || strings.Contains(upper, "ENABLE KEYS") || strings.Contains(upper, "AUTO_INCREMENT") {
		return nil
	}
	open := findOpeningParen(statement)
	if open < 0 {
		return nil
	}
	match := regexp.MustCompile(`(?is)^ALTER\s+TABLE\s+([^\s]+)`).FindStringSubmatch(statement)
	if len(match) != 2 {
		return nil
	}
	table := match[1]
	nameMatch := regexp.MustCompile(`(?is)^ALTER\s+TABLE\s+[^\s]+\s+ADD\s+(?:UNIQUE\s+)?(?:KEY|INDEX)\s+([^\s(]+)`).FindStringSubmatch(statement)
	if len(nameMatch) != 2 {
		return nil
	}
	unique := regexp.MustCompile(`(?is)^ALTER\s+TABLE\s+[^\s]+\s+ADD\s+UNIQUE`).MatchString(statement)
	close := matchingParen(statement, open)
	if close < 0 {
		return nil
	}
	prefix := "CREATE INDEX IF NOT EXISTS "
	if unique {
		prefix = "CREATE UNIQUE INDEX IF NOT EXISTS "
	}
	return []string{prefix + qualifiedIndexName(table, nameMatch[1]) + " ON " + table + " (" + normalizeIndexColumns(statement[open+1:close]) + ")"}
}

func NormalizeSQLiteInsert(statement string) (string, error) {
	upper := strings.ToUpper(statement)
	if strings.Contains(upper, "ON DUPLICATE KEY UPDATE") {
		return "", errors.New("ON DUPLICATE KEY UPDATE requires a dialect-specific statement")
	}
	statement = regexp.MustCompile(`(?is)^INSERT\s+IGNORE\s+INTO\s+`).ReplaceAllString(statement, "INSERT OR IGNORE INTO ")
	statement = regexp.MustCompile(`(?is)^INSERT\s+(?:LOW_PRIORITY\s+)?INTO\s+`).ReplaceAllString(statement, "INSERT INTO ")
	return normalizeMySQLLiterals(statement), nil
}

func NormalizeSchemaScript(script, dialect string) (string, error) {
	var out strings.Builder
	out.WriteString("-- schema-only template\n")
	skipRoutine := false
	for _, statement := range SplitSQL(script) {
		upper := strings.ToUpper(strings.TrimSpace(statement))
		if skipRoutine {
			if routineEnd(upper) {
				skipRoutine = false
			}
			continue
		}
		if routineCreate(upper) {
			skipRoutine = true
			continue
		}
		converted, err := NormalizeSchemaStatement(statement, dialect)
		if err != nil {
			return "", err
		}
		for _, part := range converted {
			if part = strings.TrimSpace(part); part != "" {
				out.WriteString(part)
				out.WriteString(";\n")
			}
		}
	}
	return out.String(), nil
}

func NormalizeSchemaStatement(statement, dialect string) ([]string, error) {
	statement = strings.TrimSpace(statement)
	if statement == "" {
		return nil, nil
	}
	upper := strings.ToUpper(statement)
	switch {
	case strings.HasPrefix(upper, "CREATE TABLE"):
		if dialect == "sqlite" {
			return NormalizeSQLiteCreateTable(statement)
		}
		return []string{normalizeMySQLCreateTable(statement)}, nil
	case strings.HasPrefix(upper, "ALTER TABLE"):
		if dialect == "sqlite" {
			return NormalizeSQLiteAlterTable(statement), nil
		}
		if strings.Contains(upper, "DISABLE KEYS") || strings.Contains(upper, "ENABLE KEYS") {
			return nil, nil
		}
		return []string{statement}, nil
	case strings.HasPrefix(upper, "CREATE INDEX"), strings.HasPrefix(upper, "CREATE UNIQUE INDEX"):
		return []string{statement}, nil
	case strings.HasPrefix(upper, "CREATE VIEW") || strings.HasPrefix(upper, "CREATE ALGORITHM") || strings.HasPrefix(upper, "CREATE DEFINER"):
		if strings.Contains(upper, " PROCEDURE ") || strings.Contains(upper, " FUNCTION ") || strings.Contains(upper, " TRIGGER ") || strings.Contains(upper, " EVENT ") {
			return nil, nil
		}
		return []string{normalizeView(statement)}, nil
	case strings.HasPrefix(upper, "CREATE PROCEDURE"), strings.HasPrefix(upper, "CREATE FUNCTION"), strings.HasPrefix(upper, "CREATE TRIGGER"), strings.HasPrefix(upper, "CREATE EVENT"):
		return nil, nil
	default:
		return nil, nil
	}
}

func ImportSQLiteDump(ctx context.Context, inputPath, outputPath string, force bool) error {
	if !force {
		if _, err := os.Stat(outputPath); err == nil {
			return fmt.Errorf("output already exists: %s", outputPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else if err := replaceSQLiteOutput(outputPath); err != nil {
		return err
	}
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", outputPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	skipRoutine := false
	for _, statement := range SplitSQL(string(data)) {
		upper := strings.ToUpper(strings.TrimSpace(statement))
		if routineCreate(upper) {
			skipRoutine = true
			continue
		}
		if skipRoutine {
			if routineEnd(upper) {
				skipRoutine = false
			}
			continue
		}
		var parts []string
		switch {
		case strings.HasPrefix(upper, "CREATE TABLE"):
			parts, err = NormalizeSQLiteCreateTable(statement)
		case strings.HasPrefix(upper, "ALTER TABLE"):
			parts = NormalizeSQLiteAlterTable(statement)
		case strings.HasPrefix(upper, "INSERT"):
			var part string
			part, err = NormalizeSQLiteInsert(statement)
			if part != "" {
				parts = []string{part}
			}
		case strings.HasPrefix(upper, "REPLACE INTO"):
			parts = []string{normalizeMySQLLiterals(statement)}
		case strings.HasPrefix(upper, "CREATE VIEW") || strings.HasPrefix(upper, "CREATE ALGORITHM") || strings.HasPrefix(upper, "CREATE DEFINER"):
			if !strings.Contains(upper, " PROCEDURE ") && !strings.Contains(upper, " FUNCTION ") && !strings.Contains(upper, " TRIGGER ") && !strings.Contains(upper, " EVENT ") {
				parts = []string{normalizeView(statement)}
			}
		}
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("%s: %w", inputPath, err)
		}
		for _, part := range parts {
			if strings.TrimSpace(part) == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, part); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("%s: %w", inputPath, err)
			}
		}
	}
	return tx.Commit()
}

func replaceSQLiteOutput(outputPath string) error {
	if info, err := os.Stat(outputPath); err == nil {
		if info.IsDir() {
			return fmt.Errorf("SQLite output is a directory: %s", outputPath)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if err := os.Remove(outputPath + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove existing SQLite output %s: %w", outputPath+suffix, err)
		}
	}
	return nil
}

func routineCreate(statement string) bool {
	return routinePattern.MatchString(strings.TrimSpace(statement))
}

func routineEnd(statement string) bool {
	statement = strings.TrimSpace(statement)
	return statement == "END" || strings.HasPrefix(statement, "END ")
}

func normalizeColumn(definition string) (string, error) {
	name, rest := firstToken(definition)
	if name == "" || rest == "" {
		return "", errors.New("invalid column definition")
	}
	match := columnTypePattern.FindStringSubmatch(strings.TrimSpace(rest))
	if len(match) == 0 {
		return "", errors.New("column has no type")
	}
	originalType := match[0]
	tail := strings.TrimSpace(rest[len(originalType):])
	typeName := strings.ToLower(match[1])
	mapped := "TEXT"
	switch typeName {
	case "tinyint", "smallint", "mediumint", "int", "integer", "bigint", "bit", "bool", "boolean", "year":
		mapped = "INTEGER"
	case "float", "double", "decimal", "numeric", "real":
		mapped = "REAL"
	case "blob", "tinyblob", "mediumblob", "longblob", "binary", "varbinary":
		mapped = "BLOB"
	}
	tail = noisePattern.ReplaceAllString(tail, "")
	tail = charsetPattern.ReplaceAllString(tail, "")
	tail = collationPattern.ReplaceAllString(tail, "")
	tail = commentPattern.ReplaceAllString(tail, "")
	tail = onUpdatePattern.ReplaceAllString(tail, "")
	tail = usingPattern.ReplaceAllString(tail, "")
	tail = bitPattern.ReplaceAllString(tail, "$1")
	return strings.TrimSpace(name + " " + mapped + " " + tail), nil
}

func normalizeConstraint(definition string) string {
	definition = usingPattern.ReplaceAllString(definition, "")
	definition = charsetPattern.ReplaceAllString(definition, "")
	definition = collationPattern.ReplaceAllString(definition, "")
	return strings.TrimSpace(definition)
}

func normalizeIndex(definition, table string) (string, error) {
	unique := regexp.MustCompile(`(?is)^UNIQUE\s+`).MatchString(definition)
	match := regexp.MustCompile(`(?is)^(?:UNIQUE\s+)?(?:KEY|INDEX)\s+([^\s(]+)`).FindStringSubmatch(definition)
	if len(match) != 2 {
		return "", errors.New("index has no name")
	}
	open := findOpeningParen(definition)
	if open < 0 {
		return "", errors.New("index has no columns")
	}
	close := matchingParen(definition, open)
	if close < 0 {
		return "", errors.New("index has an unterminated column list")
	}
	prefix := "CREATE INDEX IF NOT EXISTS "
	if unique {
		prefix = "CREATE UNIQUE INDEX IF NOT EXISTS "
	}
	return prefix + qualifiedIndexName(table, match[1]) + " ON " + table + " (" + normalizeIndexColumns(definition[open+1:close]) + ")", nil
}

func qualifiedIndexName(table, index string) string {
	table = strings.Trim(table, "`\"")
	index = strings.Trim(index, "`\"")
	var b strings.Builder
	b.WriteByte('`')
	for _, ch := range table + "__" + index {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' {
			b.WriteRune(ch)
		} else {
			b.WriteByte('_')
		}
	}
	b.WriteByte('`')
	return b.String()
}

func normalizeIndexColumns(columns string) string {
	return regexp.MustCompile(`\s*\(\s*\d+\s*\)`).ReplaceAllString(columns, "")
}

func normalizeView(statement string) string {
	statement = viewPrefixPattern.ReplaceAllString(statement, "CREATE VIEW ")
	statement = viewModifierPattern.ReplaceAllString(statement, "")
	return statement
}

func normalizeMySQLLiterals(statement string) string {
	var b strings.Builder
	inString := false
	for i := 0; i < len(statement); i++ {
		ch := statement[i]
		if !inString {
			if ch == '\'' {
				inString = true
				b.WriteByte(ch)
				continue
			}
			if ch == '0' && i+2 < len(statement) && (statement[i+1] == 'x' || statement[i+1] == 'X') {
				end := i + 2
				for end < len(statement) && ((statement[end] >= '0' && statement[end] <= '9') || (statement[end] >= 'a' && statement[end] <= 'f') || (statement[end] >= 'A' && statement[end] <= 'F')) {
					end++
				}
				if end > i+2 {
					b.WriteString("X'")
					b.WriteString(statement[i+2 : end])
					b.WriteByte('\'')
					i = end - 1
					continue
				}
			}
			b.WriteByte(ch)
			continue
		}
		if ch == '\\' && i+1 < len(statement) {
			i++
			switch statement[i] {
			case '\'':
				b.WriteString("''")
			case '\\':
				b.WriteByte('\\')
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case '0':
				b.WriteByte(0)
			default:
				b.WriteByte(statement[i])
			}
			continue
		}
		b.WriteByte(ch)
		if ch == '\'' {
			if i+1 < len(statement) && statement[i+1] == '\'' {
				i++
				b.WriteByte(statement[i])
				continue
			}
			inString = false
		}
	}
	return b.String()
}

func firstToken(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	if value[0] == '`' || value[0] == '"' {
		quote := value[0]
		for i := 1; i < len(value); i++ {
			if value[i] == quote {
				return value[:i+1], strings.TrimSpace(value[i+1:])
			}
		}
		return value, ""
	}
	for i := 0; i < len(value); i++ {
		if value[i] == ' ' || value[i] == '\t' || value[i] == '\r' || value[i] == '\n' {
			return value[:i], strings.TrimSpace(value[i:])
		}
	}
	return value, ""
}

func findOpeningParen(value string) int {
	var quote byte
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if quote != 0 {
			if ch == '\\' {
				i++
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' || ch == '`' {
			quote = ch
			continue
		}
		if ch == '(' {
			return i
		}
	}
	return -1
}

func matchingParen(value string, open int) int {
	depth := 0
	var quote byte
	for i := open; i < len(value); i++ {
		ch := value[i]
		if quote != 0 {
			if ch == '\\' {
				i++
				continue
			}
			if ch == quote {
				if i+1 < len(value) && value[i+1] == quote && quote != '`' {
					i++
					continue
				}
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' || ch == '`' {
			quote = ch
			continue
		}
		if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func splitTopLevel(value string) []string {
	parts := make([]string, 0)
	start := 0
	depth := 0
	var quote byte
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if quote != 0 {
			if ch == '\\' {
				i++
				continue
			}
			if ch == quote {
				if i+1 < len(value) && value[i+1] == quote && quote != '`' {
					i++
					continue
				}
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' || ch == '`' {
			quote = ch
		} else if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
		} else if ch == ',' && depth == 0 {
			parts = append(parts, strings.TrimSpace(value[start:i]))
			start = i + 1
		}
	}
	if tail := strings.TrimSpace(value[start:]); tail != "" {
		parts = append(parts, tail)
	}
	return parts
}
