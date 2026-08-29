package controller

import (
	"net/http"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"

	"github.com/gin-gonic/gin"
)

func (c *Cloud115Controller) InstantTransfer(ctx *gin.Context) {
	var reqData struct {
		SourceAccountID int    `json:"source_account_id" binding:"required"`
		TargetAccountID int    `json:"target_account_id" binding:"required"`
		FileSHA1        string `json:"file_sha1" binding:"required"`
		FileName        string `json:"file_name"`
		FileSize        int64  `json:"file_size"`
		TargetDirectory string `json:"target_directory"`
	}
	if err := ctx.ShouldBindJSON(&reqData); err != nil {
		logger.Warnf("Cloud115Controller[InstantTransfer] 请求体无效: %v", err)
		ErrorResp(ctx, http.StatusBadRequest, "无效的请求体")
		return
	}

	sourceFile := &domain.FileInfo{
		Name: reqData.FileName,
		Size: reqData.FileSize,
		Sha1: reqData.FileSHA1,
	}

	logger.Infof("Cloud115Controller[InstantTransfer] 秒传请求: SHA1=%s, source=%d, target=%d",
		reqData.FileSHA1, reqData.SourceAccountID, reqData.TargetAccountID)

	result, err := c.cloud115Service.InstantTransfer(sourceFile, reqData.SourceAccountID, reqData.TargetAccountID, reqData.TargetDirectory)
	if err != nil {
		logger.Errorf("Cloud115Controller[InstantTransfer] 秒传失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	status := "pending"
	if result.Skip {
		status = "skipped"
	} else if result.Success {
		status = "completed"
	}

	SuccessResp(ctx, gin.H{
		"message":    result.Message,
		"file_sha1":  result.SHA1,
		"source_id":  reqData.SourceAccountID,
		"target_id":  reqData.TargetAccountID,
		"status":     status,
		"need_retry": result.NeedRetry,
	})
}

func (c *Cloud115Controller) GetTransferCache(ctx *gin.Context) {
	sha1 := ctx.Param("sha1")
	logger.Debugf("Cloud115Controller[GetTransferCache] SHA1缓存查询: %s", sha1)

	cached, cachedCID, err := c.cloud115Service.GetTransferCache(sha1)
	if err != nil {
		logger.Errorf("Cloud115Controller[GetTransferCache] 查询失败: %v", err)
		ErrorResp(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	SuccessResp(ctx, gin.H{
		"sha1":    sha1,
		"cached":  cached,
		"cid":     cachedCID,
		"message": "SHA1缓存查询成功",
	})
}
