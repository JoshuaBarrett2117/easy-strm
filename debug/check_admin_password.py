# -*- coding: utf-8 -*-
import psycopg2
import hashlib

# 数据库配置
DB_CONFIG = {
    "host": "192.168.31.12",
    "port": 15432,
    "database": "easy_strm",
    "user": "joshua",
    "password": "dwwzbuwioalvb"
}

# 连接数据库
conn = psycopg2.connect(**DB_CONFIG)
cursor = conn.cursor()

# 查询用户表
cursor.execute("SELECT id, username, password FROM t_user WHERE username = 'admin'")
user = cursor.fetchone()

if user:
    print(f"用户ID: {user[0]}")
    print(f"用户名: {user[1]}")
    print(f"密码MD5: {user[2]}")

    # 测试密码
    test_password = "admin"
    test_md5 = hashlib.md5(test_password.encode()).hexdigest()
    print(f"\n测试密码 '{test_password}' 的MD5: {test_md5}")
    print(f"是否匹配: {test_md5 == user[2]}")
else:
    print("未找到admin用户")

cursor.close()
conn.close()
