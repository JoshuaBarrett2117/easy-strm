package domain

import "testing"

func TestMediaCategoryGetMatchRuleSupportsJSONString(t *testing.T) {
	cat := &MediaCategory{
		MatchRules: []byte(`"{\"keywords\":[\"inception\"],\"default\":false}"`),
	}

	rule := cat.GetMatchRule()
	if len(rule.Keywords) != 1 || rule.Keywords[0] != "inception" {
		t.Fatalf("expected stringified rules to be parsed, got %+v", rule)
	}
}

func TestMediaCategoryNormalizeMatchRulesConvertsJSONString(t *testing.T) {
	cat := &MediaCategory{
		MatchRules: []byte(`"{\"languages\":[\"en\"],\"default\":false}"`),
	}

	cat.NormalizeMatchRules()

	if string(cat.MatchRules) != `{"default":false,"languages":["en"]}` && string(cat.MatchRules) != `{"languages":["en"],"default":false}` {
		t.Fatalf("expected normalized json object, got %s", string(cat.MatchRules))
	}
}
