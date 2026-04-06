#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
查询数据库表结构
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

def check_table_structure():
    """检查数据库表结构"""
    try:
        conn = psycopg2.connect(**DB_CONFIG)
        print("[数据库连接] 成功")

        cursor = conn.cursor(cursor_factory=RealDictCursor)

        # 查询t_cloud_115表结构
        print("\n[t_cloud_115表结构] 查询表结构...")
        cursor.execute("""
            SELECT column_name, data_type, is_nullable, column_default
            FROM information_schema.columns
            WHERE table_name = 't_cloud_115'
            ORDER BY ordinal_position
        """)
        columns = cursor.fetchall()

        print(f"\n[t_cloud_115表结构] 共有 {len(columns)} 个字段:")
        for col in columns:
            print(f"  - {col['column_name']}: {col['data_type']}, nullable={col['is_nullable']}, default={col['column_default']}")

        cursor.close()
        conn.close()

    except Exception as e:
        print(f"[错误] 查询表结构失败: {e}")
        import traceback
        traceback.print_exc()

if __name__ == "__main__":
    check_table_structure()
