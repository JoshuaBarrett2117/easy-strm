package dao

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// 此守卫只拒绝字面量中的裸参数空值谓词，不是 PostgreSQL 语义校验器。
// 不推断列、函数、运算符或重复参数的类型，不求值动态拼接，也不解析美元引用的过程体。
// 即使参数在其他位置可推断类型，裸 IS [NOT] NULL 仍要求显式 cast，防止以后改写丢失上下文。
func bareParameterNullPredicates(query string) []string {
	tokens := sqlParameterTokens(query)
	var parameters []string
	for index, value := range tokens {
		if len(value) < 2 || value[0] != '$' || value[1] < '0' || value[1] > '9' {
			continue
		}
		start, end := index, index+1
		for start > 0 && end < len(tokens) && tokens[start-1] == "(" && tokens[end] == ")" {
			start--
			end++
		}
		if start < index && start > 0 && sqlIdentifierByte(tokens[start-1][0]) {
			switch tokens[start-1] {
			case "where", "and", "or", "not", "on", "when", "then", "else", "select", "having", "returning", "distinct", "all", "by":
			default:
				continue
			}
		}
		if end >= len(tokens) || tokens[end] != "is" {
			continue
		}
		end++
		if end < len(tokens) && tokens[end] == "not" {
			end++
		}
		if end < len(tokens) && tokens[end] == "null" {
			parameters = append(parameters, value)
		}
	}
	return parameters
}

func sqlIdentifierByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value == '_' || value >= 128
}

// 字符串、引用标识符和美元引用保留屏障 token；注释仅当空白处理，避免误拼两侧词法单元。
func sqlParameterTokens(query string) []string {
	var tokens []string
	for offset := 0; offset < len(query); {
		value := query[offset]
		if value == ' ' || value == '\t' || value == '\n' || value == '\r' || value == '\f' {
			offset++
			continue
		}
		if strings.HasPrefix(query[offset:], "--") {
			for offset < len(query) && query[offset] != '\n' && query[offset] != '\r' {
				offset++
			}
			continue
		}
		if strings.HasPrefix(query[offset:], "/*") {
			offset += 2
			depth := 1
			for offset < len(query) && depth > 0 {
				switch {
				case strings.HasPrefix(query[offset:], "/*"):
					depth++
					offset += 2
				case strings.HasPrefix(query[offset:], "*/"):
					depth--
					offset += 2
				default:
					offset++
				}
			}
			continue
		}
		if value == '\'' || value == '"' {
			escaped := value == '\'' && offset > 0 && (query[offset-1] == 'e' || query[offset-1] == 'E') && (offset < 2 || !sqlIdentifierByte(query[offset-2]))
			offset++
			for offset < len(query) {
				if escaped && query[offset] == '\\' && offset+1 < len(query) {
					offset += 2
					continue
				}
				if query[offset] == value {
					offset++
					if offset < len(query) && query[offset] == value {
						offset++
						continue
					}
					break
				}
				offset++
			}
			tokens = append(tokens, "<quoted>")
			continue
		}
		if value == '$' {
			end := offset + 1
			if end < len(query) && query[end] >= '0' && query[end] <= '9' {
				for end < len(query) && query[end] >= '0' && query[end] <= '9' {
					end++
				}
				tokens = append(tokens, query[offset:end])
				offset = end
				continue
			}
			for end < len(query) && (sqlIdentifierByte(query[end]) || query[end] >= '0' && query[end] <= '9') {
				end++
			}
			if end < len(query) && query[end] == '$' {
				delimiter := query[offset : end+1]
				offset = end + 1
				if closing := strings.Index(query[offset:], delimiter); closing >= 0 {
					offset += closing + len(delimiter)
				} else {
					offset = len(query)
				}
				tokens = append(tokens, "<quoted>")
				continue
			}
		}
		if sqlIdentifierByte(value) {
			end := offset + 1
			for end < len(query) && (sqlIdentifierByte(query[end]) || query[end] >= '0' && query[end] <= '9' || query[end] == '$') {
				end++
			}
			tokens = append(tokens, strings.ToLower(query[offset:end]))
			offset = end
			continue
		}
		tokens = append(tokens, query[offset:offset+1])
		offset++
	}
	return tokens
}

func scanProductionSQLParameterLiterals(root string) ([]string, int, int, error) {
	paths := []string{filepath.Join(root, "db_system_config.go")}
	for _, directory := range []string{"internal/dao", "cmd/share-cleanup"} {
		if err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !entry.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
				paths = append(paths, path)
			}
			return nil
		}); err != nil {
			return nil, 0, 0, err
		}
	}
	var violations []string
	literals := 0
	for _, path := range paths {
		positions := token.NewFileSet()
		file, err := parser.ParseFile(positions, path, nil, 0)
		if err != nil {
			return nil, 0, 0, err
		}
		var literalErr error
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			query, err := strconv.Unquote(literal.Value)
			if err != nil {
				literalErr = err
				return false
			}
			literals++
			for _, parameter := range bareParameterNullPredicates(query) {
				violations = append(violations, fmt.Sprintf("%s: bare %s IS [NOT] NULL requires an explicit cast", positions.Position(literal.Pos()), parameter))
			}
			return true
		})
		if literalErr != nil {
			return nil, 0, 0, literalErr
		}
	}
	return violations, len(paths), literals, nil
}

// TestSQLParameterNullGuard 验证裸空值谓词、显式类型和 SQL 词法边界，不模拟 PostgreSQL 解析。
func TestSQLParameterNullGuard(t *testing.T) {
	for _, test := range []struct {
		name  string
		query string
		want  int
	}{
		{"original_enqueue", `SELECT share_export_enqueue(ARRAY(SELECT work_key FROM selection_changed ORDER BY work_key),'selection-sync') WHERE $1 IS NOT NULL`, 1},
		{"cast_enqueue", `SELECT share_export_enqueue(ARRAY(SELECT work_key FROM selection_changed ORDER BY work_key),'selection-sync') WHERE $1::text IS NOT NULL`, 0},
		{"is_null", `SELECT 1 WHERE $1 IS NULL`, 1},
		{"mixed_case", `SELECT 1 WHERE $12 iS nOt nUlL`, 1},
		{"multiline", "SELECT 1 WHERE $123\nIS\nNOT\nNULL", 1},
		{"parentheses_comments", "SELECT 1 WHERE ( /* outer */ ( $23 -- parameter\n ) /* close */ ) IS /* nested /* comment */ still comment */ NOT -- negation\nNULL", 1},
		{"parenthesized_null", `SELECT 1 WHERE (($12)) IS NULL`, 1},
		{"distinct_parentheses", `SELECT DISTINCT (($12)) IS NULL`, 1},
		{"all_parentheses", `SELECT ALL ($12) IS NOT NULL`, 1},
		{"order_by_parentheses", `SELECT 1 ORDER BY (($12)) IS NULL`, 1},
		{"two_parameters", `SELECT 1 WHERE ($12) IS NULL OR $123 IS NOT NULL`, 2},
		{"cast_parentheses", `SELECT 1 WHERE (($12)::text) IS NULL`, 0},
		{"cast_outside", `SELECT 1 WHERE (($12))::text IS NOT NULL`, 0},
		{"cast_function", `SELECT 1 WHERE CAST(($12) AS text) IS NULL`, 0},
		{"typed_function", `SELECT 1 WHERE length(($12)) IS NULL`, 0},
		{"typed_expression", `SELECT 1 WHERE ($12 + 1) IS NULL`, 0},
		{"typed_elsewhere_still_bare", `SELECT $12::text WHERE $12 IS NULL`, 1},
		{"column_null", `SELECT 1 WHERE work_key IS NOT NULL`, 0},
		{"string_literal", `SELECT '$1 IS NULL', 'it''s $12 IS NOT NULL'`, 0},
		{"escaped_string", `SELECT E'it\'s $12 IS NOT NULL'`, 0},
		{"dollar_literals", `SELECT $$ $1 IS NULL $$, $body$ $12 IS NOT NULL $body$`, 0},
		{"quoted_identifier", `SELECT "$12 IS NULL" FROM example`, 0},
		{"identifier_parameter_suffix", `SELECT work$12 IS NULL FROM example`, 0},
		{"comment_only", "SELECT 1 -- $12 IS NULL\n/* $123 IS NOT NULL */", 0},
		{"literal_barrier", `SELECT $12 'ignored' IS NULL`, 0},
		{"identifier_barrier", `SELECT $12 "ignored" IS NULL`, 0},
		{"quoted_and_real", `SELECT '$1 IS NULL' WHERE (($23)) IS NOT NULL`, 1},
		{"comment_and_real", `SELECT 1 /* $1 IS NOT NULL */ WHERE $123 IS NULL`, 1},
		{"keyword_boundary", `SELECT 1 WHERE $12 ISNULL`, 0},
		{"comment_token_boundary", `SELECT 1 WHERE $12 I/* split */S NULL`, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			if found := bareParameterNullPredicates(test.query); len(found) != test.want {
				t.Fatalf("found %v, want %d violations in %q", found, test.want, test.query)
			}
		})
	}
	if found := bareParameterNullPredicates(`SELECT 1 WHERE ($12) IS NULL OR (($123)) IS NOT NULL`); strings.Join(found, ",") != "$12,$123" {
		t.Fatalf("multidigit placeholders must remain intact: %v", found)
	}
}

// TestSQLParameterNullGuardASTScope 验证扫描覆盖目录、解释字符串和数组字面量，并排除测试文件与 Go 注释。
func TestSQLParameterNullGuardASTScope(t *testing.T) {
	root := t.TempDir()
	for path, source := range map[string]string{
		"db_system_config.go":                  "package main\nconst query = \"SELECT 1 WHERE $12\\nIS NULL\"\n",
		"internal/dao/selection.go":            "package dao\n// SELECT $999 IS NULL\nvar queries = []string{`SELECT 1 WHERE (($23)) IS NOT NULL`}\n",
		"internal/dao/nested/query.go":         "package nested\nconst query = `SELECT 1 WHERE $123 IS NULL`\n",
		"internal/dao/query_test.go":           "package dao\nconst query = `SELECT 1 WHERE $999 IS NULL`\n",
		"cmd/share-cleanup/main.go":            "package main\nconst query = `SELECT 1 WHERE $45 IS NULL`\n",
		"cmd/share-cleanup/main_test.go":       "package main\nconst query = `SELECT 1 WHERE $999 IS NULL`\n",
		"cmd/share-cleanup/nested/fragment.go": "package nested\nconst query = `SELECT 1 WHERE $67::text IS NULL`\n",
	} {
		filename := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	violations, files, literals, err := scanProductionSQLParameterLiterals(root)
	if err != nil {
		t.Fatal(err)
	}
	if files != 5 || literals != 5 || len(violations) != 4 || strings.Contains(strings.Join(violations, "\n"), "$999") {
		t.Fatalf("files=%d literals=%d violations=%v", files, literals, violations)
	}
}

// TestSQLParameterNullGuardProductionLiterals 遍历实际生产源码的字符串字面量，遇到裸空值谓词即失败。
func TestSQLParameterNullGuardProductionLiterals(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate backend source")
	}
	root := filepath.Join(filepath.Dir(filename), "../..")
	violations, files, literals, err := scanProductionSQLParameterLiterals(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("AST guard scanned %d production Go files and %d string literals; not a PostgreSQL semantic validator", files, literals)
	for _, violation := range violations {
		t.Error(violation)
	}
}

// TestSQLParameterNullGuardSelectionSync 额外检查运行时实际使用的同步 SQL，避免只检查测试副本。
func TestSQLParameterNullGuardSelectionSync(t *testing.T) {
	if len(shareSelectionSyncSQL) == 0 {
		t.Fatal("selection sync SQL is empty")
	}
	for index, query := range shareSelectionSyncSQL {
		if found := bareParameterNullPredicates(query); len(found) != 0 {
			t.Errorf("shareSelectionSyncSQL[%d]: bare null predicates %v", index, found)
		}
	}
}
