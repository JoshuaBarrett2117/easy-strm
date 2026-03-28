package dao

import (
	"database/sql"
	"fmt"

	"easy-strm/internal/domain"
)

// StrmConfigDAO STRM配置数据访问层
type StrmConfigDAO struct{}

// NewStrmConfigDAO 创建STRM配置DAO实例
func NewStrmConfigDAO() *StrmConfigDAO {
	return &StrmConfigDAO{}
}

// GetByID 根据ID获取STRM配置
func (s *StrmConfigDAO) GetByID(id int) (*domain.StrmConfig, error) {
	cfg := &domain.StrmConfig{}
	err := db.QueryRow(
		`SELECT id, cloud115_id, net_disk_path, local_path, cron, extension,
		COALESCE(dir_tree_file, ''), sync_mode, COALESCE(source_account, 0), COALESCE(target_account, 0),
		COALESCE(target_directory, ''), auto_cleanup, COALESCE(cleanup_threshold, 0),
		COALESCE(cleanup_policy, ''), COALESCE(max_concurrency, 0), create_time, update_time
		FROM t_strm_config WHERE id = $1`,
		id,
	).Scan(&cfg.ID, &cfg.Cloud115Id, &cfg.NetDiskPath, &cfg.LocalPath,
		&cfg.Cron, &cfg.Extension, &cfg.DirTreeFile, &cfg.SyncMode, &cfg.SourceAccount,
		&cfg.TargetAccount, &cfg.TargetDirectory, &cfg.AutoCleanup, &cfg.CleanupThreshold,
		&cfg.CleanupPolicy, &cfg.MaxConcurrency, &cfg.CreateTime, &cfg.UpdateTime)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("StrmConfigDAO[GetByID] 查询失败: %v", err)
	}
	return cfg, nil
}

// GetAll 获取所有STRM配置
func (s *StrmConfigDAO) GetAll(sortField, sortOrder string) ([]*domain.StrmConfig, error) {
	orderClause := "id ASC"
	if sortField != "" {
		if sortOrder == "" {
			sortOrder = "ASC"
		}
		orderClause = fmt.Sprintf("%s %s", sortField, sortOrder)
	}

	query := fmt.Sprintf(
		`SELECT id, cloud115_id, net_disk_path, local_path, cron, extension,
		COALESCE(dir_tree_file, ''), sync_mode, COALESCE(source_account, 0), COALESCE(target_account, 0),
		COALESCE(target_directory, ''), auto_cleanup, COALESCE(cleanup_threshold, 0),
		COALESCE(cleanup_policy, ''), COALESCE(max_concurrency, 0), create_time, update_time
		FROM t_strm_config ORDER BY %s`, orderClause)

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("StrmConfigDAO[GetAll] 查询失败: %v", err)
	}
	defer rows.Close()

	var list []*domain.StrmConfig
	for rows.Next() {
		cfg := &domain.StrmConfig{}
		err := rows.Scan(&cfg.ID, &cfg.Cloud115Id, &cfg.NetDiskPath, &cfg.LocalPath,
			&cfg.Cron, &cfg.Extension, &cfg.DirTreeFile, &cfg.SyncMode, &cfg.SourceAccount,
			&cfg.TargetAccount, &cfg.TargetDirectory, &cfg.AutoCleanup, &cfg.CleanupThreshold,
			&cfg.CleanupPolicy, &cfg.MaxConcurrency, &cfg.CreateTime, &cfg.UpdateTime)
		if err != nil {
			return nil, fmt.Errorf("StrmConfigDAO[GetAll] 扫描失败: %v", err)
		}
		list = append(list, cfg)
	}
	return list, nil
}

// Create 创建STRM配置
func (s *StrmConfigDAO) Create(cloud115Id int, netDiskPath, localPath, cron, extension string) (*domain.StrmConfig, error) {
	cfg := &domain.StrmConfig{}
	err := db.QueryRow(
		`INSERT INTO t_strm_config (cloud115_id, net_disk_path, local_path, cron, extension)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, cloud115_id, net_disk_path, local_path, cron, extension, create_time, update_time`,
		cloud115Id, netDiskPath, localPath, cron, extension,
	).Scan(&cfg.ID, &cfg.Cloud115Id, &cfg.NetDiskPath, &cfg.LocalPath,
		&cfg.Cron, &cfg.Extension, &cfg.CreateTime, &cfg.UpdateTime)
	if err != nil {
		return nil, fmt.Errorf("StrmConfigDAO[Create] 创建失败: %v", err)
	}
	return cfg, nil
}

// Update 更新STRM配置
func (s *StrmConfigDAO) Update(id, cloud115Id int, netDiskPath, localPath, cron, extension string) (*domain.StrmConfig, error) {
	cfg := &domain.StrmConfig{}
	err := db.QueryRow(
		`UPDATE t_strm_config SET cloud115_id=$1, net_disk_path=$2, local_path=$3, cron=$4, extension=$5
		WHERE id=$6
		RETURNING id, cloud115_id, net_disk_path, local_path, cron, extension, create_time, update_time`,
		cloud115Id, netDiskPath, localPath, cron, extension, id,
	).Scan(&cfg.ID, &cfg.Cloud115Id, &cfg.NetDiskPath, &cfg.LocalPath,
		&cfg.Cron, &cfg.Extension, &cfg.CreateTime, &cfg.UpdateTime)
	if err != nil {
		return nil, fmt.Errorf("StrmConfigDAO[Update] 更新失败: %v", err)
	}
	return cfg, nil
}

// CreateExt 创建STRM配置（扩展版，包含秒传同步字段）
func (s *StrmConfigDAO) CreateExt(cloud115Id int, netDiskPath, localPath, cron, extension, syncMode string, sourceAccount, targetAccount int, targetDirectory string, autoCleanup bool, cleanupThreshold int, cleanupPolicy string, maxConcurrency int) (*domain.StrmConfig, error) {
	cfg := &domain.StrmConfig{}
	err := db.QueryRow(
		`INSERT INTO t_strm_config (cloud115_id, net_disk_path, local_path, cron, extension, sync_mode, source_account, target_account, target_directory, auto_cleanup, cleanup_threshold, cleanup_policy, max_concurrency)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, cloud115_id, net_disk_path, local_path, cron, extension, dir_tree_file, sync_mode, source_account, target_account, target_directory, auto_cleanup, cleanup_threshold, cleanup_policy, max_concurrency, create_time, update_time`,
		cloud115Id, netDiskPath, localPath, cron, extension, syncMode, sourceAccount, targetAccount, targetDirectory, autoCleanup, cleanupThreshold, cleanupPolicy, maxConcurrency,
	).Scan(&cfg.ID, &cfg.Cloud115Id, &cfg.NetDiskPath, &cfg.LocalPath, &cfg.Cron, &cfg.Extension, &cfg.DirTreeFile, &cfg.SyncMode, &cfg.SourceAccount, &cfg.TargetAccount, &cfg.TargetDirectory, &cfg.AutoCleanup, &cfg.CleanupThreshold, &cfg.CleanupPolicy, &cfg.MaxConcurrency, &cfg.CreateTime, &cfg.UpdateTime)
	if err != nil {
		return nil, fmt.Errorf("StrmConfigDAO[CreateExt] 创建失败: %v", err)
	}
	return cfg, nil
}

// UpdateExt 更新STRM配置（扩展版，包含秒传同步字段）
func (s *StrmConfigDAO) UpdateExt(id, cloud115Id int, netDiskPath, localPath, cron, extension, syncMode string, sourceAccount, targetAccount int, targetDirectory string, autoCleanup bool, cleanupThreshold int, cleanupPolicy string, maxConcurrency int) (*domain.StrmConfig, error) {
	cfg := &domain.StrmConfig{}
	err := db.QueryRow(
		`UPDATE t_strm_config SET cloud115_id=$1, net_disk_path=$2, local_path=$3, cron=$4, extension=$5, sync_mode=$6, source_account=$7, target_account=$8, target_directory=$9, auto_cleanup=$10, cleanup_threshold=$11, cleanup_policy=$12, max_concurrency=$13
		WHERE id=$14
		RETURNING id, cloud115_id, net_disk_path, local_path, cron, extension, dir_tree_file, sync_mode, source_account, target_account, target_directory, auto_cleanup, cleanup_threshold, cleanup_policy, max_concurrency, create_time, update_time`,
		cloud115Id, netDiskPath, localPath, cron, extension, syncMode, sourceAccount, targetAccount, targetDirectory, autoCleanup, cleanupThreshold, cleanupPolicy, maxConcurrency, id,
	).Scan(&cfg.ID, &cfg.Cloud115Id, &cfg.NetDiskPath, &cfg.LocalPath, &cfg.Cron, &cfg.Extension, &cfg.DirTreeFile, &cfg.SyncMode, &cfg.SourceAccount, &cfg.TargetAccount, &cfg.TargetDirectory, &cfg.AutoCleanup, &cfg.CleanupThreshold, &cfg.CleanupPolicy, &cfg.MaxConcurrency, &cfg.CreateTime, &cfg.UpdateTime)
	if err != nil {
		return nil, fmt.Errorf("StrmConfigDAO[UpdateExt] 更新失败: %v", err)
	}
	return cfg, nil
}

// Delete 删除STRM配置
func (s *StrmConfigDAO) Delete(id int) error {
	result, err := db.Exec("DELETE FROM t_strm_config WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("StrmConfigDAO[Delete] 删除失败: %v", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("StrmConfigDAO[Delete] 配置不存在")
	}
	return nil
}

// StrmFileDAO STRM文件记录数据访问层
type StrmFileDAO struct{}

// NewStrmFileDAO 创建STRM文件记录DAO实例
func NewStrmFileDAO() *StrmFileDAO {
	return &StrmFileDAO{}
}

// GetByConfigID 根据配置ID获取所有STRM文件记录
func (s *StrmFileDAO) GetByConfigID(strmConfigID int) ([]*domain.StrmFile, error) {
	rows, err := db.Query(
		`SELECT id, strm_config_id, file_name, file_path, COALESCE(pick_code, ''),
		COALESCE(sha1, ''), COALESCE(file_size, 0), COALESCE(local_strm_path, ''),
		create_time, update_time FROM t_strm_file WHERE strm_config_id = $1`,
		strmConfigID)
	if err != nil {
		return nil, fmt.Errorf("StrmFileDAO[GetByConfigID] 查询失败: %v", err)
	}
	defer rows.Close()

	var list []*domain.StrmFile
	for rows.Next() {
		f := &domain.StrmFile{}
		err := rows.Scan(&f.ID, &f.StrmConfigID, &f.FileName, &f.FilePath,
			&f.PickCode, &f.Sha1, &f.FileSize, &f.LocalStrmPath, &f.CreateTime, &f.UpdateTime)
		if err != nil {
			return nil, fmt.Errorf("StrmFileDAO[GetByConfigID] 扫描失败: %v", err)
		}
		list = append(list, f)
	}
	return list, nil
}

// GetByPath 根据配置ID和文件路径获取STRM文件记录
func (s *StrmFileDAO) GetByPath(strmConfigID int, filePath string) (*domain.StrmFile, error) {
	f := &domain.StrmFile{}
	err := db.QueryRow(
		`SELECT id, strm_config_id, file_name, file_path, COALESCE(pick_code, ''),
		COALESCE(sha1, ''), COALESCE(file_size, 0), COALESCE(local_strm_path, ''),
		create_time, update_time FROM t_strm_file WHERE strm_config_id = $1 AND file_path = $2`,
		strmConfigID, filePath,
	).Scan(&f.ID, &f.StrmConfigID, &f.FileName, &f.FilePath,
		&f.PickCode, &f.Sha1, &f.FileSize, &f.LocalStrmPath, &f.CreateTime, &f.UpdateTime)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("StrmFileDAO[GetByPath] 查询失败: %v", err)
	}
	return f, nil
}

// Upsert 创建或更新STRM文件记录
func (s *StrmFileDAO) Upsert(strmConfigID int, fileName, filePath, pickCode, sha1 string, fileSize int64, localStrmPath string) (*domain.StrmFile, error) {
	f := &domain.StrmFile{}
	err := db.QueryRow(
		`INSERT INTO t_strm_file (strm_config_id, file_name, file_path, pick_code, sha1, file_size, local_strm_path)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (strm_config_id, file_path) DO UPDATE SET
			file_name=$2, pick_code=$4, sha1=$5, file_size=$6, local_strm_path=$7
		RETURNING id, strm_config_id, file_name, file_path, pick_code, sha1, file_size, local_strm_path, create_time, update_time`,
		strmConfigID, fileName, filePath, pickCode, sha1, fileSize, localStrmPath,
	).Scan(&f.ID, &f.StrmConfigID, &f.FileName, &f.FilePath,
		&f.PickCode, &f.Sha1, &f.FileSize, &f.LocalStrmPath, &f.CreateTime, &f.UpdateTime)
	if err != nil {
		return nil, fmt.Errorf("StrmFileDAO[Upsert] 失败: %v", err)
	}
	return f, nil
}

// DeleteByConfigID 删除指定配置的所有STRM文件记录
func (s *StrmFileDAO) DeleteByConfigID(strmConfigID int) error {
	_, err := db.Exec("DELETE FROM t_strm_file WHERE strm_config_id = $1", strmConfigID)
	if err != nil {
		return fmt.Errorf("StrmFileDAO[DeleteByConfigID] 删除失败: %v", err)
	}
	return nil
}

// CountByConfigID 统计指定配置的STRM文件数量
func (s *StrmFileDAO) CountByConfigID(strmConfigID int) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM t_strm_file WHERE strm_config_id = $1", strmConfigID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("StrmFileDAO[CountByConfigID] 统计失败: %v", err)
	}
	return count, nil
}
