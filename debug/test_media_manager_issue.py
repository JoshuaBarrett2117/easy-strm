#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
媒体管理模块问题排查脚本
用于测试新增115云盘数据源和浏览文件列表功能
"""

import requests
import json
import hashlib

BASE_URL = "http://localhost:8082"

def login():
    """登录获取token"""
    url = f"{BASE_URL}/login"
    # 前端会对密码进行MD5哈希，所以这里也使用MD5哈希值
    password_md5 = hashlib.md5("admin".encode()).hexdigest()
    data = {
        "name": "admin",  # 注意：前端使用的是"name"字段，而不是"username"
        "password": password_md5
    }
    print(f"[登录] 密码MD5: {password_md5}")
    response = requests.post(url, json=data)
    print(f"[登录] 状态码: {response.status_code}")
    print(f"[登录] 响应: {response.text}")

    if response.status_code == 200:
        result = response.json()
        token = result.get("token")  # 注意：token在根级别，不在data中
        print(f"[登录] Token: {token}")
        return token
    return None

def get_115_accounts(token):
    """获取115账号列表"""
    url = f"{BASE_URL}/cloud115"
    headers = {"Authorization": f"Bearer {token}"}
    response = requests.get(url, headers=headers)
    print(f"\n[获取115账号] 状态码: {response.status_code}")
    print(f"[获取115账号] 响应: {response.text}")
    return response.json()

def create_media_source(token, name, source_type, path, cloud115_id=None):
    """创建媒体源"""
    url = f"{BASE_URL}/media/sources"
    headers = {"Authorization": f"Bearer {token}"}
    data = {
        "name": name,
        "source_type": source_type,
        "path": path,
        "cloud115_id": cloud115_id,
        "priority": 10,
        "enabled": True
    }
    print(f"\n[创建媒体源] 请求数据: {json.dumps(data, ensure_ascii=False, indent=2)}")
    response = requests.post(url, json=data, headers=headers)
    print(f"[创建媒体源] 状态码: {response.status_code}")
    print(f"[创建媒体源] 响应: {response.text}")
    return response

def get_media_sources(token):
    """获取媒体源列表"""
    url = f"{BASE_URL}/media/sources"
    headers = {"Authorization": f"Bearer {token}"}
    response = requests.get(url, headers=headers)
    print(f"\n[获取媒体源列表] 状态码: {response.status_code}")
    print(f"[获取媒体源列表] 响应: {response.text}")
    return response.json()

def get_files(token, source_id, path=""):
    """获取文件列表"""
    url = f"{BASE_URL}/media/files"
    headers = {"Authorization": f"Bearer {token}"}
    params = {
        "source_id": source_id,
        "path": path,
        "page": 1,
        "page_size": 50
    }
    print(f"\n[获取文件列表] 请求参数: {json.dumps(params, ensure_ascii=False, indent=2)}")
    response = requests.get(url, params=params, headers=headers)
    print(f"[获取文件列表] 状态码: {response.status_code}")
    print(f"[获取文件列表] 响应: {response.text}")
    return response

def main():
    print("=" * 80)
    print("媒体管理模块问题排查")
    print("=" * 80)

    # 1. 登录
    token = login()
    if not token:
        print("登录失败，退出测试")
        return

    # 2. 获取115账号列表
    accounts_result = get_115_accounts(token)
    accounts = accounts_result.get("data", [])

    if not accounts:
        print("\n没有可用的115账号，无法继续测试")
        return

    # 使用第一个115账号
    first_account = accounts[0]
    cloud115_id = first_account.get("id")
    print(f"\n使用115账号: {first_account.get('name')} (ID: {cloud115_id})")

    # 3. 创建115云盘媒体源
    print("\n" + "=" * 80)
    print("测试1: 创建115云盘媒体源")
    print("=" * 80)
    create_response = create_media_source(
        token=token,
        name="测试115媒体源",
        source_type="cloud115",
        path="0",  # 根目录
        cloud115_id=cloud115_id
    )

    # 4. 获取媒体源列表
    print("\n" + "=" * 80)
    print("测试2: 获取媒体源列表")
    print("=" * 80)
    sources_result = get_media_sources(token)
    sources = sources_result.get("data", [])

    if not sources:
        print("\n没有媒体源，无法继续测试文件列表")
        return

    # 使用第一个媒体源
    first_source = sources[0]
    source_id = first_source.get("id")
    print(f"\n使用媒体源: {first_source.get('name')} (ID: {source_id})")

    # 5. 获取文件列表
    print("\n" + "=" * 80)
    print("测试3: 获取115云盘文件列表")
    print("=" * 80)
    get_files(token, source_id, path="")

if __name__ == "__main__":
    main()
