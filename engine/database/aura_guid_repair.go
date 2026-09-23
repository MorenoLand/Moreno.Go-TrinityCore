package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type AuraGUIDRepairReport struct {
	CharacterSourceRows int
	PetSourceRows       int
	CharacterRecovered  int
	CharacterSkipped    int
	PetRecovered        int
	PetSkipped          int
}

type auraGUIDKey struct {
	guid   uint64
	item   uint64
	spell  uint64
	effect uint64
}

type auraGUIDSource struct {
	character map[auraGUIDKey][]uint64
	pet       map[auraGUIDKey][]uint64
}

type auraGUIDRepairRow struct {
	key    auraGUIDKey
	value  float64
	caster uint64
}

func RepairSQLiteAuraGUIDs(ctx context.Context, db *sql.DB, sourceSQLPath string) (AuraGUIDRepairReport, error) {
	if db == nil || sourceSQLPath == "" {
		return AuraGUIDRepairReport{}, fmt.Errorf("aura GUID repair requires a SQLite database and source SQL dump")
	}
	script, err := os.ReadFile(sourceSQLPath)
	if err != nil {
		return AuraGUIDRepairReport{}, err
	}
	source := parseAuraGUIDSource(string(script))
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return AuraGUIDRepairReport{}, err
	}
	characterRecovered, characterSkipped, err := repairAuraGUIDTable(ctx, tx, "character_aura", true, source.character)
	if err != nil {
		_ = tx.Rollback()
		return AuraGUIDRepairReport{}, err
	}
	petRecovered, petSkipped, err := repairAuraGUIDTable(ctx, tx, "pet_aura", false, source.pet)
	if err != nil {
		_ = tx.Rollback()
		return AuraGUIDRepairReport{}, err
	}
	if err := tx.Commit(); err != nil {
		return AuraGUIDRepairReport{}, err
	}
	return AuraGUIDRepairReport{CharacterSourceRows: len(source.character), PetSourceRows: len(source.pet), CharacterRecovered: characterRecovered, CharacterSkipped: characterSkipped, PetRecovered: petRecovered, PetSkipped: petSkipped}, nil
}

func parseAuraGUIDSource(script string) auraGUIDSource {
	return auraGUIDSource{character: parseAuraGUIDTable(script, "character_aura", []string{"guid", "casterGuid", "itemGuid", "spell", "effectMask"}), pet: parseAuraGUIDTable(script, "pet_aura", []string{"guid", "casterGuid", "spell", "effectMask"})}
}

func parseAuraGUIDTable(script, table string, defaultColumns []string) map[auraGUIDKey][]uint64 {
	result := make(map[auraGUIDKey][]uint64)
	rows := regexp.MustCompile(`\(([^()]*)\)`)
	for _, statement := range auraGUIDInsertStatements(script, table) {
		tableMarker := "`" + table + "`"
		tableIndex := strings.Index(strings.ToLower(statement), strings.ToLower(tableMarker))
		if tableIndex < 0 {
			continue
		}
		tail := statement[tableIndex+len(tableMarker):]
		valuesIndex := strings.Index(strings.ToUpper(tail), "VALUES")
		if valuesIndex < 0 {
			continue
		}
		columns := defaultColumns
		columnList := strings.TrimSpace(tail[:valuesIndex])
		if strings.HasPrefix(columnList, "(") {
			close := strings.LastIndex(columnList, ")")
			if close < 0 {
				continue
			}
			columns = make([]string, 0)
			for _, column := range strings.Split(columnList[1:close], ",") {
				columns = append(columns, strings.ToLower(strings.Trim(strings.TrimSpace(column), "`\"")))
			}
		}
		columnIndexes := make(map[string]int, len(columns))
		for index, column := range columns {
			columnIndexes[strings.ToLower(column)] = index
		}
		guidIndex, guidOK := columnIndexes["guid"]
		casterIndex, casterOK := columnIndexes["casterguid"]
		spellIndex, spellOK := columnIndexes["spell"]
		effectIndex, effectOK := columnIndexes["effectmask"]
		itemIndex, itemOK := columnIndexes["itemguid"]
		if !guidOK || !casterOK || !spellOK || !effectOK || table == "character_aura" && !itemOK {
			continue
		}
		for _, row := range rows.FindAllStringSubmatch(tail[valuesIndex+len("VALUES"):], -1) {
			values := strings.Split(row[1], ",")
			if len(values) <= maxIndex(guidIndex, casterIndex, spellIndex, effectIndex, itemIndex) {
				continue
			}
			guid, guidErr := parseAuraGUIDSQLUint(values[guidIndex])
			caster, casterErr := parseAuraGUIDSQLUint(values[casterIndex])
			spell, spellErr := parseAuraGUIDSQLUint(values[spellIndex])
			effect, effectErr := parseAuraGUIDSQLUint(values[effectIndex])
			item := uint64(0)
			itemErr := error(nil)
			if itemOK {
				item, itemErr = parseAuraGUIDSQLUint(values[itemIndex])
			}
			if guidErr != nil || casterErr != nil || spellErr != nil || effectErr != nil || itemErr != nil {
				continue
			}
			key := auraGUIDKey{guid: guid, item: item, spell: spell, effect: effect}
			duplicate := false
			for _, existing := range result[key] {
				if existing == caster {
					duplicate = true
					break
				}
			}
			if !duplicate {
				result[key] = append(result[key], caster)
			}
		}
	}
	return result
}

func auraGUIDInsertStatements(script, table string) []string {
	upper := strings.ToUpper(script)
	marker := "INSERT INTO `" + strings.ToUpper(table) + "`"
	statements := make([]string, 0)
	start := 0
	for start < len(script) {
		match := strings.Index(upper[start:], marker)
		if match < 0 {
			break
		}
		start += match
		end := strings.Index(script[start:], ";")
		if end < 0 {
			break
		}
		end += start + 1
		statements = append(statements, script[start:end])
		start = end
	}
	return statements
}

func parseAuraGUIDSQLUint(value string) (uint64, error) {
	value = strings.Trim(strings.TrimSpace(value), "'\"")
	return strconv.ParseUint(value, 10, 64)
}

func maxIndex(values ...int) int {
	maximum := -1
	for _, value := range values {
		if value > maximum {
			maximum = value
		}
	}
	return maximum
}

func repairAuraGUIDTable(ctx context.Context, tx *sql.Tx, table string, hasItem bool, source map[auraGUIDKey][]uint64) (int, int, error) {
	query := "SELECT guid, itemGuid, spell, effectMask, casterGuid FROM character_aura WHERE typeof(casterGuid) = 'real'"
	update := "UPDATE character_aura SET casterGuid = ? WHERE guid = ? AND itemGuid = ? AND spell = ? AND effectMask = ? AND typeof(casterGuid) = 'real' AND casterGuid = ?"
	if !hasItem {
		query = "SELECT guid, 0, spell, effectMask, casterGuid FROM pet_aura WHERE typeof(casterGuid) = 'real'"
		update = "UPDATE pet_aura SET casterGuid = ? WHERE guid = ? AND spell = ? AND effectMask = ? AND typeof(casterGuid) = 'real' AND casterGuid = ?"
	}
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	pending := make([]auraGUIDRepairRow, 0)
	skipped := 0
	for rows.Next() {
		var guid, item, spell, effect int64
		var stored float64
		if err := rows.Scan(&guid, &item, &spell, &effect, &stored); err != nil {
			_ = rows.Close()
			return 0, 0, err
		}
		key := auraGUIDKey{guid: uint64(guid), item: uint64(item), spell: uint64(spell), effect: uint64(effect)}
		candidates := source[key]
		match := uint64(0)
		matches := 0
		for _, candidate := range candidates {
			if float64(candidate) == stored {
				match = candidate
				matches++
			}
		}
		if matches != 1 {
			skipped++
			continue
		}
		pending = append(pending, auraGUIDRepairRow{key: key, value: stored, caster: match})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, 0, err
	}
	recovered := 0
	for _, row := range pending {
		caster := row.caster
		var result sql.Result
		if hasItem {
			result, err = tx.ExecContext(ctx, update, []byte(strconv.FormatUint(caster, 10)), row.key.guid, row.key.item, row.key.spell, row.key.effect, row.value)
		} else {
			result, err = tx.ExecContext(ctx, update, []byte(strconv.FormatUint(caster, 10)), row.key.guid, row.key.spell, row.key.effect, row.value)
		}
		if err != nil {
			return 0, skipped, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, skipped, err
		}
		if count == 1 {
			recovered++
		} else {
			skipped++
		}
	}
	return recovered, skipped, nil
}
