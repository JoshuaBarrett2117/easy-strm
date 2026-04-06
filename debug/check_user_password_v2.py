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
    'database': 'easystrm',
    'user': 'joshua',
    'password': 'dwwzbuwioalvb'
}

def check_users():
    """检查用户表"""
    try:
        conn = psycopg2.connect(**DB_CONFIG)
        cursor = conn.cursor()

        # 查询用户表
        cursor.execute("SELECT id, name, password, role FROM users")
        users = cursor.fetchall()

        print("="*60)
        print("数据库中的用户信息:")
        print("="*60)
        for user in users:
            user_id, name, password, role = user
            print(f"\n用户ID: {user_id}")
            print(f"用户名: {name}")
            print(f"密码(MD5): {password}")
            print(f"角色: {role}")

            # 测试常见密码
            test_passwords = ['admin', '123456', 'password', 'admin123']
            print(f"\n测试常见密码:")
            for pwd in test_passwords:
                md5_pwd = hashlib.md5(pwd.encode()).hexdigest()
                match = "✓ 匹配" if md5_pwd == password else "✗ 不匹配"
                print(f"  {pwd}: {md5_pwd[:16]}... {match}")

        cursor.close()
        conn.close()

    except Exception as e:
        print(f"❌ 数据库连接失败: {e}")

if __name__ == "__main__":
    check_users()
