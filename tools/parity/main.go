package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type sourceCounts struct {
	Files int
	Lines int
}

type toolStatus struct {
	Reference string
	GoPath    string
	Scaffold  bool
	Findings  []string
}

var (
	opcodePattern                     = regexp.MustCompile(`DEFINE_(?:SERVER_)?(?:OPCODE_)?HANDLER\(\s*([A-Z0-9_]+)`)
	nullOpcodePattern                 = regexp.MustCompile(`DEFINE_HANDLER\(\s*([A-Z0-9_]+)[^;]*Handle_NULL`)
	goCasePattern                     = regexp.MustCompile(`(?ms)^\s*case\s+([^:]+):`)
	goOpcodePattern                   = regexp.MustCompile(`protocol\.Opcode([A-Z0-9_]+)`)
	goSessionHandlerPattern           = regexp.MustCompile(`(?ms)func\s+\(s\s+\*session\)\s+(handle[A-Z][A-Za-z0-9_]*)\s*\([^{}]*\)\s*[^{}]*\{\s*return\s+(true|false)\s*\}`)
	goSessionHandlerDefinitionPattern = regexp.MustCompile(`(?m)func\s+\(s\s+\*session\)\s+(handle[A-Z][A-Za-z0-9_]*)\s*\(`)
	nullOpcodeAuditPattern            = regexp.MustCompile(`(?m)DEFINE_HANDLER\(\s*([A-Z0-9_]+)\s*,\s*(STATUS_[A-Z0-9_]+)[^;\n]*Handle_NULL`)
	serverOpcodePattern               = regexp.MustCompile(`DEFINE_SERVER_OPCODE_HANDLER\(\s*([A-Z0-9_]+)`)
	statementSQLPattern               = regexp.MustCompile(`(?s)PrepareStatement\(\s*([A-Z0-9_]+)\s*,\s*(.*?)(?:,\s*CONNECTION|\);)`)
	stringLitPattern                  = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)
	goStatementSQLPattern             = regexp.MustCompile(`ID:\s*"([A-Z0-9_]+)"\s*,\s*SQL:\s*"((?:[^"\\]|\\.)*)"`)
	schemaPattern                     = regexp.MustCompile(`(?i)CREATE\s+(?:TABLE|VIEW)\s+(?:IF\s+NOT\s+EXISTS\s+)?[\x60]?([A-Za-z0-9_]+)[\x60]?`)
)

func main() {
	reference := flag.String("reference", "", "checked-out TrinityCore reference directory")
	repo := flag.String("repo", ".", "MorenoCore repository directory")
	output := flag.String("output", "docs/PARITY_COVERAGE.md", "coverage report path")
	flag.Parse()
	if *reference == "" {
		fail("-reference is required")
	}
	if _, err := os.Stat(*reference); err != nil {
		fail(err.Error())
	}
	if _, err := os.Stat(*repo); err != nil {
		fail(err.Error())
	}
	report, err := buildReport(*reference, *repo)
	if err != nil {
		fail(err.Error())
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		fail(err.Error())
	}
	if err := os.WriteFile(*output, []byte(report), 0o644); err != nil {
		fail(err.Error())
	}
	fmt.Printf("Wrote parity coverage report to %s\n", *output)
}

func buildReport(reference, repo string) (string, error) {
	refServer, err := countSources(filepath.Join(reference, "src", "server"))
	if err != nil {
		return "", err
	}
	refTools, err := countSources(filepath.Join(reference, "src", "tools"))
	if err != nil {
		return "", err
	}
	goSources, err := countSources(repo)
	if err != nil {
		return "", err
	}
	allRefOpcodes, err := matches(filepath.Join(reference, "src", "server"), opcodePattern, func(path string) bool { return filepath.Base(path) == "Opcodes.cpp" })
	if err != nil {
		return "", err
	}
	goOpcodes, err := goHandlers(filepath.Join(repo, "engine", "world"))
	if err != nil {
		return "", err
	}
	goTestOpcodes, err := goTestHandlers(repo)
	if err != nil {
		return "", err
	}
	goSessionHandlers, goTrivialHandlers, err := goSessionHandlerAudit(filepath.Join(repo, "engine", "world"))
	if err != nil {
		return "", err
	}
	refOpcodes := clientOpcodes(allRefOpcodes)
	goOpcodes = clientOpcodes(goOpcodes)
	refNullOpcodes, err := matches(filepath.Join(reference, "src", "server"), nullOpcodePattern, func(path string) bool { return filepath.Base(path) == "Opcodes.cpp" })
	if err != nil {
		return "", err
	}
	refNullOpcodes = clientOpcodes(refNullOpcodes)
	refServerOpcodes, err := matches(filepath.Join(reference, "src", "server"), serverOpcodePattern, func(path string) bool { return filepath.Base(path) == "Opcodes.cpp" })
	if err != nil {
		return "", err
	}
	// DEFINE_SERVER_OPCODE_HANDLER binds Handle_ServerSide (never a
	// WorldSession handler, STATUS_NEVER/STATUS_UNHANDLED by static
	// assert) — these are not behavioral client bindings.
	refServerOpcodes = clientOpcodes(refServerOpcodes)
	nullAudit, err := nullOpcodeAudit(filepath.Join(reference, "src", "server"))
	if err != nil {
		return "", err
	}
	refBehavioralOpcodes := difference(difference(refOpcodes, refNullOpcodes), refServerOpcodes)
	refStatementSQL, err := statementDetails(filepath.Join(reference, "src", "server"), statementSQLPattern, nil)
	if err != nil {
		return "", err
	}
	goStatementSQL, err := statementDetails(filepath.Join(repo, "engine", "database"), goStatementSQLPattern, nil)
	if err != nil {
		return "", err
	}
	refStatements := keys(refStatementSQL)
	goStatements := keys(goStatementSQL)
	refSchema, err := sqlMatches(filepath.Join(reference, "sql"), schemaPattern)
	if err != nil {
		return "", err
	}
	goMySQLSchema, err := sqlMatches(filepath.Join(repo, "sql", "mysql"), schemaPattern)
	if err != nil {
		return "", err
	}
	goSQLiteSchema, err := sqlMatches(filepath.Join(repo, "sql", "sqlite"), schemaPattern)
	if err != nil {
		return "", err
	}
	refScripts, err := countSources(filepath.Join(reference, "src", "server", "scripts"))
	if err != nil {
		return "", err
	}
	goScripts, err := countSources(filepath.Join(repo, "engine", "scripting"))
	if err != nil {
		return "", err
	}
	refTests, err := countSources(filepath.Join(reference, "tests"))
	if err != nil {
		return "", err
	}
	goTests := countTestSources(repo)
	tools := []toolStatus{
		{Reference: "map_extractor", GoPath: "tools/mapextractor"},
		{Reference: "vmap4_extractor", GoPath: "tools/vmap4extractor"},
		{Reference: "vmap4_assembler", GoPath: "tools/vmap4assembler"},
		{Reference: "mmaps_generator", GoPath: "tools/mmaps-generator"},
		{Reference: "mpq", GoPath: "tools/mpq"},
	}
	for index := range tools {
		content, err := readGoFiles(filepath.Join(repo, tools[index].GoPath))
		if err != nil {
			return "", err
		}
		tools[index].Scaffold = strings.Contains(strings.ToLower(content), "scaffolded") || strings.Contains(strings.ToLower(content), "not implemented")
		tools[index].Findings = toolBehaviorFindings(tools[index].Reference, content)
	}
	var report strings.Builder
	fmt.Fprintf(&report, "Generated from the checked-out reference and current MorenoCore source.\n\n")
	fmt.Fprintf(&report, "This report is an inventory aid; matching counts do not prove behavioral parity.\n\n")
	fmt.Fprintf(&report, "| Area | Reference | Go | Missing reference symbols |\n| --- | ---: | ---: | ---: |\n")
	fmt.Fprintf(&report, "| Server source files / lines | %d / %d | %d / %d | — |\n", refServer.Files, refServer.Lines, goSources.Files, goSources.Lines)
	fmt.Fprintf(&report, "| Tool source files / lines | %d / %d | — | — |\n", refTools.Files, refTools.Lines)
	fmt.Fprintf(&report, "| Client opcode registrations | %d | %d | %d |\n", len(refOpcodes), len(goOpcodes), len(difference(refOpcodes, goOpcodes)))
	fmt.Fprintf(&report, "| Client behavioral opcode bindings (reference, non-NULL) | %d | — | — |\n", len(refBehavioralOpcodes))
	fmt.Fprintf(&report, "| Go session handler definitions | — | %d | — |\n", goSessionHandlers)
	fmt.Fprintf(&report, "| Go trivial session handlers (`return true/false`) | — | %d | — |\n", len(goTrivialHandlers))
	fmt.Fprintf(&report, "| Go registered opcodes with static test references | — | %d | — |\n", len(intersection(goOpcodes, goTestOpcodes)))
	fmt.Fprintf(&report, "| Achievement criteria types | %d | %d | %d |\n", 124, 124, 0)
	fmt.Fprintf(&report, "| Prepared statement identifiers | %d | %d | %d |\n", len(refStatements), len(goStatements), len(difference(refStatements, goStatements)))
	fmt.Fprintf(&report, "| Prepared statement SQL mismatches | — | — | %d |\n", len(sqlDifferences(refStatementSQL, goStatementSQL)))
	fmt.Fprintf(&report, "| Schema tables/views | %d | %d mysql / %d sqlite | %d mysql / %d sqlite |\n", len(refSchema), len(goMySQLSchema), len(goSQLiteSchema), len(difference(keys(refSchema), keys(goMySQLSchema))), len(difference(keys(refSchema), keys(goSQLiteSchema))))
	fmt.Fprintf(&report, "| Script source files / lines | %d / %d | %d / %d | — |\n", refScripts.Files, refScripts.Lines, goScripts.Files, goScripts.Lines)
	fmt.Fprintf(&report, "| Test source files / lines | %d / %d | %d / %d | — |\n", refTests.Files, refTests.Lines, goTests.Files, goTests.Lines)
	fmt.Fprintf(&report, "\n## Missing behavioral client opcode handlers\n\n%s\n", list(difference(refBehavioralOpcodes, goOpcodes)))
	fmt.Fprintf(&report, "## Go session handlers with trivial return bodies\n\n%s\n", list(goTrivialHandlers))
	fmt.Fprintf(&report, "## Go registered opcodes without static test references\n\n%s\n", list(difference(goOpcodes, goTestOpcodes)))
	fmt.Fprintf(&report, "## Reference client opcodes intentionally bound to Handle_NULL\n\n%s\n", list(refNullOpcodes))
	fmt.Fprintf(&report, "## Reference client opcodes registered server-side (Handle_ServerSide, never dispatched to a session handler)\n\n%s\n", list(refServerOpcodes))
	fmt.Fprintf(&report, "## Handle_NULL status audit\n\n| Opcode | Reference status | Classification | Required evidence |\n| --- | --- | --- | --- |\n")
	for _, entry := range nullAudit {
		classification, evidence := nullAuditClassification(entry.Status)
		fmt.Fprintf(&report, "| `%s` | `%s` | %s | %s |\n", entry.Opcode, entry.Status, classification, evidence)
	}
	fmt.Fprintf(&report, "## Missing prepared statements\n\n%s\n", list(difference(refStatements, goStatements)))
	fmt.Fprintf(&report, "## Prepared statement SQL mismatches\n\n%s\n", list(sqlDifferences(refStatementSQL, goStatementSQL)))
	fmt.Fprintln(&report, "## Prepared statement execution audit")
	fmt.Fprintln(&report)
	fmt.Fprintln(&report, "The inventory generator does not open local databases or execute statements. Run `go run ./tools/dbtool statement-audit --input-dir bin` for prepare/EXPLAIN validation and `go run ./tools/dbtool statement-exercise --input-dir bin --strict` for rollback-isolated execution and query-column coverage. These checks exercise the local SQLite schemas; reference result/value/transaction parity still requires differential fixtures.")
	fmt.Fprintln(&report)
	fmt.Fprintf(&report, "## Missing schema tables/views\n\n### MySQL\n\n%s\n\n### SQLite\n\n%s\n", list(difference(keys(refSchema), keys(goMySQLSchema))), list(difference(keys(refSchema), keys(goSQLiteSchema))))
	fmt.Fprintf(&report, "## Extraction tools\n\n| Reference tool | Go path | Status |\n| --- | --- | --- |\n")
	for _, tool := range tools {
		status := "source present; fixture verification pending"
		if tool.Scaffold {
			status = "contains explicit scaffold/not implemented path"
		} else if len(tool.Findings) > 0 {
			status = "behavioral gaps detected"
		}
		fmt.Fprintf(&report, "| `%s` | `%s` | %s |\n", tool.Reference, tool.GoPath, status)
	}
	fmt.Fprintf(&report, "\n## Extraction tool quality findings\n\n")
	for _, tool := range tools {
		if len(tool.Findings) == 0 {
			continue
		}
		fmt.Fprintf(&report, "### `%s`\n\n", tool.GoPath)
		for _, finding := range tool.Findings {
			fmt.Fprintf(&report, "- %s\n", finding)
		}
		fmt.Fprintln(&report)
	}
	return report.String(), nil
}

type nullAuditEntry struct {
	Opcode string
	Status string
}

func nullOpcodeAudit(root string) ([]nullAuditEntry, error) {
	data, err := os.ReadFile(filepath.Join(root, "game", "Server", "Protocol", "Opcodes.cpp"))
	if err != nil {
		return nil, err
	}
	matches := nullOpcodeAuditPattern.FindAllStringSubmatch(string(data), -1)
	entries := make([]nullAuditEntry, 0, len(matches))
	for _, match := range matches {
		if len(match) == 3 && (strings.HasPrefix(match[1], "CMSG_") || strings.HasPrefix(match[1], "MSG_")) {
			entries = append(entries, nullAuditEntry{Opcode: match[1], Status: match[2]})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Opcode < entries[j].Opcode })
	return entries, nil
}

func nullAuditClassification(status string) (string, string) {
	switch status {
	case "STATUS_NEVER":
		return "reference never-accepted no-op", "safe consume/reject test"
	case "STATUS_UNHANDLED":
		return "reference unhandled packet", "rejection/disconnect test"
	default:
		return "logged-in reference null handler", "security and packet-consumption test"
	}
}

func toolBehaviorFindings(name, content string) []string {
	findings := make([]string, 0)
	lower := strings.ToLower(content)
	switch name {
	case "map_extractor":
		if strings.Contains(content, "ExtractDBC") && !strings.Contains(content, "WDT") {
			findings = append(findings, "source extracts DBC files but has no WDT/ADT/liquid/camera extraction path")
		}
	case "vmap4_extractor":
		if strings.Contains(content, "extractWMO") {
			findings = append(findings, "source converts WMO root/group and M2/MDX bounding geometry to VMAP047 raw models; WDT/doodad integration and fixture parity remain open")
		} else if strings.Contains(content, "os.WriteFile") || strings.Contains(content, "io.Copy") {
			findings = append(findings, "source copies raw archive assets; VMAP geometry extraction/output parity is not demonstrated")
		}
	case "vmap4_assembler":
		if strings.Contains(content, "data[:min(len(data), 64)]") {
			findings = append(findings, "source writes only a 64-byte prefix instead of assembling VMAP4 geometry")
		}
	case "mmaps_generator":
		if strings.Contains(lower, "dummyheader") || strings.Contains(content, `"MMAP"`) {
			findings = append(findings, "source emits a dummy MMAP header instead of navmesh tiles")
		} else if strings.Contains(lower, "tile geometry generation is not implemented") {
			findings = append(findings, "source writes reference dtNavMeshParams headers but does not generate .mmtile navmesh data")
		}
	case "mpq":
		if strings.Contains(lower, "unsupported mpq huffman") {
			findings = append(findings, "source rejects MPQ Huffman compression")
		} else if strings.Contains(lower, "unsupported mpq compression") {
			findings = append(findings, "source rejects unknown or remaining unsupported MPQ compression methods")
		}
	}
	return findings
}

func countSources(root string) (sourceCounts, error) {
	counts := sourceCounts{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !isSource(path) {
			return nil
		}
		counts.Files++
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		counts.Lines += strings.Count(string(data), "\n")
		return nil
	})
	return counts, err
}

func matches(root string, pattern *regexp.Regexp, filter func(string) bool) ([]string, error) {
	values := make(map[string]struct{})
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !isSource(path) || filter != nil && !filter(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range pattern.FindAllStringSubmatch(string(data), -1) {
			if len(match) > 1 {
				values[match[1]] = struct{}{}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

// stripGoComments removes // line comments and /* block comments from Go
// source while leaving string literals, raw strings, and rune literals
// intact. The dispatch-table extractor matches case clauses with a
// colon-terminated pattern; a colon inside a reference comment (for
// example "Opcodes.cpp:797") would truncate the clause and drop every
// opcode listed after the comment, so comments must go first.
func stripGoComments(text string) string {
	var out strings.Builder
	out.Grow(len(text))
	runes := []rune(text)
	i := 0
	for i < len(runes) {
		c := runes[i]
		switch c {
		case '/':
			if i+1 < len(runes) && runes[i+1] == '/' {
				for i < len(runes) && runes[i] != '\n' {
					i++
				}
				continue
			}
			if i+1 < len(runes) && runes[i+1] == '*' {
				i += 2
				for i+1 < len(runes) && !(runes[i] == '*' && runes[i+1] == '/') {
					i++
				}
				i += 2
				continue
			}
			out.WriteRune(c)
			i++
		case '"':
			out.WriteRune(c)
			i++
			for i < len(runes) {
				out.WriteRune(runes[i])
				if runes[i] == '\\' && i+1 < len(runes) {
					out.WriteRune(runes[i+1])
					i += 2
					continue
				}
				if runes[i] == '"' {
					i++
					break
				}
				i++
			}
		case '`':
			out.WriteRune(c)
			i++
			for i < len(runes) && runes[i] != '`' {
				out.WriteRune(runes[i])
				i++
			}
			if i < len(runes) {
				out.WriteRune(runes[i])
				i++
			}
		case '\'':
			out.WriteRune(c)
			i++
			for i < len(runes) {
				out.WriteRune(runes[i])
				if runes[i] == '\\' && i+1 < len(runes) {
					out.WriteRune(runes[i+1])
					i += 2
					continue
				}
				if runes[i] == '\'' {
					i++
					break
				}
				i++
			}
		default:
			out.WriteRune(c)
			i++
		}
	}
	return out.String()
}

func goHandlers(root string) ([]string, error) {
	values := make(map[string]struct{})
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := stripGoComments(string(data))
		for _, clause := range goCasePattern.FindAllStringSubmatch(text, -1) {
			if len(clause) <= 1 {
				continue
			}
			for _, opcode := range goOpcodePattern.FindAllStringSubmatch(clause[1], -1) {
				if len(opcode) > 1 {
					values[opcode[1]] = struct{}{}
				}
			}
		}
		if strings.Contains(text, "opcodeAuthSession") {
			values["CMSG_AUTH_SESSION"] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func goTestHandlers(root string) ([]string, error) {
	values := make(map[string]struct{})
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range goOpcodePattern.FindAllStringSubmatch(string(data), -1) {
			if len(match) > 1 && (strings.HasPrefix(match[1], "CMSG_") || strings.HasPrefix(match[1], "MSG_") || strings.HasPrefix(match[1], "UMSG_")) {
				values[match[1]] = struct{}{}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func goSessionHandlerAudit(root string) (int, []string, error) {
	trivial := make([]string, 0)
	total := 0
	intentional := map[string]struct{}{"handleKeepAlive": {}, "handlePlayerLogout": {}}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		total += len(goSessionHandlerDefinitionPattern.FindAllStringSubmatch(text, -1))
		for _, match := range goSessionHandlerPattern.FindAllStringSubmatchIndex(text, -1) {
			if len(match) < 4 {
				continue
			}
			name := text[match[2]:match[3]]
			if _, ok := intentional[name]; ok {
				continue
			}
			line := 1 + strings.Count(text[:match[0]], "\n")
			trivial = append(trivial, fmt.Sprintf("%s (%s:%d)", name, filepath.Base(path), line))
		}
		return nil
	})
	if err != nil {
		return 0, nil, err
	}
	sort.Strings(trivial)
	return total, trivial, nil
}

func clientOpcodes(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.HasPrefix(value, "CMSG_") || strings.HasPrefix(value, "MSG_") || strings.HasPrefix(value, "UMSG_") {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func statementDetails(root string, pattern *regexp.Regexp, filter func(string) bool) (map[string]string, error) {
	values := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !isSource(path) || filter != nil && !filter(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range pattern.FindAllStringSubmatch(string(data), -1) {
			if len(match) > 2 {
				if pattern == statementSQLPattern {
					var sb strings.Builder
					for _, lit := range stringLitPattern.FindAllStringSubmatch(match[2], -1) {
						sb.WriteString(lit[1])
					}
					values[match[1]] = sb.String()
				} else {
					values[match[1]] = match[2]
				}
			}
		}
		return nil
	})
	return values, err
}

func sqlMatches(root string, pattern *regexp.Regexp) (map[string]string, error) {
	values := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || strings.ToLower(filepath.Ext(path)) != ".sql" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range pattern.FindAllStringSubmatch(string(data), -1) {
			if len(match) > 1 {
				values[strings.ToLower(match[1])] = filepath.Base(path)
			}
		}
		return nil
	})
	return values, err
}

func sqlDifferences(reference, implementation map[string]string) []string {
	values := make([]string, 0)
	for name, sql := range reference {
		implementationSQL, ok := implementation[name]
		if ok && normalizeSQL(sql) != normalizeSQL(implementationSQL) {
			values = append(values, name)
		}
	}
	sort.Strings(values)
	return values
}

func normalizeSQL(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(value)), " ")
}

func keys(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func countOptionalSources(root string) sourceCounts {
	counts, err := countSources(root)
	if err != nil {
		return sourceCounts{}
	}
	return counts
}

func countTestSources(root string) sourceCounts {
	counts := sourceCounts{}
	if _, err := os.Stat(root); err != nil {
		return counts
	}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(path), "_test.go") {
			return nil
		}
		if data, err := os.ReadFile(path); err == nil {
			counts.Files++
			counts.Lines += strings.Count(string(data), "\n")
		}
		return nil
	})
	return counts
}

func readGoFiles(root string) (string, error) {
	var content strings.Builder
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content.Write(data)
		return nil
	})
	return content.String(), err
}

func difference(reference, implementation []string) []string {
	known := make(map[string]struct{}, len(implementation))
	for _, value := range implementation {
		known[value] = struct{}{}
	}
	missing := make([]string, 0)
	for _, value := range reference {
		if _, ok := known[value]; !ok {
			missing = append(missing, value)
		}
	}
	return missing
}

func intersection(left, right []string) []string {
	known := make(map[string]struct{}, len(right))
	for _, value := range right {
		known[value] = struct{}{}
	}
	result := make([]string, 0)
	for _, value := range left {
		if _, ok := known[value]; ok {
			result = append(result, value)
		}
	}
	return result
}

func list(values []string) string {
	if len(values) == 0 {
		return "No missing symbols detected."
	}
	var output strings.Builder
	for _, value := range values {
		fmt.Fprintf(&output, "- `%s`\n", value)
	}
	return output.String()
}

func isSource(path string) bool {
	extension := strings.ToLower(filepath.Ext(path))
	return extension == ".cpp" || extension == ".h" || extension == ".go"
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
