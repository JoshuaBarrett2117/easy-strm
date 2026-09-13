package controller

import (
	"database/sql"
	"easy-strm/internal/domain"
	"easy-strm/internal/service"
	"errors"
	"github.com/gin-gonic/gin"
	"strconv"
	"strings"
)

// Library 返回作品级海报墙。
func (c *ShareRecordController) Library(x *gin.Context) {
	var q domain.ShareLibraryQuery
	if err := x.ShouldBindQuery(&q); err != nil {
		ErrorResp(x, 400, "查询参数格式无效")
		return
	}
	if err := service.ValidateLibraryQuery(&q); err != nil {
		ErrorResp(x, 400, err.Error())
		return
	}
	v, err := c.s.Library(x, q)
	if err != nil {
		ErrorResp(x, 500, err.Error())
		return
	}
	SuccessResp(x, v)
}

// LibrarySources 返回作品来源分页。
func (c *ShareRecordController) LibrarySources(x *gin.Context) {
	var q domain.ShareLibraryQuery
	if x.ShouldBindQuery(&q) != nil || service.ValidateLibraryQuery(&q) != nil || x.Query("work_key") == "" {
		ErrorResp(x, 400, "作品或分页参数无效")
		return
	}
	v, err := c.s.LibrarySources(x, x.Query("work_key"), q.Page, q.PageSize)
	if err != nil {
		ErrorResp(x, 500, err.Error())
		return
	}
	SuccessResp(x, v)
}

// LibraryTVDetail 返回电视剧的完整季目录和本地资源覆盖统计。
func (c *ShareRecordController) LibraryTVDetail(x *gin.Context) {
	key := strings.TrimSpace(x.Query("work_key"))
	if key == "" || len(key) > 1024 {
		ErrorResp(x, 400, "作品参数无效")
		return
	}
	v, err := c.s.LibraryTVDetail(x, key)
	if errors.Is(err, sql.ErrNoRows) {
		ErrorResp(x, 404, "作品不存在")
		return
	}
	if errors.Is(err, service.ErrLibraryNotTV) {
		ErrorResp(x, 400, err.Error())
		return
	}
	if err != nil {
		ErrorResp(x, 500, err.Error())
		return
	}
	SuccessResp(x, v)
}

// LibraryTVSeason 返回电视剧单季完整集目录及其分享文件。
func (c *ShareRecordController) LibraryTVSeason(x *gin.Context) {
	key := strings.TrimSpace(x.Query("work_key"))
	season, err := strconv.Atoi(x.Query("season_number"))
	if key == "" || len(key) > 1024 || err != nil || season < 0 || season > 10000 {
		ErrorResp(x, 400, "作品或季参数无效")
		return
	}
	v, err := c.s.LibraryTVSeason(x, key, season)
	if errors.Is(err, sql.ErrNoRows) {
		ErrorResp(x, 404, "作品不存在")
		return
	}
	if errors.Is(err, service.ErrLibraryNotTV) {
		ErrorResp(x, 400, err.Error())
		return
	}
	if err != nil {
		ErrorResp(x, 500, err.Error())
		return
	}
	SuccessResp(x, v)
}

// LibraryOptions 返回筛选选项。
func (c *ShareRecordController) LibraryOptions(x *gin.Context) {
	v, err := c.s.LibraryOptions(x)
	if err != nil {
		ErrorResp(x, 500, err.Error())
		return
	}
	SuccessResp(x, v)
}

// EnrichLibrary 创建可取消的历史补全任务。
func (c *ShareRecordController) EnrichLibrary(x *gin.Context) {
	id, err := c.s.StartLibraryEnrichment()
	if err != nil {
		ErrorResp(x, 500, err.Error())
		return
	}
	SuccessResp(x, gin.H{"task_id": id})
}
