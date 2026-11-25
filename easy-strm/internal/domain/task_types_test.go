package domain

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestTaskTypeCatalogUniqueAndLabelled(t *testing.T) {
	seen := map[string]bool{}
	for _, item := range TaskTypeCatalog {
		if item.Value == "" || item.Label == "" || item.Group == "" {
			t.Fatalf("incomplete entry: %+v", item)
		}
		if seen[item.Value] {
			t.Fatalf("duplicate task type %s", item.Value)
		}
		seen[item.Value] = true
	}
	if TaskTypeLabel("strm_generate") != "STRM 生成" || TaskTypeLabel("not_registered") != "not_registered" {
		t.Fatal("TaskTypeLabel mismatch")
	}
}

// 扫描后端源码中创建任务时使用的类型，确保每一种都在目录中登记了中文名。
func TestTaskTypeCatalogCoversCreatedTaskTypes(t *testing.T) {
	root := filepath.Join("..", "..")
	constValues := map[string]string{}
	constPattern := regexp.MustCompile(`(TaskType\w+)\s+TaskType\s*=\s*"([a-z0-9_]+)"`)
	models, err := os.ReadFile(filepath.Join(root, "internal", "domain", "models.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, match := range constPattern.FindAllStringSubmatch(string(models), -1) {
		constValues[match[1]] = match[2]
	}
	patterns := []*regexp.Regexp{
		// tasks.Create(id, "type", ...) / CreateContext(ctx, id, "type", ...) / CreateWithPriority(id, "type", ...)
		regexp.MustCompile(`\.(?:Create|CreateContext|CreateWithPriority)\((?:[^,()]+,\s*){1,2}"([a-z0-9_]+)"`),
		regexp.MustCompile(`\.(?:Create|CreateContext|CreateWithPriority)\((?:[^,()]+,\s*){1,2}string\(domain\.(TaskType\w+)\)`),
		regexp.MustCompile(`runShortTask\(domain\.(TaskType\w+)`),
		regexp.MustCompile(`const \w*[tT]askType\w* = "([a-z0-9_]+)"`),
		regexp.MustCompile(`createTask\(\w+, "([a-z0-9_]+)"`),
	}
	cronKindBody := regexp.MustCompile(`(?s)func cronTaskKind\(.*?\n}`)
	cronKindReturn := regexp.MustCompile(`return "([a-z0-9_]+)"`)
	found := map[string]string{}
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && path != root && (info.Name() == "cmd" || strings.HasPrefix(info.Name(), ".")) {
			return filepath.SkipDir
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		source := string(raw)
		// cronTaskKind 的返回值即定时任务写入任务中心的类型。
		if body := cronKindBody.FindString(source); body != "" {
			for _, match := range cronKindReturn.FindAllStringSubmatch(body, -1) {
				found[match[1]] = path
			}
		}
		for _, pattern := range patterns {
			for _, match := range pattern.FindAllStringSubmatch(source, -1) {
				value := match[1]
				if mapped, ok := constValues[value]; ok {
					value = mapped
				}
				found[value] = path
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) < 15 {
		t.Fatalf("scan found too few task types (%d), regexes likely stale: %v", len(found), found)
	}
	t.Logf("scanned task types: %d", len(found))
	for value, path := range found {
		if _, ok := taskTypeLabels[value]; !ok {
			if strings.HasPrefix(value, "TaskType") {
				t.Errorf("%s: unknown constant %s", path, value)
				continue
			}
			t.Errorf("%s creates task type %q which is missing from TaskTypeCatalog", path, value)
		}
	}
}
