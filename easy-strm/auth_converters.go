package main

import "easy-strm/internal/controller"

// convertCloud115ToBrief 将 main.Cloud115 转换为 controller.Cloud115AccountBrief
// NOTE: 两个结构体字段一致，但属于不同包的类型定义，需要手动转换
func convertCloud115ToBrief(acc *Cloud115) *controller.Cloud115AccountBrief {
	if acc == nil {
		return nil
	}
	return &controller.Cloud115AccountBrief{
		ID:                acc.ID,
		Name:              acc.Name,
		Cookie:            acc.Cookie,
		RefreshToken:      acc.RefreshToken,
		AccessToken:       acc.AccessToken,
		ExpiresIn:         acc.ExpiresIn,
		TransferAccountID: acc.TransferAccountID,
		TransferDirectory: acc.TransferDirectory,
		AccountType:       acc.AccountType,
		Priority:          acc.Priority,
		Status:            acc.Status,
		TransferMethod:    acc.TransferMethod,
		AlistUrl:          acc.AlistUrl,
		AlistToken:        acc.AlistToken,
		CreateTime:        acc.CreateTime,
		UpdateTime:        acc.UpdateTime,
	}
}

// convertStrmConfigToDetail 将 main.StrmConfig 转换为 controller.StrmConfigDetail
func convertStrmConfigToDetail(cfg *StrmConfig) *controller.StrmConfigDetail {
	if cfg == nil {
		return nil
	}
	return &controller.StrmConfigDetail{
		ID:               cfg.ID,
		Cloud115Id:       cfg.Cloud115Id,
		NetDiskPath:      cfg.NetDiskPath,
		LocalPath:        cfg.LocalPath,
		Cron:             cfg.Cron,
		Extension:        cfg.Extension,
		SyncMode:         cfg.SyncMode,
		SourceAccount:    cfg.SourceAccount,
		TargetAccount:    cfg.TargetAccount,
		TargetDirectory:  cfg.TargetDirectory,
		AutoCleanup:      cfg.AutoCleanup,
		CleanupThreshold: cfg.CleanupThreshold,
		CleanupPolicy:    cfg.CleanupPolicy,
		MaxConcurrency:   cfg.MaxConcurrency,
		CreateTime:       cfg.CreateTime,
		UpdateTime:       cfg.UpdateTime,
	}
}
