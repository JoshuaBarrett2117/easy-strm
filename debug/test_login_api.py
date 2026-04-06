# -*- coding: utf-8 -*-
"""
测试登录 API
"""
import requests
import hashlib
import json

BASE_URL = "http://localhost:8082"

def test_login(username, password):
    """测试登录"""
    # 计算 MD5 密码
    md5_password = hashlib.md5(password.encode()).hexdigest()

    print(f"\n{'='*60}")
    print(f"测试登录: {username} / {password}")
    print(f"MD5密码: {md5_password}")
    print('='*60)

    try:
        response = requests.post(
            f"{BASE_URL}/auth/login",
            json={
                "name": username,
                "password": md5_password
            },
            timeout=5
        )

        print(f"状态码: {response.status_code}")
        print(f"响应头: {dict(response.headers)}")

        try:
            data = response.json()
            print(f"响应数据: {json.dumps(data, ensure_ascii=False, indent=2)}")
        except:
            print(f"响应文本: {response.text}")

        return response.status_code == 200

    except Exception as e:
        print(f"❌ 请求失败: {e}")
        return False

if __name__ == "__main__":
    # 测试常见密码
    test_passwords = [
        ('admin', 'admin'),
        ('admin', '123456'),
        ('admin', 'password'),
        ('admin', 'admin123'),
        ('test', 'test'),
    ]

    for username, password in test_passwords:
        if test_login(username, password):
            print(f"\n✅ 登录成功: {username} / {password}")
            break
        else:
            print(f"❌ 登录失败: {username} / {password}")
