#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
测试媒体管理模块115客户端初始化修复
测试接口:
1. GET /media/files - 文件浏览接口
2. GET /media/files/search - 文件搜索接口
"""

import requests
import json

BASE_URL = "http://localhost:8082"

def login():
    """登录获取token"""
    print("=" * 60)
    print("[测试1] 用户登录")
    print("=" * 60)

    url = f"{BASE_URL}/login"
    # 密码是 "admin" 的 MD5 值
    data = {
        "name": "admin",
        "password": "21232f297a57a5a743894a0e4a801fc3"
    }

    response = requests.post(url, json=data)
    print(f"状态码: {response.status_code}")
    result = response.json()
    print(f"响应: {json.dumps(result, ensure_ascii=False, indent=2)}")

    if response.status_code == 200 and result.get("token"):
        print("✓ 登录成功")
        return result["token"]
    else:
        print("✗ 登录失败")
        return None

def get_cloud115_accounts(token):
    """获取115账号列表"""
    print("\n" + "=" * 60)
    print("[测试2] 获取115账号列表")
    print("=" * 60)

    url = f"{BASE_URL}/cloud115"
    headers = {"Authorization": f"Bearer {token}"}

    response = requests.get(url, headers=headers)
    print(f"状态码: {response.status_code}")
    result = response.json()
    print(f"响应: {json.dumps(result, ensure_ascii=False, indent=2)}")

    if response.status_code == 200:
        accounts = result.get("data", [])
        if accounts:
            print(f"✓ 找到 {len(accounts)} 个115账号")
            return accounts[0]["id"]
        else:
            print("⚠ 没有找到115账号,请先添加115账号")
            return None
    else:
        print("✗ 获取115账号列表失败")
        return None

def get_media_sources(token):
    """获取媒体源列表"""
    print("\n" + "=" * 60)
    print("[测试3] 获取媒体源列表")
    print("=" * 60)

    url = f"{BASE_URL}/media/sources"
    headers = {"Authorization": f"Bearer {token}"}

    response = requests.get(url, headers=headers)
    print(f"状态码: {response.status_code}")
    result = response.json()
    print(f"响应: {json.dumps(result, ensure_ascii=False, indent=2)}")

    if response.status_code == 200:
        # 处理嵌套的data结构
        data = result.get("data", {})
        if isinstance(data, dict):
            sources = data.get("data", [])
        else:
            sources = data if isinstance(data, list) else []

        # 查找115类型的媒体源
        for source in sources:
            if isinstance(source, dict) and source.get("source_type") == "cloud115":
                print(f"✓ 找到115媒体源: {source['name']} (ID: {source['id']})")
                return source["id"], source.get("cloud115_id")

        print("⚠ 没有找到115类型的媒体源")
        return None, None
    else:
        print("✗ 获取媒体源列表失败")
        return None, None

def create_115_media_source(token, cloud115_id):
    """创建115媒体源"""
    print("\n" + "=" * 60)
    print("[测试4] 创建115媒体源")
    print("=" * 60)

    url = f"{BASE_URL}/media/sources"
    headers = {"Authorization": f"Bearer {token}"}
    data = {
        "name": "测试115媒体源",
        "source_type": "cloud115",
        "path": "0",  # 根目录
        "cloud115_id": cloud115_id,
        "priority": 10,
        "enabled": True
    }

    response = requests.post(url, json=data, headers=headers)
    print(f"状态码: {response.status_code}")
    result = response.json()
    print(f"响应: {json.dumps(result, ensure_ascii=False, indent=2)}")

    if response.status_code == 200:
        source_id = result.get("data", {}).get("id")
        print(f"✓ 创建115媒体源成功 (ID: {source_id})")
        return source_id
    else:
        print("✗ 创建115媒体源失败")
        return None

def test_get_files(token, source_id):
    """测试文件浏览接口"""
    print("\n" + "=" * 60)
    print("[测试5] 测试文件浏览接口 GET /media/files")
    print("=" * 60)

    url = f"{BASE_URL}/media/files"
    headers = {"Authorization": f"Bearer {token}"}
    params = {
        "source_id": source_id,
        "path": "",  # 根目录
        "page": 1,
        "page_size": 10
    }

    response = requests.get(url, params=params, headers=headers)
    print(f"状态码: {response.status_code}")
    result = response.json()
    print(f"响应: {json.dumps(result, ensure_ascii=False, indent=2)}")

    if response.status_code == 200:
        total = result.get("data", {}).get("total", 0)
        files = result.get("data", {}).get("files", [])
        print(f"✓ 文件浏览成功,共 {total} 个文件")
        if files:
            print(f"  前 {len(files)} 个文件:")
            for file in files[:5]:
                file_type = "目录" if file.get("is_directory") else "文件"
                print(f"    - [{file_type}] {file.get('name')}")
        return True
    else:
        error = result.get("error", "未知错误")
        print(f"✗ 文件浏览失败: {error}")
        return False

def test_search_files(token, source_id):
    """测试文件搜索接口"""
    print("\n" + "=" * 60)
    print("[测试6] 测试文件搜索接口 GET /media/files/search")
    print("=" * 60)

    url = f"{BASE_URL}/media/files/search"
    headers = {"Authorization": f"Bearer {token}"}
    params = {
        "source_id": source_id,
        "keyword": "test",  # 搜索关键词
        "page": 1,
        "page_size": 10
    }

    response = requests.get(url, params=params, headers=headers)
    print(f"状态码: {response.status_code}")
    result = response.json()
    print(f"响应: {json.dumps(result, ensure_ascii=False, indent=2)}")

    if response.status_code == 200:
        total = result.get("data", {}).get("total", 0)
        files = result.get("data", {}).get("files", [])
        print(f"✓ 文件搜索成功,共找到 {total} 个匹配文件")
        if files:
            print(f"  前 {len(files)} 个文件:")
            for file in files[:5]:
                file_type = "目录" if file.get("is_directory") else "文件"
                print(f"    - [{file_type}] {file.get('name')}")
        return True
    else:
        error = result.get("error", "未知错误")
        print(f"✗ 文件搜索失败: {error}")
        return False

def main():
    """主测试流程"""
    print("\n" + "=" * 60)
    print("媒体管理模块115客户端初始化修复测试")
    print("=" * 60)

    # 1. 登录
    token = login()
    if not token:
        print("\n测试终止: 无法获取token")
        return

    # 2. 获取115账号
    cloud115_id = get_cloud115_accounts(token)
    if not cloud115_id:
        print("\n测试终止: 没有可用的115账号")
        return

    # 3. 获取或创建115媒体源
    source_id, existing_cloud115_id = get_media_sources(token)
    if not source_id:
        # 如果没有115媒体源,创建一个
        source_id = create_115_media_source(token, cloud115_id)
        if not source_id:
            print("\n测试终止: 无法创建115媒体源")
            return

    # 4. 测试文件浏览接口
    success1 = test_get_files(token, source_id)

    # 5. 测试文件搜索接口
    success2 = test_search_files(token, source_id)

    # 总结
    print("\n" + "=" * 60)
    print("测试总结")
    print("=" * 60)
    if success1 and success2:
        print("✓ 所有测试通过!115客户端初始化问题已修复")
    else:
        print("✗ 部分测试失败,请检查日志")
    print("=" * 60)

if __name__ == "__main__":
    main()
