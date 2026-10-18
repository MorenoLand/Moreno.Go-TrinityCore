package scripting

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/database"
	"github.com/Shopify/go-lua"
)

const queryMetaTable = "MorenoCore.Query"

type Query struct {
	columns []string
	rows    [][]any
	current int
}

func queryDatabase(db *sql.DB, statement string) (*Query, error) {
	if db == nil {
		return nil, errors.New("database is not configured")
	}
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(statement)), "SELECT") {
		if err := executeDatabase(db, statement); err != nil {
			return nil, err
		}
		return nil, nil
	}
	rows, err := db.Query(statement)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result := &Query{columns: columns, rows: make([][]any, 0)}
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(values))
		for index := range values {
			pointers[index] = &values[index]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		for index, value := range values {
			if bytes, ok := value.([]byte); ok {
				values[index] = append([]byte(nil), bytes...)
			}
		}
		result.rows = append(result.rows, values)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result.rows) == 0 {
		return nil, nil
	}
	return result, nil
}

func executeDatabase(db *sql.DB, script string) error {
	if db == nil {
		return errors.New("database is not configured")
	}
	for _, statement := range database.SplitSQL(script) {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		upper := strings.ToUpper(statement)
		parts := []string{statement}
		if strings.HasPrefix(upper, "CREATE TABLE") || strings.HasPrefix(upper, "ALTER TABLE") || strings.HasPrefix(upper, "CREATE INDEX") {
			converted, err := database.NormalizeSchemaStatement(statement, "sqlite")
			if err != nil {
				return err
			}
			parts = converted
		}
		for _, part := range parts {
			if strings.TrimSpace(part) == "" {
				continue
			}
			if _, err := db.Exec(part); err != nil {
				return err
			}
		}
	}
	return nil
}

func pushQuery(state *lua.State, query *Query) {
	state.PushUserData(query)
	lua.SetMetaTableNamed(state, queryMetaTable)
}

func queryIndex(state *lua.State) int {
	query := lua.CheckUserData(state, 1, queryMetaTable).(*Query)
	name := lua.CheckString(state, 2)
	state.PushGoFunction(func(call *lua.State) int {
		switch name {
		case "GetRow":
			return queryGetRow(call, query)
		case "NextRow":
			if query.current+1 >= len(query.rows) {
				call.PushBoolean(false)
				return 1
			}
			query.current++
			call.PushBoolean(true)
			return 1
		case "GetColumnCount":
			call.PushUnsigned(uint(len(query.columns)))
			return 1
		case "GetRowCount":
			call.PushUnsigned(uint(len(query.rows)))
			return 1
		case "IsNull":
			value, ok := queryValue(query, call, 2)
			call.PushBoolean(!ok || value == nil)
			return 1
		case "GetBool":
			return queryGetBool(call, query, 2)
		case "GetUInt8":
			return queryGetNumber(call, query, 2, func(value uint64) { call.PushUnsigned(uint(value & 0xFF)) })
		case "GetUInt16":
			return queryGetNumber(call, query, 2, func(value uint64) { call.PushUnsigned(uint(value & 0xFFFF)) })
		case "GetUInt32":
			return queryGetNumber(call, query, 2, func(value uint64) { call.PushUnsigned(uint(value)) })
		case "GetUInt64":
			return queryGetNumber(call, query, 2, func(value uint64) { pushUInt64(call, value) })
		case "GetInt8":
			return queryGetSigned(call, query, 2, func(value int64) { call.PushInteger(int(value)) })
		case "GetInt16":
			return queryGetSigned(call, query, 2, func(value int64) { call.PushInteger(int(value)) })
		case "GetInt32":
			return queryGetSigned(call, query, 2, func(value int64) { call.PushInteger(int(value)) })
		case "GetInt64":
			return queryGetSigned(call, query, 2, func(value int64) { pushInt64(call, value) })
		case "GetString":
			return queryGetString(call, query, 2)
		case "GetFloat", "GetDouble":
			return queryGetFloat(call, query, 2)
		default:
			call.PushNil()
			return 1
		}
	})
	return 1
}

func queryGetBool(state *lua.State, query *Query, index int) int {
	value, ok := queryValue(query, state, index)
	if !ok || value == nil {
		state.PushBoolean(false)
		return 1
	}
	if number, err := asInt64(value); err == nil {
		// C++ Field::GetBool is GetUInt8() == 1 — only 1 reads true.
		state.PushBoolean(number == 1)
		return 1
	}
	text := strings.ToLower(fmt.Sprint(value))
	state.PushBoolean(text == "1" || text == "true" || text == "yes")
	return 1
}

func queryGetRow(state *lua.State, query *Query) int {
	if query.current < 0 || query.current >= len(query.rows) {
		state.PushNil()
		return 1
	}
	state.NewTable()
	for index, column := range query.columns {
		pushValue(state, query.rows[query.current][index])
		state.SetField(-2, column)
	}
	return 1
}

func queryGetNumber(state *lua.State, query *Query, index int, push func(uint64)) int {
	value, ok := queryValue(query, state, index)
	if !ok {
		state.PushUnsigned(0)
		return 1
	}
	number, err := asUint64(value)
	if err != nil {
		state.PushUnsigned(0)
		return 1
	}
	push(number)
	return 1
}

func queryGetSigned(state *lua.State, query *Query, index int, push func(int64)) int {
	value, ok := queryValue(query, state, index)
	if !ok {
		state.PushInteger(0)
		return 1
	}
	number, err := asInt64(value)
	if err != nil {
		state.PushInteger(0)
		return 1
	}
	push(number)
	return 1
}

func queryGetString(state *lua.State, query *Query, index int) int {
	value, ok := queryValue(query, state, index)
	if !ok || value == nil {
		// C++ Field::GetString returns "" when data.value is null.
		state.PushString("")
		return 1
	}
	if bytes, ok := value.([]byte); ok {
		state.PushString(string(bytes))
		return 1
	}
	state.PushString(fmt.Sprint(value))
	return 1
}

func queryGetFloat(state *lua.State, query *Query, index int) int {
	value, ok := queryValue(query, state, index)
	if !ok {
		state.PushNumber(0)
		return 1
	}
	number, err := asFloat64(value)
	if err != nil {
		state.PushNumber(0)
		return 1
	}
	state.PushNumber(number)
	return 1
}

func queryValue(query *Query, state *lua.State, index int) (any, bool) {
	column, ok := state.ToInteger(index)
	if !ok || column < 0 || column >= len(query.columns) || query.current < 0 || query.current >= len(query.rows) {
		return nil, false
	}
	return query.rows[query.current][column], true
}

func asUint64(value any) (uint64, error) {
	switch value := value.(type) {
	case UInt64:
		return uint64(value), nil
	case Int64:
		if value >= 0 {
			return uint64(value), nil
		}
	case uint64:
		return value, nil
	case uint32:
		return uint64(value), nil
	case uint16:
		return uint64(value), nil
	case uint8:
		return uint64(value), nil
	case int64:
		if value >= 0 {
			return uint64(value), nil
		}
	case int32:
		if value >= 0 {
			return uint64(value), nil
		}
	case int:
		if value >= 0 {
			return uint64(value), nil
		}
	case float64:
		if value >= 0 && value <= math.MaxUint64 {
			return uint64(value), nil
		}
	case []byte:
		return strconv.ParseUint(string(value), 10, 64)
	case string:
		return strconv.ParseUint(value, 10, 64)
	}
	return 0, errors.New("value is not unsigned")
}

func asInt64(value any) (int64, error) {
	switch value := value.(type) {
	case Int64:
		return int64(value), nil
	case UInt64:
		if value <= math.MaxInt64 {
			return int64(value), nil
		}
	case int64:
		return value, nil
	case int32:
		return int64(value), nil
	case int:
		return int64(value), nil
	case uint64:
		if value <= math.MaxInt64 {
			return int64(value), nil
		}
	case uint32:
		return int64(value), nil
	case uint16:
		return int64(value), nil
	case uint8:
		return int64(value), nil
	case float64:
		if value >= math.MinInt64 && value <= math.MaxInt64 {
			return int64(value), nil
		}
	case []byte:
		return strconv.ParseInt(string(value), 10, 64)
	case string:
		return strconv.ParseInt(value, 10, 64)
	}
	return 0, errors.New("value is not signed")
}

func asFloat64(value any) (float64, error) {
	switch value := value.(type) {
	case UInt64:
		return float64(uint64(value)), nil
	case Int64:
		return float64(int64(value)), nil
	case float64:
		return value, nil
	case float32:
		return float64(value), nil
	case int64:
		return float64(value), nil
	case int32:
		return float64(value), nil
	case uint64:
		return float64(value), nil
	case uint32:
		return float64(value), nil
	case uint16:
		return float64(value), nil
	case uint8:
		return float64(value), nil
	case int:
		return float64(value), nil
	case int16:
		return float64(value), nil
	case int8:
		return float64(value), nil
	case []byte:
		return strconv.ParseFloat(string(value), 64)
	case string:
		return strconv.ParseFloat(value, 64)
	}
	return 0, errors.New("value is not number")
}
