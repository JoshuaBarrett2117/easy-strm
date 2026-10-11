package domain

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestWorkReexportEmptyEpisodesSurvivesPayloadRoundTrip(test *testing.T) {
	for _, episodes := range [][]ShareEpisode{nil, {}} {
		source := ShareStrmSource{URL: "https://115.com/s/fixture", FileName: "combined.mkv", MediaID: 7, Episodes: episodes}
		entry, err := BuildShareWorkReexportEntry(source, "tmdb:movie:7:0:0")
		if err != nil {
			test.Fatal(err)
		}
		payload, err := json.Marshal(entry)
		if err != nil {
			test.Fatal(err)
		}
		var actual ShareStrmEntry
		if err = json.Unmarshal(payload, &actual); err != nil {
			test.Fatal(err)
		}
		if !reflect.DeepEqual(actual, entry) || entry.Episodes != nil {
			test.Fatalf("空分集必须规范为 nil，保证 JSONB 写后回读相等: expected=%#v actual=%#v", entry, actual)
		}
		legacy, err := BuildShareStrmEntry(source)
		if err != nil || !reflect.DeepEqual(legacy.Episodes, episodes) {
			test.Fatal("不得改变普通导出的分集规则")
		}
	}
}

func TestWorkReexportMappingIsPerItemWithoutChangingLegacyID(test *testing.T) {
	source := ShareStrmSource{URL: "https://115.com/s/fixture", FileName: "show/combined.mkv", MediaID: 1}
	legacy, err := BuildShareStrmEntry(source)
	if err != nil {
		test.Fatal(err)
	}
	first, err := BuildShareWorkReexportEntry(source, "tv:1:1")
	if err != nil {
		test.Fatal(err)
	}
	second, err := BuildShareWorkReexportEntry(source, "tv:1:2")
	if err != nil || first.ID == second.ID || first.ID == legacy.ID || second.ID == legacy.ID {
		test.Fatalf("每 item 必须独立且不得覆盖旧 URL: %+v %+v %v", first, second, err)
	}
	again, err := BuildShareWorkReexportEntry(source, "tv:1:1")
	if err != nil || again.ID != first.ID {
		test.Fatal("重复重导 ID 必须稳定")
	}
	if _, err := BuildShareWorkReexportEntry(source, " "); err == nil {
		test.Fatal("空 item 不得生成映射")
	}
	unchanged, _ := BuildShareStrmEntry(source)
	if unchanged.ID != legacy.ID {
		test.Fatal("不得改变普通导出 ID")
	}
}
