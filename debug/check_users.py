import psycopg2
import os

# 数据库配置
DB_CONFIG = {
    'host': '192.168.31.12',
    'port': 15432,
    'database': 'easystrm',
    'user': 'joshua',
    'password': 'dwwzbuwioalvb'
}

def check_users():
    """检查数据库中的用户信息"""
    try:
        conn = psycopg2.connect(**DB_CONFIG)
        cursor = conn.cursor()

        # 查询用户表
        cursor.execute("SELECT id, username, created_at FROM users ORDER BY id")
        users = cursor.fetchall()

        print("数据库中的用户列表：")
        print("-" * 60)
        for user in users:
            print(f"ID: {user[0]}, 用户名: {user[1]}, 创建时间: {user[2]}")

        cursor.close()
        conn.close()

    except Exception as e:
        print(f"数据库连接失败: {e}")

if __name__ == "__main__":
    check_users()
