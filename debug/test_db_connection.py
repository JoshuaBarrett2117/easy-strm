#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
数据库连接测试脚本
用于验证数据库连接和查询115账号信息
"""

import psycopg2
from psycopg2.extras import RealDictCursor

# 数据库连接配置
DB_CONFIG = {
    "host": "192.168.31.12",
    "port": 15432,
    "user": "joshua",
    "password": "jcboom17",
    "database": "easy_strm"
}

def test_connection():
    """测试数据库连接"""
    try:
        conn = psycopg2.connect(**DB_CONFIG)
        print("[数据库连接] 成功")

        cursor = conn.cursor(cursor_factory=RealDictCursor)

        # 查询115账号
        print("\n[查询115账号] 执行SQL查询...")
        sql = """
        SELECT id, name, cookie, refresh_token, access_token, expires_in,
        COALESCE(transfer_account_id, 0) as transfer_account_id,
        COALESCE(transfer_directory, '') as transfer_directory,
        COALESCE(account_type, 'resource') as account_type,
        COALESCE(quota_used, 0) as quota_used,
        COALESCE(priority, 5) as priority,
        COALESCE(status, 'active') as status,
        cooling_start_time,
        transfer_method,
        COALESCE(alist_url, '') as alist_url,
        COALESCE(alist_token, '') as alist_token,
        create_time, update_time
        FROM t_cloud_115
        WHERE id = 2
        """
        cursor.execute(sql)
        result = cursor.fetchone()

        if result:
            print(f"[查询115账号] 成功获取账号: {result['name']} (ID: {result['id']})")
            print(f"[查询115账号] 账号类型: {result['account_type']}")
            print(f"[查询115账号] 状态: {result['status']}")
            print(f"[查询115账号] Cookie长度: {len(result['cookie']) if result['cookie'] else 0}")
        else:
            print("[查询115账号] 未找到ID为2的账号")

        # 查询所有115账号
        print("\n[查询所有115账号] 执行SQL查询...")
        cursor.execute("SELECT id, name, status FROM t_cloud_115")
        all_accounts = cursor.fetchall()
        print(f"[查询所有115账号] 共找到 {len(all_accounts)} 个账号:")
        for acc in all_accounts:
            print(f"  - ID: {acc['id']}, 名称: {acc['name']}, 状态: {acc['status']}")

        cursor.close()
        conn.close()

    except Exception as e:
        print(f"[错误] 数据库操作失败: {e}")
        import traceback
        traceback.print_exc()

if __name__ == "__main__":
    test_connection()
