package dao

import (
	"easy-strm/internal/domain"
	"strings"
	"testing"
)

func TestLibraryAggregationScopeAndCollectedSort(t *testing.T) {
	for _, sort := range []string{"created", "year", "title", "rating", ""} {
		for _, available := range []bool{false, true} {
			query, args := buildShareLibraryQuery(domain.ShareLibraryQuery{WorkKey: "作品", MediaType: "tv", Sort: sort, Available: available, Page: 3, PageSize: 20})
			if len(args) != 4 || args[0] != "作品" || args[1] != "tv" || args[2] != 20 || args[3] != 40 {
				t.Fatalf("筛选与分页绑定改变：%v", args)
			}
			metadataEnd := strings.Index(query, "),")
			if metadataEnd < 0 || !strings.Contains(query[:metadataEnd], "work_key=$1") || !strings.Contains(query[:metadataEnd], "media_type=$2") {
				t.Fatal("作品与类型过滤必须先于文件聚合")
			}
			if strings.Contains(query, "m.created_at") || !strings.Contains(query, "max(f.created_at) collected_at") {
				t.Fatal("收录时间必须取文件创建时间最大值")
			}
			if sort == "created" || sort == "" {
				if !strings.Contains(query, "JOIN matched m ON m.id=f.media_id") || !strings.Contains(query, "ORDER BY collected_at DESC NULLS LAST,work_key ASC") {
					t.Fatal("收录时间排序不得只聚合分页作品后再排序")
				}
			} else {
				if !strings.Contains(query, "JOIN page m ON m.id=f.media_id") || !strings.Contains(query, "SELECT count(*) FROM filtered") {
					t.Fatal("主数据排序只应聚合本页；总数必须来自全部筛选结果")
				}
			}
			if available && !strings.Contains(query, "st.source_count>0") && !strings.Contains(query, "AND f.available AND NOT s.share_cancelled") {
				t.Fatal("可用性筛选丢失")
			}
		}
	}
}
