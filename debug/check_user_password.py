#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
检查数据库中的用户信息
"""

import psycopg2
import hashlib

# 数据库连接配置
DB_CONFIG = {
    'host': '192.168.31.12',
    'port': 15432,
    'user': 'joshua',
    'password': 'dwwzbuwioalvb',
    'database': 'easy_strm'
}

def check_users():
    """检查数据库中的用户信息"""
    try:
        conn = psycopg2.connect(**DB_CONFIG)
        cursor = conn.cursor()

        # 查询所有用户
        cursor.execute("SELECT id, name, password, role FROM users")
        users = cursor.fetchall()

        print("=" * 80)
        print("数据库中的用户信息：")
        print("=" * 80)
        for user in users:
            user_id, name, password, role = user
            print(f"ID: {user_id}")
            print(f"用户名: {name}")
            print(f"密码哈希: {password}")
            print(f"角色: {role}")
            print("-" * 80)

        # 测试密码
        test_passwords = ['admin', 'admin123', '123456']
        print("\n测试密码哈希：")
        print("=" * 80)
        for pwd in test_passwords:
            md5_hash = hashlib.md5(pwd.encode()).hexdigest()
            print(f"密码: {pwd}")
            print(f"MD5哈希: {md5_hash}")
            print("-" * 80)

        cursor.close()
        conn.close()

    except Exception as e:
        print(f"错误: {e}")

if __name__ == "__main__":
    check_users()
