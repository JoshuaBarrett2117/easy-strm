package dao

import (
	"database/sql"
	"fmt"

	"easy-strm/internal/domain"
	"easy-strm/internal/pkg/logger"
)

// Cloud115DAO 115云账号数据访问层
type Cloud115DAO struct{}

// NewCloud115DAO 创建115云账号DAO实例
func NewCloud115DAO() *Cloud115DAO {
	return &Cloud115DAO{}
}

// GetByID 根据ID获取115云账号
func (c *Cloud115DAO) GetByID(id int) (*domain.Cloud115, error) {
	cloud115 := &domain.Cloud115{}
	err := db.QueryRow(
		`SELECT id, name, cookie, refresh_token, access_token, expires_in,
		COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''),
		create_time, update_time FROM t_cloud_115 WHERE id = $1`,
		id,
	).Scan(
		&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken,
		&cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID,
		&cloud115.TransferDirectory, &cloud115.CreateTime, &cloud115.UpdateTime,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("Cloud115DAO[GetByID] 查询失败: %v", err)
	}
	return cloud115, nil
}

// GetAll 获取所有115云账号
func (c *Cloud115DAO) GetAll(sortField, sortOrder string) ([]*domain.Cloud115, error) {
	orderClause := "id ASC"
	if sortField != "" {
		orderClause = fmt.Sprintf("%s %s", sortField, sortOrder)
		if sortOrder == "" {
			orderClause = fmt.Sprintf("%s ASC", sortField)
		}
	}

	rows, err := db.Query(
		fmt.Sprintf(`SELECT id, name, cookie, refresh_token, access_token, expires_in,
		COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''),
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
			&cloud115.TransferDirectory, &cloud115.CreateTime, &cloud115.UpdateTime,
		)
		if err != nil {
			return nil, fmt.Errorf("Cloud115DAO[GetAll] 扫描失败: %v", err)
		}
		list = append(list, cloud115)
	}
	return list, nil
}

// Create 创建115云账号
func (c *Cloud115DAO) Create(name, cookie, refreshToken, accessToken string, expiresIn, transferAccountID int, transferDirectory string) (*domain.Cloud115, error) {
	cloud115 := &domain.Cloud115{}
	err := db.QueryRow(
		`INSERT INTO t_cloud_115 (name, cookie, refresh_token, access_token, expires_in, transfer_account_id, transfer_directory)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, name, cookie, refresh_token, access_token, expires_in,
		COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''), create_time, update_time`,
		name, cookie, refreshToken, accessToken, expiresIn, transferAccountID, transferDirectory,
	).Scan(
		&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken,
		&cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID,
		&cloud115.TransferDirectory, &cloud115.CreateTime, &cloud115.UpdateTime,
	)
	if err != nil {
		return nil, fmt.Errorf("Cloud115DAO[Create] 创建失败: %v", err)
	}
	return cloud115, nil
}

// Update 更新115云账号
func (c *Cloud115DAO) Update(id int, name, cookie, refreshToken, accessToken string, expiresIn, transferAccountID int, transferDirectory string) (*domain.Cloud115, error) {
	cloud115 := &domain.Cloud115{}
	err := db.QueryRow(
		`UPDATE t_cloud_115 SET name=$2, cookie=$3, refresh_token=$4, access_token=$5,
		expires_in=$6, transfer_account_id=$7, transfer_directory=$8
		WHERE id=$1
		RETURNING id, name, cookie, refresh_token, access_token, expires_in,
		COALESCE(transfer_account_id, 0), COALESCE(transfer_directory, ''), create_time, update_time`,
		id, name, cookie, refreshToken, accessToken, expiresIn, transferAccountID, transferDirectory,
	).Scan(
		&cloud115.ID, &cloud115.Name, &cloud115.Cookie, &cloud115.RefreshToken,
		&cloud115.AccessToken, &cloud115.ExpiresIn, &cloud115.TransferAccountID,
		&cloud115.TransferDirectory, &cloud115.CreateTime, &cloud115.UpdateTime,
	)
	if err != nil {
		return nil, fmt.Errorf("Cloud115DAO[Update] 更新失败: %v", err)
	}
	return cloud115, nil
}

// Delete 删除115云账号
func (c *Cloud115DAO) Delete(id int) error {
	result, err := db.Exec("DELETE FROM t_cloud_115 WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("Cloud115DAO[Delete] 删除失败: %v", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("Cloud115DAO[Delete] 账号不存在")
	}
	return nil
}
