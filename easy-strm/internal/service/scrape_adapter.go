package service

import (
	"context"
	"fmt"

	"easy-strm/internal/domain"
)

// PostTransferScraper 转存后刮削统一接口。
// 实现按媒体源类型分流：
//   - 本地源（local）：走真实 ScrapeService.NFO 生成与写入；
//   - 115 云盘（cloud115）：本期优雅降级，返回 skipped 结果且不报错、不阻断转存
//     （NFO 需写入远程 115 文件系统，本期未实现上传，仅预留接口）。
//
// Scrape 的返回语义：
//   - 正常刮削：返回各文件的 ScrapeResult（Success=true/false）；
//   - 115 降级：返回全部 Success=false 且带明确原因的结果，error 必须为 nil，
//     以便上层将任务标记为 completed（带 skipped 明细）而非 failed。
type PostTransferScraper interface {
	// Scrape 对已成功整理的文件路径执行刮削。
	//   - ctx: 上下文（可携带超时/取消）；
	//   - source: 媒体源（已构造，可能 ID=0 的临时源）；
	//   - filePaths: 待刮削文件在源中的路径（115 为 /电影/x.mkv 形式，本地为绝对路径）。
	//
	// 返回：
	//   - []ScrapeResult: 逐文件刮削结果；
	//   - error: 仅真实刮削失败（如本地 NFO 写入失败）时非 nil，触发刮削任务 failed。
	Scrape(ctx context.Context, source *domain.MediaSource, filePaths []string) ([]ScrapeResult, error)
}

// LocalScraper 本地媒体源刮削适配器，包装既有 ScrapeService。
// 仅对持久化本地源（source.ID>0）有效，调用 ScrapeFiles 生成并写入 NFO/侧车。
type LocalScraper struct {
	svc *ScrapeService
}

// NewLocalScraper 创建本地刮削适配器。
func NewLocalScraper(svc *ScrapeService) *LocalScraper {
	return &LocalScraper{svc: svc}
}

// Scrape 调用 ScrapeService.ScrapeFiles 对本地文件真实刮削。
func (l *LocalScraper) Scrape(ctx context.Context, source *domain.MediaSource, filePaths []string) ([]ScrapeResult, error) {
	if l.svc == nil {
		return nil, fmt.Errorf("ScrapeService 未初始化")
	}
	return l.svc.ScrapeFiles(source.ID, filePaths)
}

// Cloud115Scraper 115 云盘刮削适配器（本期降级实现）。
// 因 NFO/海报需写入远程 115 文件系统、而本期未实现上传能力，
// 故对所有文件返回 skipped 结果且 error=nil，保证任务 completed 且转存不受影响。
// 预留 svc 以在后续迭代接入 NFO 内容生成 + 115 上传。
type Cloud115Scraper struct {
	svc    *ScrapeService
	client Cloud115Client
}

// NewCloud115Scraper 创建 115 刮削适配器（降级）。
func NewCloud115Scraper(svc *ScrapeService, client Cloud115Client) *Cloud115Scraper {
	return &Cloud115Scraper{svc: svc, client: client}
}

// Scrape 115 云盘刮削（本期降级）：对每个文件返回 skipped 结果，error 为 nil。
// 上层据此将刮削任务标记为 completed（带 scrape_status=skipped 明细），不报错、不阻断。
func (c *Cloud115Scraper) Scrape(ctx context.Context, source *domain.MediaSource, filePaths []string) ([]ScrapeResult, error) {
	results := make([]ScrapeResult, 0, len(filePaths))
	for _, fp := range filePaths {
		results = append(results, ScrapeResult{
			FilePath: fp,
			Success:  false,
			Message:  "115 云盘暂不支持 NFO 写入（本期未实现上传）",
		})
	}
	return results, nil
}
