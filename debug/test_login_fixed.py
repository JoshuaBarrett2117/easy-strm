#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
简单的登录测试 - 修正版
"""

import requests
import json

BASE_URL = "http://localhost:8082"

# 测试登录 - 使用正确的接口和参数
url = f"{BASE_URL}/login"
data = {
    "name": "admin",
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

        # 提取token
        if "token" in result:
            print(f"\n✅ 登录成功，获取到token: {result['token'][:20]}...")
    except Exception as e:
        print(f"\nJSON解析失败: {e}")

except Exception as e:
    print(f"请求异常: {e}")
