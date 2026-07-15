package main

import (
	"fmt"
)

func GetCloud115ByID(id int) (*Cloud115, error) {
	Debug("Getting cloud_115 by ID: %d", id)
	cloud115 := &Cloud115{}
	err := db.QueryRow(`SELECT id, name, cookie, refresh_token, access_token, expires_in,
		COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''),
		COALESCE(account_type, 'resource'), COALESCE(quota_used, 0), COALESCE(priority, 5),
		COALESCE(status, 'active'), cooling_start_time,
		COALESCE(transfer_method, ''),
		COALESCE(alist_url, ''), COALESCE(alist_token, ''),
		create_time, update_time FROM t_cloud_115 WHERE id = $1`, id).Scan(
		&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken,
		&cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID,
		&cloud115.TransferDirectory, &cloud115.AccountType, &cloud115.QuotaUsed,
		&cloud115.Priority, &cloud115.Status, &cloud115.CoolingStartTime,
		&cloud115.TransferMethod, &cloud115.AlistUrl, &cloud115.AlistToken,
		&cloud115.CreateTime, &cloud115.UpdateTime)
	if err != nil {
		Error("Failed to get cloud_115 by ID %d: %v", id, err)
		return nil, err
	}
	Debug("Found cloud_115 by ID %d: %s", id, cloud115.Name)
	return cloud115, nil
}

// GetCloud115ByName 根据名称获取115云账号

func GetCloud115ByName(name string) (*Cloud115, error) {
	Debug("Getting cloud_115 by name: %s", name)
	cloud115 := &Cloud115{}
	err := db.QueryRow(`SELECT id, name, cookie, refresh_token, access_token, expires_in,
		COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''),
		COALESCE(account_type, 'resource'), COALESCE(quota_used, 0), COALESCE(priority, 5),
		COALESCE(status, 'active'), cooling_start_time,
		COALESCE(transfer_method, ''),
		COALESCE(alist_url, ''), COALESCE(alist_token, ''),
		create_time, update_time FROM t_cloud_115 WHERE name = $1`, name).Scan(
		&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken,
		&cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID,
		&cloud115.TransferDirectory, &cloud115.AccountType, &cloud115.QuotaUsed,
		&cloud115.Priority, &cloud115.Status, &cloud115.CoolingStartTime,
		&cloud115.TransferMethod, &cloud115.AlistUrl, &cloud115.AlistToken,
		&cloud115.CreateTime, &cloud115.UpdateTime)
	if err != nil {
		Debug("Cloud_115 not found by name: %s", name)
		return nil, err
	}
	Debug("Found cloud_115 by name %s: ID %d", name, cloud115.ID)
	return cloud115, nil
}

// GetAllCloud115 获取所有115云账号

func GetAllCloud115(sortField, sortOrder string) ([]*Cloud115, error) {
	Debug("Getting all cloud_115 accounts with sort: %s %s", sortField, sortOrder)

	if sortField == "" {
		sortField = "id"
	}
	if sortOrder == "" {
		sortOrder = "asc"
	}

	query := fmt.Sprintf(`SELECT id, name, cookie, refresh_token, access_token, expires_in,
		COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''),
		COALESCE(account_type, 'resource'), COALESCE(quota_used, 0), COALESCE(priority, 5),
		COALESCE(status, 'active'), cooling_start_time,
		COALESCE(transfer_method, ''),
		COALESCE(alist_url, ''), COALESCE(alist_token, ''),
		create_time, update_time FROM t_cloud_115 ORDER BY %s %s`, sortField, sortOrder)
	rows, err := db.Query(query)
	if err != nil {
		Error("Failed to get all cloud_115 accounts: %v", err)
		return nil, err
	}
	defer rows.Close()

	var cloud115List []*Cloud115
	for rows.Next() {
		cloud115 := &Cloud115{}
		err := rows.Scan(&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken,
			&cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID,
			&cloud115.TransferDirectory, &cloud115.AccountType, &cloud115.QuotaUsed,
			&cloud115.Priority, &cloud115.Status, &cloud115.CoolingStartTime,
			&cloud115.TransferMethod, &cloud115.AlistUrl, &cloud115.AlistToken,
			&cloud115.CreateTime, &cloud115.UpdateTime)
		if err != nil {
			Error("Failed to scan cloud_115 row: %v", err)
			return nil, err
		}
		cloud115List = append(cloud115List, cloud115)
	}

	if err = rows.Err(); err != nil {
		Error("Error iterating cloud_115 rows: %v", err)
		return nil, err
	}

	Debug("Found %d cloud_115 accounts", len(cloud115List))
	return cloud115List, nil
}

// CreateCloud115 创建115云账号

func CreateCloud115(name, cookie, refreshToken, accessToken string, expiresIn, transferAccountID int, transferDirectory string, accountType string, priority int, transferMethod string, alistUrl string, alistToken string) (*Cloud115, error) {
	Debug("Creating new cloud_115 account: %s", name)
	if accountType == "" {
		accountType = "resource"
	}
	if priority == 0 {
		priority = 5
	}
	if transferMethod == "" {
		transferMethod = "115driver"
	}
	cloud115 := &Cloud115{}
	err := db.QueryRow(
		`INSERT INTO t_cloud_115 (name, cookie, refresh_token, access_token, expires_in, transfer_account_id, transfer_directory, account_type, priority, status, transfer_method, alist_url, alist_token)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'active', $10, $11, $12)
		RETURNING id, name, cookie, refresh_token, access_token, expires_in,
		COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''),
		COALESCE(account_type, 'resource'), COALESCE(quota_used, 0), COALESCE(priority, 5),
		COALESCE(status, 'active'), cooling_start_time, COALESCE(transfer_method, ''),
		COALESCE(alist_url, ''), COALESCE(alist_token, ''),
		create_time, update_time`,
		name, cookie, refreshToken, accessToken, expiresIn, transferAccountID, transferDirectory, accountType, priority, transferMethod, alistUrl, alistToken,
	).Scan(&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken, &cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID, &cloud115.TransferDirectory, &cloud115.AccountType, &cloud115.QuotaUsed, &cloud115.Priority, &cloud115.Status, &cloud115.CoolingStartTime, &cloud115.TransferMethod, &cloud115.AlistUrl, &cloud115.AlistToken, &cloud115.CreateTime, &cloud115.UpdateTime)
	if err != nil {
		Error("Failed to create cloud_115 account %s: %v", name, err)
		return nil, err
	}
	Info("Created new cloud_115 account: %s (ID: %d)", name, cloud115.ID)
	return cloud115, nil
}

// UpdateCloud115 更新115云账号

func UpdateCloud115(id int, name, cookie, refreshToken, accessToken string, expiresIn, transferAccountID int, transferDirectory string, accountType string, priority int, status string, transferMethod string, alistUrl string, alistToken string) (*Cloud115, error) {
	Debug("Updating cloud_115 account with ID: %d", id)
	cloud115 := &Cloud115{}
	err := db.QueryRow(
		`UPDATE t_cloud_115 SET name = $1, cookie = $2, refresh_token = $3, access_token = $4, expires_in = $5, transfer_account_id = $6, transfer_directory = $7, account_type = $8, priority = $9, status = $10, transfer_method = $11, alist_url = $12, alist_token = $13 WHERE id = $14
		RETURNING id, name, cookie, refresh_token, access_token, expires_in,
		COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''),
		COALESCE(account_type, 'resource'), COALESCE(quota_used, 0), COALESCE(priority, 5),
		COALESCE(status, 'active'), cooling_start_time, COALESCE(transfer_method, ''),
		COALESCE(alist_url, ''), COALESCE(alist_token, ''),
		create_time, update_time`,
		name, cookie, refreshToken, accessToken, expiresIn, transferAccountID, transferDirectory, accountType, priority, status, transferMethod, alistUrl, alistToken, id,
	).Scan(&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken, &cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID, &cloud115.TransferDirectory, &cloud115.AccountType, &cloud115.QuotaUsed, &cloud115.Priority, &cloud115.Status, &cloud115.CoolingStartTime, &cloud115.TransferMethod, &cloud115.AlistUrl, &cloud115.AlistToken, &cloud115.CreateTime, &cloud115.UpdateTime)
	if err != nil {
		Error("Failed to update cloud_115 account with ID %d: %v", id, err)
		return nil, err
	}
	Info("Updated cloud_115 account: %s (ID: %d)", cloud115.Name, cloud115.ID)
	return cloud115, nil
}

// DeleteCloud115 删除115云账号

func DeleteCloud115(id int) error {
	Debug("Deleting cloud_115 account with ID: %d", id)
	result, err := db.Exec("DELETE FROM t_cloud_115 WHERE id = $1", id)
	if err != nil {
		Error("Failed to delete cloud_115 account with ID %d: %v", id, err)
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		Error("Failed to get rows affected for delete operation: %v", err)
		return err
	}

	if rowsAffected == 0 {
		Debug("No cloud_115 account found with ID %d for deletion", id)
		return fmt.Errorf("no cloud_115 account found with ID %d", id)
	}

	Info("Deleted cloud_115 account with ID: %d", id)
	return nil
}
