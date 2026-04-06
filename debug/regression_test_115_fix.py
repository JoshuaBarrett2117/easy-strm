#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
媒体管理接口回归测试脚本
测试目标：验证115客户端初始化问题的修复效果
"""

import requests
import json
import sys
import hashlib
from datetime import datetime

# 配置
BASE_URL = "http://localhost:8082"
ADMIN_USERNAME = "admin"
ADMIN_PASSWORD = "admin"  # 原始密码，会在登录时转换为MD5

# 测试结果收集
test_results = {
    "total": 0,
    "passed": 0,
    "failed": 0,
    "errors": [],
    "start_time": datetime.now().strftime("%Y-%m-%d %H:%M:%S")
}

def log_test(api_name, method, url, status, response_data, error_msg=None):
    """记录测试结果"""
    test_results["total"] += 1
    result = {
        "api": api_name,
        "method": method,
        "url": url,
        "status": status,
        "response": response_data,
        "error": error_msg,
        "timestamp": datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    }

    if status == "PASS":
        test_results["passed"] += 1
        print(f"✅ [{api_name}] {method} {url} - 通过")
    else:
        test_results["failed"] += 1
        test_results["errors"].append(result)
        print(f"❌ [{api_name}] {method} {url} - 失败")
        if error_msg:
            print(f"   错误信息: {error_msg}")
        if response_data:
            print(f"   响应数据: {json.dumps(response_data, ensure_ascii=False, indent=2)}")

def login():
    """登录获取token"""
    print("\n" + "="*80)
    print("步骤1: 用户登录")
    print("="*80)

    url = f"{BASE_URL}/login"
    # 密码需要MD5加密
    password_md5 = hashlib.md5(ADMIN_PASSWORD.encode()).hexdigest()
    data = {
        "name": ADMIN_USERNAME,
        "password": password_md5
    }

    try:
        response = requests.post(url, json=data, timeout=10)
        result = response.json()

        if response.status_code == 200 and "token" in result:
            token = result.get("token")
            if token:
                print(f"✅ 登录成功，获取到token: {token[:20]}...")
                return token
            else:
                print(f"❌ 登录失败：响应中未找到token")
                return None
        else:
            print(f"❌ 登录失败：{result.get('error', '未知错误')}")
            return None
    except Exception as e:
        print(f"❌ 登录异常：{str(e)}")
        return None

def test_get_media_sources(token):
    """测试获取媒体源列表"""
    print("\n" + "="*80)
    print("步骤2: 测试获取媒体源列表 GET /media/sources")
    print("="*80)

    url = f"{BASE_URL}/media/sources"
    headers = {"Authorization": f"Bearer {token}"}

    try:
        response = requests.get(url, headers=headers, timeout=10)
        result = response.json()

        # 适配实际的响应格式: {"state": true, "code": 0, "data": {"data": [...], "total": N}}
        if response.status_code == 200 and result.get("state") == True:
            data_wrapper = result.get("data", {})
            data = data_wrapper.get("data", [])
            total = data_wrapper.get("total", 0)
            log_test("获取媒体源列表", "GET", url, "PASS", {
                "total": total,
                "sources_count": len(data)
            })
            return data
        else:
            log_test("获取媒体源列表", "GET", url, "FAIL", result, result.get("message"))
            return []
    except Exception as e:
        log_test("获取媒体源列表", "GET", url, "FAIL", None, str(e))
        return []

def test_get_cloud115_sources(token, sources):
    """测试查询所有115数据源"""
    print("\n" + "="*80)
    print("步骤3: 查询所有115数据源")
    print("="*80)

    # 查询所有115数据源(包括禁用的)
    cloud115_sources = [s for s in sources if s.get("source_type") == "cloud115"]
    print(f"找到 {len(cloud115_sources)} 个115数据源")

    for source in cloud115_sources:
        status = "启用" if source.get("enabled") else "禁用"
        print(f"\n  - ID: {source.get('id')}, 名称: {source.get('name')}, Cloud115ID: {source.get('cloud115_id')}, 状态: {status}")

    # 返回所有115数据源(包括禁用的),用于后续测试
    return cloud115_sources

def test_get_files(token, source_id, source_name):
    """测试文件浏览接口"""
    print("\n" + "="*80)
    print(f"步骤4: 测试文件浏览接口 GET /media/files (媒体源: {source_name})")
    print("="*80)

    url = f"{BASE_URL}/media/files"
    headers = {"Authorization": f"Bearer {token}"}
    params = {
        "source_id": source_id,
        "path": "",  # 浏览根目录
        "page": 1,
        "page_size": 20
    }

    try:
        response = requests.get(url, headers=headers, params=params, timeout=15)
        result = response.json()

        # 适配实际的响应格式
        if response.status_code == 200 and result.get("state") == True:
            data = result.get("data", {})
            files = data.get("files", [])
            total = data.get("total", 0)

            log_test(f"文件浏览-{source_name}", "GET", f"{url}?source_id={source_id}", "PASS", {
                "total": total,
                "files_count": len(files),
                "page": data.get("page"),
                "page_size": data.get("page_size")
            })

            # 显示部分文件信息
            if files:
                print(f"\n  前5个文件:")
                for i, file in enumerate(files[:5], 1):
                    print(f"    {i}. {file.get('name')} ({file.get('type')}, {file.get('size')} bytes)")

            return True
        else:
            error_msg = result.get("message", "未知错误")
            # 检查是否是"115客户端未初始化"错误
            if "115客户端未初始化" in error_msg or "未初始化" in error_msg:
                log_test(f"文件浏览-{source_name}", "GET", f"{url}?source_id={source_id}", "FAIL",
                        result, f"关键错误: {error_msg}")
            else:
                log_test(f"文件浏览-{source_name}", "GET", f"{url}?source_id={source_id}", "FAIL",
                        result, error_msg)
            return False
    except Exception as e:
        log_test(f"文件浏览-{source_name}", "GET", f"{url}?source_id={source_id}", "FAIL", None, str(e))
        return False

def test_search_files(token, source_id, source_name):
    """测试文件搜索接口"""
    print("\n" + "="*80)
    print(f"步骤5: 测试文件搜索接口 GET /media/files/search (媒体源: {source_name})")
    print("="*80)

    url = f"{BASE_URL}/media/files/search"
    headers = {"Authorization": f"Bearer {token}"}
    params = {
        "source_id": source_id,
        "keyword": "test",  # 搜索关键词
        "page": 1,
        "page_size": 20
    }

    try:
        response = requests.get(url, headers=headers, params=params, timeout=15)
        result = response.json()

        # 适配实际的响应格式
        if response.status_code == 200 and result.get("state") == True:
            data = result.get("data", {})
            files = data.get("files", [])
            total = data.get("total", 0)

            log_test(f"文件搜索-{source_name}", "GET", f"{url}?source_id={source_id}&keyword=test", "PASS", {
                "total": total,
                "files_count": len(files)
            })
            return True
        else:
            error_msg = result.get("message", "未知错误")
            # 检查是否是"115客户端未初始化"错误
            if "115客户端未初始化" in error_msg or "未初始化" in error_msg:
                log_test(f"文件搜索-{source_name}", "GET", f"{url}?source_id={source_id}&keyword=test", "FAIL",
                        result, f"关键错误: {error_msg}")
            else:
                log_test(f"文件搜索-{source_name}", "GET", f"{url}?source_id={source_id}&keyword=test", "FAIL",
                        result, error_msg)
            return False
    except Exception as e:
        log_test(f"文件搜索-{source_name}", "GET", f"{url}?source_id={source_id}&keyword=test", "FAIL", None, str(e))
        return False

def test_create_media_source(token):
    """测试创建媒体源"""
    print("\n" + "="*80)
    print("步骤6: 测试创建媒体源 POST /media/sources")
    print("="*80)

    url = f"{BASE_URL}/media/sources"
    headers = {"Authorization": f"Bearer {token}"}
    data = {
        "name": "测试本地媒体源",
        "source_type": "local",
        "path": "C:\\test_media",
        "priority": 99,
        "enabled": True
    }

    try:
        response = requests.post(url, json=data, headers=headers, timeout=10)
        result = response.json()

        # 适配实际的响应格式
        if response.status_code == 200 and result.get("state") == True:
            data_wrapper = result.get("data", {})
            source_data = data_wrapper.get("data", {})
            log_test("创建媒体源", "POST", url, "PASS", {
                "id": source_data.get("id"),
                "name": source_data.get("name")
            })
            return source_data.get("id")
        else:
            log_test("创建媒体源", "POST", url, "FAIL", result, result.get("message"))
            return None
    except Exception as e:
        log_test("创建媒体源", "POST", url, "FAIL", None, str(e))
        return None

def test_update_media_source(token, source_id):
    """测试更新媒体源"""
    print("\n" + "="*80)
    print(f"步骤7: 测试更新媒体源 PUT /media/sources/{source_id}")
    print("="*80)

    url = f"{BASE_URL}/media/sources/{source_id}"
    headers = {"Authorization": f"Bearer {token}"}
    data = {
        "name": "测试本地媒体源-已更新",
        "priority": 88
    }

    try:
        response = requests.put(url, json=data, headers=headers, timeout=10)
        result = response.json()

        # 适配实际的响应格式
        if response.status_code == 200 and result.get("state") == True:
            data_wrapper = result.get("data", {})
            log_test("更新媒体源", "PUT", url, "PASS", data_wrapper.get("data"))
            return True
        else:
            log_test("更新媒体源", "PUT", url, "FAIL", result, result.get("message"))
            return False
    except Exception as e:
        log_test("更新媒体源", "PUT", url, "FAIL", None, str(e))
        return False

def test_get_media_source_by_id(token, source_id):
    """测试获取单个媒体源"""
    print("\n" + "="*80)
    print(f"步骤8: 测试获取单个媒体源 GET /media/sources/{source_id}")
    print("="*80)

    url = f"{BASE_URL}/media/sources/{source_id}"
    headers = {"Authorization": f"Bearer {token}"}

    try:
        response = requests.get(url, headers=headers, timeout=10)
        result = response.json()

        # 适配实际的响应格式
        if response.status_code == 200 and result.get("state") == True:
            log_test("获取单个媒体源", "GET", url, "PASS", result.get("data"))
            return True
        else:
            log_test("获取单个媒体源", "GET", url, "FAIL", result, result.get("message"))
            return False
    except Exception as e:
        log_test("获取单个媒体源", "GET", url, "FAIL", None, str(e))
        return False

def test_delete_media_source(token, source_id):
    """测试删除媒体源"""
    print("\n" + "="*80)
    print(f"步骤9: 测试删除媒体源 DELETE /media/sources/{source_id}")
    print("="*80)

    url = f"{BASE_URL}/media/sources/{source_id}"
    headers = {"Authorization": f"Bearer {token}"}

    try:
        response = requests.delete(url, headers=headers, timeout=10)
        result = response.json()

        # 适配实际的响应格式
        if response.status_code == 200 and result.get("state") == True:
            log_test("删除媒体源", "DELETE", url, "PASS", result)
            return True
        else:
            log_test("删除媒体源", "DELETE", url, "FAIL", result, result.get("message"))
            return False
    except Exception as e:
        log_test("删除媒体源", "DELETE", url, "FAIL", None, str(e))
        return False

def generate_report():
    """生成测试报告"""
    print("\n" + "="*80)
    print("回归测试报告")
    print("="*80)

    print(f"\n测试时间: {test_results['start_time']}")
    print(f"测试总数: {test_results['total']}")
    print(f"通过数量: {test_results['passed']}")
    print(f"失败数量: {test_results['failed']}")
    print(f"通过率: {(test_results['passed']/test_results['total']*100):.2f}%" if test_results['total'] > 0 else "通过率: 0%")

    if test_results['errors']:
        print("\n" + "="*80)
        print("失败详情:")
        print("="*80)
        for i, error in enumerate(test_results['errors'], 1):
            print(f"\n{i}. API: {error['api']}")
            print(f"   方法: {error['method']}")
            print(f"   URL: {error['url']}")
            print(f"   时间: {error['timestamp']}")
            if error['error']:
                print(f"   错误: {error['error']}")
            if error['response']:
                print(f"   响应: {json.dumps(error['response'], ensure_ascii=False, indent=2)}")

    # 保存报告到文件
    report_file = "debug/regression_test_report.json"
    with open(report_file, 'w', encoding='utf-8') as f:
        json.dump(test_results, f, ensure_ascii=False, indent=2)
    print(f"\n详细报告已保存至: {report_file}")

def main():
    """主测试流程"""
    print("="*80)
    print("媒体管理接口回归测试")
    print("测试目标: 验证115客户端初始化问题的修复效果")
    print("="*80)

    # 1. 登录
    token = login()
    if not token:
        print("\n❌ 登录失败，无法继续测试")
        return

    # 2. 获取媒体源列表
    sources = test_get_media_sources(token)

    # 3. 查询所有115数据源
    cloud115_sources = test_get_cloud115_sources(token, sources)

    # 4. 测试每个115数据源的文件浏览接口
    print("\n" + "="*80)
    print("重点测试: 115数据源文件浏览接口")
    print("="*80)

    if cloud115_sources:
        for source in cloud115_sources:
            test_get_files(token, source['id'], source['name'])
            test_search_files(token, source['id'], source['name'])
    else:
        print("\n⚠️  未找到115数据源，跳过115相关测试")
        print("提示: 请先在系统中创建115数据源")

    # 5. 测试CRUD接口
    print("\n" + "="*80)
    print("测试媒体源CRUD接口")
    print("="*80)

    created_id = test_create_media_source(token)
    if created_id:
        test_get_media_source_by_id(token, created_id)
        test_update_media_source(token, created_id)
        test_delete_media_source(token, created_id)

    # 6. 生成测试报告
    generate_report()

    # 返回退出码
    sys.exit(0 if test_results['failed'] == 0 else 1)

if __name__ == "__main__":
    main()
