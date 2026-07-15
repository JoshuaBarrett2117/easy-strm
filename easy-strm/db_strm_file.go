package main

func GetStrmFileByID(id int) (*StrmFile, error) {
	Debug("Getting strm file by ID: %d", id)
	strmFile := &StrmFile{}
	err := db.QueryRow("SELECT id, strm_config_id, file_name, file_path, COALESCE(pick_code, ''), COALESCE(sha1, ''), COALESCE(file_size, 0), COALESCE(local_strm_path, ''), create_time, update_time FROM t_strm_file WHERE id = $1", id).Scan(
		&strmFile.ID, &strmFile.StrmConfigID, &strmFile.FileName, &strmFile.FilePath, &strmFile.PickCode, &strmFile.Sha1, &strmFile.FileSize, &strmFile.LocalStrmPath, &strmFile.CreateTime, &strmFile.UpdateTime)
	if err != nil {
		Error("Failed to get strm file by ID %d: %v", id, err)
		return nil, err
	}
	return strmFile, nil
}

// GetStrmFilesByConfigID 根据STRM配置ID获取所有STRM文件记录

func GetStrmFilesByConfigID(strmConfigID int) ([]*StrmFile, error) {
	Debug("Getting strm files by config ID: %d", strmConfigID)
	rows, err := db.Query("SELECT id, strm_config_id, file_name, file_path, COALESCE(pick_code, ''), COALESCE(sha1, ''), COALESCE(file_size, 0), COALESCE(local_strm_path, ''), create_time, update_time FROM t_strm_file WHERE strm_config_id = $1", strmConfigID)
	if err != nil {
		Error("Failed to get strm files by config ID %d: %v", strmConfigID, err)
		return nil, err
	}
	defer rows.Close()

	var strmFileList []*StrmFile
	for rows.Next() {
		strmFile := &StrmFile{}
		err := rows.Scan(&strmFile.ID, &strmFile.StrmConfigID, &strmFile.FileName, &strmFile.FilePath, &strmFile.PickCode, &strmFile.Sha1, &strmFile.FileSize, &strmFile.LocalStrmPath, &strmFile.CreateTime, &strmFile.UpdateTime)
		if err != nil {
			Error("Failed to scan strm file row: %v", err)
			return nil, err
		}
		strmFileList = append(strmFileList, strmFile)
	}

	return strmFileList, nil
}

// GetStrmFileByPath 根据配置ID和文件路径获取STRM文件记录

func GetStrmFileByPath(strmConfigID int, filePath string) (*StrmFile, error) {
	Debug("Getting strm file by config ID %d and path: %s", strmConfigID, filePath)
	strmFile := &StrmFile{}
	err := db.QueryRow("SELECT id, strm_config_id, file_name, file_path, COALESCE(pick_code, ''), COALESCE(sha1, ''), COALESCE(file_size, 0), COALESCE(local_strm_path, ''), create_time, update_time FROM t_strm_file WHERE strm_config_id = $1 AND file_path = $2", strmConfigID, filePath).Scan(
		&strmFile.ID, &strmFile.StrmConfigID, &strmFile.FileName, &strmFile.FilePath, &strmFile.PickCode, &strmFile.Sha1, &strmFile.FileSize, &strmFile.LocalStrmPath, &strmFile.CreateTime, &strmFile.UpdateTime)
	if err != nil {
		return nil, err
	}
	return strmFile, nil
}

// CreateStrmFile 创建STRM文件记录

func CreateStrmFile(strmConfigID int, fileName, filePath, pickCode, sha1 string, fileSize int64, localStrmPath string) (*StrmFile, error) {
	Debug("Creating strm file: %s", fileName)
	strmFile := &StrmFile{}
	err := db.QueryRow(
		"INSERT INTO t_strm_file (strm_config_id, file_name, file_path, pick_code, sha1, file_size, local_strm_path) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, strm_config_id, file_name, file_path, pick_code, sha1, file_size, local_strm_path, create_time, update_time",
		strmConfigID, fileName, filePath, pickCode, sha1, fileSize, localStrmPath,
	).Scan(&strmFile.ID, &strmFile.StrmConfigID, &strmFile.FileName, &strmFile.FilePath, &strmFile.PickCode, &strmFile.Sha1, &strmFile.FileSize, &strmFile.LocalStrmPath, &strmFile.CreateTime, &strmFile.UpdateTime)
	if err != nil {
		Error("Failed to create strm file: %v", err)
		return nil, err
	}
	Info("Created strm file: %s (ID: %d)", fileName, strmFile.ID)
	return strmFile, nil
}

// UpsertStrmFile 创建或更新STRM文件记录

func UpsertStrmFile(strmConfigID int, fileName, filePath, pickCode, sha1 string, fileSize int64, localStrmPath string) (*StrmFile, error) {
	Debug("Upserting strm file: %s", fileName)
	strmFile := &StrmFile{}
	err := db.QueryRow(`
		INSERT INTO t_strm_file (strm_config_id, file_name, file_path, pick_code, sha1, file_size, local_strm_path)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (strm_config_id, file_path) DO UPDATE SET file_name = $2, pick_code = $4, sha1 = $5, file_size = $6, local_strm_path = $7
		RETURNING id, strm_config_id, file_name, file_path, pick_code, sha1, file_size, local_strm_path, create_time, update_time
	`, strmConfigID, fileName, filePath, pickCode, sha1, fileSize, localStrmPath).Scan(&strmFile.ID, &strmFile.StrmConfigID, &strmFile.FileName, &strmFile.FilePath, &strmFile.PickCode, &strmFile.Sha1, &strmFile.FileSize, &strmFile.LocalStrmPath, &strmFile.CreateTime, &strmFile.UpdateTime)
	if err != nil {
		Error("Failed to upsert strm file: %v", err)
		return nil, err
	}
	return strmFile, nil
}

// DeleteStrmFile 删除STRM文件记录

func DeleteStrmFile(id int) error {
	Debug("Deleting strm file with ID: %d", id)
	_, err := db.Exec("DELETE FROM t_strm_file WHERE id = $1", id)
	if err != nil {
		Error("Failed to delete strm file with ID %d: %v", id, err)
		return err
	}
	return nil
}

// DeleteStrmFileByPath 根据配置ID和文件路径删除STRM文件记录

func DeleteStrmFileByPath(strmConfigID int, filePath string) error {
	Debug("Deleting strm file by config ID %d and path: %s", strmConfigID, filePath)
	_, err := db.Exec("DELETE FROM t_strm_file WHERE strm_config_id = $1 AND file_path = $2", strmConfigID, filePath)
	if err != nil {
		Error("Failed to delete strm file: %v", err)
		return err
	}
	return nil
}

// DeleteStrmFilesByConfigID 删除指定配置的所有STRM文件记录

func DeleteStrmFilesByConfigID(strmConfigID int) error {
	Debug("Deleting all strm files for config ID: %d", strmConfigID)
	_, err := db.Exec("DELETE FROM t_strm_file WHERE strm_config_id = $1", strmConfigID)
	if err != nil {
		Error("Failed to delete strm files for config ID %d: %v", strmConfigID, err)
		return err
	}
	return nil
}

// CountStrmFilesByConfigID 统计指定配置的STRM文件数量

func CountStrmFilesByConfigID(strmConfigID int) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM t_strm_file WHERE strm_config_id = $1", strmConfigID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}
