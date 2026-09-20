package scripting

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Shopify/go-lua"
)

func installLuaStringCompatibility(state *lua.State) {
	state.Global("string")
	if !state.IsTable(-1) {
		state.Pop(1)
		return
	}
	state.PushGoFunction(luaStringFind)
	state.SetField(-2, "find")
	state.PushGoFunction(luaStringMatch)
	state.SetField(-2, "match")
	state.PushGoFunction(luaStringGMatch)
	state.SetField(-2, "gmatch")
	state.Pop(1)
}

func compileLuaPattern(pattern string) (*regexp.Regexp, int, error) {
	var builder strings.Builder
	captures := 0
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '%':
			if i+1 >= len(pattern) {
				builder.WriteString("%")
				continue
			}
			i++
			builder.WriteString(luaPatternClass(pattern[i]))
		case '[':
			end := i + 1
			if end < len(pattern) && pattern[end] == '^' {
				end++
			}
			for end < len(pattern) && pattern[end] != ']' {
				end++
			}
			if end >= len(pattern) {
				return nil, 0, fmt.Errorf("unterminated character class")
			}
			class := pattern[i : end+1]
			class = strings.ReplaceAll(class, "%w", "A-Za-z0-9_")
			class = strings.ReplaceAll(class, "%a", "A-Za-z")
			class = strings.ReplaceAll(class, "%d", "0-9")
			class = strings.ReplaceAll(class, "%s", "\\t\\n\\v\\f\\r ")
			builder.WriteString(class)
			i = end
		case '(':
			captures++
			builder.WriteByte(ch)
		case ')', '.', '*', '+', '?', '|':
			builder.WriteByte(ch)
		case '^', '$':
			builder.WriteByte(ch)
		case '-':
			builder.WriteByte('*')
		default:
			if strings.ContainsRune(`\\.+*?()|{}[]^$`, rune(ch)) {
				builder.WriteByte('\\')
			}
			builder.WriteByte(ch)
		}
	}
	re, err := regexp.Compile(builder.String())
	return re, captures, err
}

func luaPatternClass(ch byte) string {
	negated := ch >= 'A' && ch <= 'Z'
	if negated {
		ch += 'a' - 'A'
	}
	var value string
	switch ch {
	case 'a':
		value = "A-Za-z"
	case 'c':
		value = "\\x00-\\x1F\\x7F"
	case 'd':
		value = "0-9"
	case 'l':
		value = "a-z"
	case 'p':
		value = "\\x21-\\x2F\\x3A-\\x40\\x5B-\\x60\\x7B-\\x7E"
	case 's':
		value = "\\t\\n\\v\\f\\r "
	case 'u':
		value = "A-Z"
	case 'w':
		value = "A-Za-z0-9_"
	case 'x':
		value = "A-Fa-f0-9"
	case 'z':
		value = "\\x00"
	default:
		value = regexp.QuoteMeta(string(ch))
	}
	if negated {
		return "[^" + value + "]"
	}
	return "[" + value + "]"
}

func luaMatchIndexes(l *lua.State, value string, pattern string, init int) ([]int, int, error) {
	re, captures, err := compileLuaPattern(pattern)
	if err != nil {
		return nil, 0, err
	}
	if init < 1 {
		init = 1
	}
	if init > len(value)+1 {
		return nil, captures, nil
	}
	indexes := re.FindStringSubmatchIndex(value[init-1:])
	if indexes == nil {
		return nil, captures, nil
	}
	for i := range indexes {
		if indexes[i] >= 0 {
			indexes[i] += init - 1
		}
	}
	return indexes, captures, nil
}

func pushLuaMatch(l *lua.State, value string, indexes []int, captures int) int {
	if captures == 0 {
		l.PushString(value[indexes[0]:indexes[1]])
		return 1
	}
	for index := 0; index < captures; index++ {
		start, end := indexes[(index+1)*2], indexes[(index+1)*2+1]
		if start < 0 || end < 0 {
			l.PushNil()
		} else {
			l.PushString(value[start:end])
		}
	}
	return captures
}

func luaStringMatch(l *lua.State) int {
	value, pattern := lua.CheckString(l, 1), lua.CheckString(l, 2)
	indexes, captures, err := luaMatchIndexes(l, value, pattern, lua.OptInteger(l, 3, 1))
	if err != nil {
		lua.Errorf(l, "invalid pattern: %v", err)
	}
	if indexes == nil {
		l.PushNil()
		return 1
	}
	return pushLuaMatch(l, value, indexes, captures)
}

func luaStringFind(l *lua.State) int {
	value, pattern := lua.CheckString(l, 1), lua.CheckString(l, 2)
	indexes, captures, err := luaMatchIndexes(l, value, pattern, lua.OptInteger(l, 3, 1))
	if err != nil {
		lua.Errorf(l, "invalid pattern: %v", err)
	}
	if indexes == nil {
		l.PushNil()
		return 1
	}
	l.PushInteger(indexes[0] + 1)
	l.PushInteger(indexes[1])
	if captures == 0 {
		return 2
	}
	return 2 + pushLuaMatch(l, value, indexes, captures)
}

func luaStringGMatch(l *lua.State) int {
	value, pattern := lua.CheckString(l, 1), lua.CheckString(l, 2)
	re, captures, err := compileLuaPattern(pattern)
	if err != nil {
		lua.Errorf(l, "invalid pattern: %v", err)
	}
	position := 0
	l.PushGoFunction(func(next *lua.State) int {
		if position > len(value) {
			next.PushNil()
			return 1
		}
		indexes := re.FindStringSubmatchIndex(value[position:])
		if indexes == nil {
			next.PushNil()
			return 1
		}
		for index := range indexes {
			if indexes[index] >= 0 {
				indexes[index] += position
			}
		}
		end := indexes[1]
		if end <= position {
			position++
		} else {
			position = end
		}
		return pushLuaMatch(next, value, indexes, captures)
	})
	return 1
}
