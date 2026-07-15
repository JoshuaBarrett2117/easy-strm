package dao

import (
	"database/sql"
	"fmt"
	"time"

	"easy-strm/internal/domain"
)

type Cloud115DAO struct{}

func NewCloud115DAO() *Cloud115DAO {
	return &Cloud115DAO{}
}

func (c *Cloud115DAO) GetByID(id int) (*domain.Cloud115, error) {
	cloud115 := &domain.Cloud115{}
	err := DB.QueryRow(
		`SELECT id, name, cookie, refresh_token, access_token, expires_in,
		COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''),
		COALESCE(account_type, 'resource'), COALESCE(quota_used, 0), COALESCE(priority, 5),
		COALESCE(status, 'active'), cooling_start_time, COALESCE(transfer_method, ''),
		COALESCE(alist_url, ''), COALESCE(alist_token, ''),
		create_time, update_time FROM t_cloud_115 WHERE id = $1`,
		id,
	).Scan(
		&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken,
		&cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID,
		&cloud115.TransferDirectory, &cloud115.AccountType, &cloud115.QuotaUsed,
		&cloud115.Priority, &cloud115.Status, &cloud115.CoolingStartTime,
		&cloud115.TransferMethod, &cloud115.AlistUrl, &cloud115.AlistToken,
		&cloud115.CreateTime, &cloud115.UpdateTime,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("Cloud115DAO[GetByID] 查询失败: %v", err)
	}
	return cloud115, nil
}

func (c *Cloud115DAO) GetAll(sortField, sortOrder string) ([]*domain.Cloud115, error) {
	orderClause := "id ASC"
	if sortField != "" {
		orderClause = fmt.Sprintf("%s %s", sortField, sortOrder)
		if sortOrder == "" {
			orderClause = fmt.Sprintf("%s ASC", sortField)
		}
	}

	rows, err := DB.Query(
		fmt.Sprintf(`SELECT id, name, cookie, refresh_token, access_token, expires_in,
		COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''),
		COALESCE(account_type, 'resource'), COALESCE(quota_used, 0), COALESCE(priority, 5),
		COALESCE(status, 'active'), cooling_start_time, COALESCE(transfer_method, ''),
		COALESCE(alist_url, ''), COALESCE(alist_token, ''),
		create_time, update_time FROM t_cloud_115 ORDER BY %s`, orderClause),
	)
	if err != nil {
		return nil, fmt.Errorf("Cloud115DAO[GetAll] 查询失败: %v", err)
	}
	defer rows.Close()
	var list []*domain.Cloud115
	for rows.Next() {
		cloud115 := &domain.Cloud115{}
		err := rows.Scan(
			&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken,
			&cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID,
			&cloud115.TransferDirectory, &cloud115.AccountType, &cloud115.QuotaUsed,
			&cloud115.Priority, &cloud115.Status, &cloud115.CoolingStartTime,
			&cloud115.TransferMethod, &cloud115.AlistUrl, &cloud115.AlistToken,
			&cloud115.CreateTime, &cloud115.UpdateTime,
		)
		if err != nil {
			return nil, fmt.Errorf("Cloud115DAO[GetAll] 扫描失败: %v", err)
		}
		list = append(list, cloud115)
	}
	return list, nil
}
func (c *Cloud115DAO) Create(name, cookie, refreshToken, accessToken string, expiresIn, transferAccountID int, transferDirectory string, accountType string, priority int, transferMethod string, alistUrl string, alistToken string) (*domain.Cloud115, error) {
	if accountType == "" {
		accountType = "resource"
	}
	if priority == 0 {
		priority = 5
	}
	if transferMethod == "" {
		transferMethod = "115driver"
	}
	cloud115 := &domain.Cloud115{}
	err := DB.QueryRow(
		`INSERT INTO t_cloud_115 (name, cookie, refresh_token, access_token, expires_in, transfer_account_id, transfer_directory, account_type, priority, status, transfer_method, alist_url, alist_token)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'active', $10, $11, $12)
        RETURNING id, name, cookie, refresh_token, access_token, expires_in,
        COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''),
        COALESCE(account_type, 'resource'), COALESCE(quota_used, 0), COALESCE(priority, 5),
        COALESCE(status, 'active'), cooling_start_time, transfer_method,
        COALESCE(alist_url, ''), COALESCE(alist_token, ''),
        create_time, update_time`,
		name, cookie, refreshToken, accessToken, expiresIn, transferAccountID, transferDirectory, accountType, priority, transferMethod, alistUrl, alistToken,
	).Scan(
		&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken,
		&cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID,
		&cloud115.TransferDirectory, &cloud115.AccountType, &cloud115.QuotaUsed,
		&cloud115.Priority, &cloud115.Status, &cloud115.CoolingStartTime,
		&cloud115.TransferMethod, &cloud115.AlistUrl, &cloud115.AlistToken,
		&cloud115.CreateTime, &cloud115.UpdateTime,
	)
	if err != nil {
		return nil, fmt.Errorf("Cloud115DAO[Create] 创建失败: %v", err)
	}
	return cloud115, nil
}
func (c *Cloud115DAO) Update(id int, name, cookie, refreshToken, accessToken string, expiresIn, transferAccountID int, transferDirectory string, accountType string, priority int, status string, transferMethod string, alistUrl string, alistToken string) (*domain.Cloud115, error) {
	cloud115 := &domain.Cloud115{}
	err := DB.QueryRow(
		`UPDATE t_cloud_115 SET name=$2, cookie=$3, refresh_token=$4, access_token=$5,
        expires_in=$6, transfer_account_id=$7, transfer_directory=$8, account_type=$9, priority=$10, status=$11, transfer_method=$12, alist_url=$13, alist_token=$14
        WHERE id=$1
        RETURNING id, name, cookie, refresh_token, access_token, expires_in,
        COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''),
        COALESCE(account_type, 'resource'), COALESCE(quota_used, 0), COALESCE(priority, 5),
        COALESCE(status, 'active'), cooling_start_time, transfer_method,
        COALESCE(alist_url, ''), COALESCE(alist_token, ''),
        create_time, update_time`,
		id, name, cookie, refreshToken, accessToken, expiresIn, transferAccountID, transferDirectory, accountType, priority, status, transferMethod, alistUrl, alistToken,
	).Scan(
		&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken,
		&cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID,
		&cloud115.TransferDirectory, &cloud115.AccountType, &cloud115.QuotaUsed,
		&cloud115.Priority, &cloud115.Status, &cloud115.CoolingStartTime,
		&cloud115.TransferMethod, &cloud115.AlistUrl, &cloud115.AlistToken,
		&cloud115.CreateTime, &cloud115.UpdateTime,
	)
	if err != nil {
		return nil, fmt.Errorf("Cloud115DAO[Update] 更新失败: %v", err)
	}
	return cloud115, nil
}
func (c *Cloud115DAO) UpdateStatus(id int, status string, coolingStartTime *time.Time) error {
	_, err := DB.Exec(
		`UPDATE t_cloud_115 SET status=$2, cooling_start_time=$3 WHERE id=$1`,
		id, status, coolingStartTime,
	)
	if err != nil {
		return fmt.Errorf("Cloud115DAO[UpdateStatus] 更新状态失败: %v", err)
	}
	return nil
}
func (c *Cloud115DAO) UpdateQuotaUsed(id int, quotaUsed int64) error {
	_, err := DB.Exec(
		`UPDATE t_cloud_115 SET quota_used=$2 WHERE id=$1`,
		id, quotaUsed,
	)
	if err != nil {
		return fmt.Errorf("Cloud115DAO[UpdateQuotaUsed] 更新空间使用量失败: %v", err)
	}
	return nil
}
func (c *Cloud115DAO) Delete(id int) error {
	result, err := DB.Exec("DELETE FROM t_cloud_115 WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("Cloud115DAO[Delete] 删除失败: %v", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("Cloud115DAO[Delete] 账号不存在")
	}
	return nil
}
func (c *Cloud115DAO) GetByStatus(status string) ([]*domain.Cloud115, error) {
	rows, err := DB.Query(
		`SELECT id, name, cookie, refresh_token, access_token, expires_in,
		COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''),
		COALESCE(account_type, 'resource'), COALESCE(quota_used, 0), COALESCE(priority, 5),
		COALESCE(status, 'active'), cooling_start_time, COALESCE(transfer_method, ''),
		COALESCE(alist_url, ''), COALESCE(alist_token, ''),
		create_time, update_time FROM t_cloud_115 WHERE status = $1`,
		status,
	)
	if err != nil {
		return nil, fmt.Errorf("Cloud115DAO[GetByStatus] 查询失败: %v", err)
	}
	defer rows.Close()
	var list []*domain.Cloud115
	for rows.Next() {
		cloud115 := &domain.Cloud115{}
		err := rows.Scan(
			&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken,
			&cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID,
			&cloud115.TransferDirectory, &cloud115.AccountType, &cloud115.QuotaUsed,
			&cloud115.Priority, &cloud115.Status, &cloud115.CoolingStartTime,
			&cloud115.TransferMethod, &cloud115.AlistUrl, &cloud115.AlistToken,
			&cloud115.CreateTime, &cloud115.UpdateTime,
		)
		if err != nil {
			return nil, fmt.Errorf("Cloud115DAO[GetByStatus] 扫描失败: %v", err)
		}
		list = append(list, cloud115)
	}
	return list, nil
}
