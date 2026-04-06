#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
简单的登录测试
"""

import requests
import json

BASE_URL = "http://localhost:8082"

# 测试登录
url = f"{BASE_URL}/api/auth/login"
data = {
    "username": "admin",
    "password": "admin123"
}

print(f"请求URL: {url}")
print(f"请求数据: {json.dumps(data, ensure_ascii=False)}")

try:
    response = requests.post(url, json=data, timeout=10)
    print(f"\n状态码: {response.status_code}")
    print(f"响应内容: {response.text}")

    # 尝试解析JSON
    try:
        result = response.json()
        print(f"\nJSON解析成功:")
        print(json.dumps(result, ensure_ascii=False, indent=2))
    except Exception as e:
        print(f"\nJSON解析失败: {e}")

except Exception as e:
    print(f"请求异常: {e}")
