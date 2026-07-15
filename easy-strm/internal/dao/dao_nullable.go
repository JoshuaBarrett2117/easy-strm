package dao

import "time"

// nullString 将空字符串转换为数据库 NULL，避免写入无意义空值。
func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// nullInt 将零值转换为数据库 NULL，匹配可选数字字段语义。
func nullInt(i int) interface{} {
	if i == 0 {
		return nil
	}
	return i
}

// nullTime 将空时间指针转换为数据库 NULL。
func nullTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}
