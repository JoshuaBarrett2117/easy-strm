# -*- coding: utf-8 -*-
"""
媒体管理接口全面测试脚本 - 简化版
"""
import requests
import json
import hashlib
from datetime import datetime

BASE_URL = "http://localhost:8082"
test_results = {"pass": [], "fail": [], "warnings": []}

def log_test(category, test_name, status, message="", details=None):
    result = {
        "category": category,
        "test_name": test_name,
        "status": status,
        "message": message,
        "details": details,
        "timestamp": datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    }

    if status == "PASS":
        test_results["pass"].append(result)
        print(f"[PASS] [{category}] {test_name}: {message}")
    elif status == "FAIL":
        test_results["fail"].append(result)
        print(f"[FAIL] [{category}] {test_name}: {message}")
    else:
        test_results["warnings"].append(result)
        print(f"[WARN] [{category}] {test_name}: {message}")

    if details:
        print(f"   Details: {json.dumps(details, ensure_ascii=False, indent=2)}")

def get_token():
    password_md5 = hashlib.md5("admin".encode()).hexdigest()
    response = requests.post(f"{BASE_URL}/login", json={"name": "admin", "password": password_md5})
    if response.status_code == 200:
        return response.json().get("token")
    return None

def test_media_sources(token):
    """测试媒体源CRUD接口"""
    headers = {"Authorization": f"Bearer {token}"}

    # 1. 获取媒体源列表
    print("\n========== 测试获取媒体源列表 ==========")
    response = requests.get(f"{BASE_URL}/media/sources", headers=headers)
    data = response.json()

    if response.status_code == 200 and data.get("state"):
        sources = data.get("data", {}).get("data", [])
        log_test("媒体源API", "获取媒体源列表", "PASS", f"成功获取 {len(sources)} 个媒体源")

        # 找出115数据源
        cloud115_sources = [s for s in sources if s.get("source_type") == "cloud115"]
        print(f"\n找到 {len(cloud115_sources)} 个115数据源:")
        for s in cloud115_sources:
            print(f"  - ID: {s['id']}, 名称: {s['name']}, cloud115_id: {s.get('cloud115_id')}")

        return sources, cloud115_sources
    else:
        log_test("媒体源API", "获取媒体源列表", "FAIL", f"失败: {data.get('message')}")
        return [], []

def test_create_media_source(token):
    """测试创建媒体源"""
    headers = {"Authorization": f"Bearer {token}"}

    # 创建本地媒体源
    print("\n========== 测试创建本地媒体源 ==========")
    create_data = {
        "name": f"测试本地媒体源_{datetime.now().strftime('%H%M%S')}",
        "source_type": "local",
        "path": "D:\\Media\\Test",
        "priority": 99,
        "enabled": True
    }

    response = requests.post(f"{BASE_URL}/media/sources", headers=headers, json=create_data)
    data = response.json()

    if response.status_code == 200 and data.get("state"):
        source_id = data.get("data", {}).get("data", {}).get("id")
        log_test("媒体源API", "创建本地媒体源", "PASS", f"成功创建, ID: {source_id}")

        # 更新
        print("\n========== 测试更新媒体源 ==========")
        update_data = {"name": f"已更新_{create_data['name']}", "priority": 88}
        response = requests.put(f"{BASE_URL}/media/sources/{source_id}", headers=headers, json=update_data)
        data = response.json()

        if response.status_code == 200 and data.get("state"):
            log_test("媒体源API", "更新媒体源", "PASS", f"成功更新媒体源 ID: {source_id}")
        else:
            log_test("媒体源API", "更新媒体源", "FAIL", f"更新失败: {data.get('message')}")

        # 删除
        print("\n========== 测试删除媒体源 ==========")
        response = requests.delete(f"{BASE_URL}/media/sources/{source_id}", headers=headers)
        data = response.json()

        if response.status_code == 200 and data.get("state"):
            log_test("媒体源API", "删除媒体源", "PASS", f"成功删除媒体源 ID: {source_id}")
        else:
            log_test("媒体源API", "删除媒体源", "FAIL", f"删除失败: {data.get('message')}")
    else:
        log_test("媒体源API", "创建本地媒体源", "FAIL", f"创建失败: {data.get('message')}")

def test_file_browsing(token, cloud115_sources):
    """测试文件浏览接口"""
    headers = {"Authorization": f"Bearer {token}"}

    print("\n========== 测试文件浏览接口 ==========")

    for source in cloud115_sources:
        source_id = source["id"]
        source_name = source["name"]

        print(f"\n--- 测试数据源: {source_name} (ID: {source_id}) ---")

        # 浏览根目录
        response = requests.get(
            f"{BASE_URL}/media/files",
            headers=headers,
            params={"source_id": source_id, "path": "", "page": 1, "page_size": 20}
        )
        data = response.json()

        if response.status_code == 200 and data.get("state"):
            result = data.get("data", {})
            files = result.get("files", [])
            total = result.get("total", 0)

            log_test("文件浏览", f"浏览根目录-{source_name}", "PASS", f"成功获取 {total} 个文件/目录")

            # 显示前3个文件
            if files:
                print(f"  前3个文件/目录:")
                for f in files[:3]:
                    print(f"    - {f['name']} ({'目录' if f['is_directory'] else '文件'})")
        else:
            log_test("文件浏览", f"浏览根目录-{source_name}", "FAIL",
                    f"获取失败: {data.get('message')}", {"response": data})

        # 测试搜索
        response = requests.get(
            f"{BASE_URL}/media/files/search",
            headers=headers,
            params={"source_id": source_id, "keyword": "mp4", "page": 1, "page_size": 10}
        )
        data = response.json()

        if response.status_code == 200 and data.get("state"):
            files = data.get("data", {}).get("files", [])
            log_test("文件搜索", f"搜索文件-{source_name}", "PASS", f"找到 {len(files)} 个匹配文件")
        else:
            log_test("文件搜索", f"搜索文件-{source_name}", "FAIL", f"搜索失败: {data.get('message')}")

def test_error_scenarios(token):
    """测试异常场景"""
    headers = {"Authorization": f"Bearer {token}"}

    print("\n========== 测试异常场景 ==========")

    # 测试缺少必填字段
    test_cases = [
        ("缺少name字段", {"source_type": "local", "path": "D:\\Test"}),
        ("缺少source_type字段", {"name": "测试", "path": "D:\\Test"}),
        ("缺少path字段", {"name": "测试", "source_type": "local"}),
        ("115类型缺少cloud115_id", {"name": "测试", "source_type": "cloud115", "path": "0"}),
        ("无效的source_type", {"name": "测试", "source_type": "invalid", "path": "D:\\Test"}),
    ]

    print("\n--- 测试缺少必填字段 ---")
    for name, data in test_cases:
        response = requests.post(f"{BASE_URL}/media/sources", headers=headers, json=data)
        resp_data = response.json()

        if response.status_code != 200 or not resp_data.get("state"):
            log_test("异常测试", name, "PASS", f"正确拦截: {resp_data.get('message')}")
        else:
            log_test("异常测试", name, "FAIL", f"未正确拦截异常")

    # 测试访问不存在的资源
    print("\n--- 测试访问不存在的资源 ---")
    response = requests.get(f"{BASE_URL}/media/sources/99999", headers=headers)
    data = response.json()

    if response.status_code == 404 or "不存在" in data.get("message", ""):
        log_test("异常测试", "获取不存在的媒体源", "PASS", f"正确处理: {data.get('message')}")
    else:
        log_test("异常测试", "获取不存在的媒体源", "FAIL", f"未正确处理404")

    # 测试文件浏览异常
    response = requests.get(
        f"{BASE_URL}/media/files",
        headers=headers,
        params={"source_id": 99999, "path": "", "page": 1, "page_size": 10}
    )
    data = response.json()

    if "不存在" in data.get("message", "") or response.status_code == 404:
        log_test("异常测试", "浏览不存在的数据源", "PASS", f"正确处理: {data.get('message')}")
    else:
        log_test("异常测试", "浏览不存在的数据源", "FAIL", f"未正确处理")

def generate_report():
    """生成测试报告"""
    print("\n" + "="*80)
    print("媒体管理接口测试报告")
    print("="*80)
    print(f"测试时间: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print(f"测试环境: {BASE_URL}")
    print("\n" + "-"*80)

    total = len(test_results["pass"]) + len(test_results["fail"]) + len(test_results["warnings"])
    pass_rate = (len(test_results["pass"]) / total * 100) if total > 0 else 0

    print(f"\n测试统计:")
    print(f"  [PASS] 通过: {len(test_results['pass'])} 个")
    print(f"  [FAIL] 失败: {len(test_results['fail'])} 个")
    print(f"  [WARN] 警告: {len(test_results['warnings'])} 个")
    print(f"  [INFO] 通过率: {pass_rate:.2f}%")

    if test_results["fail"]:
        print("\n" + "-"*80)
        print("[FAIL] 失败用例详情:")
        print("-"*80)
        for i, result in enumerate(test_results["fail"], 1):
            print(f"\n{i}. [{result['category']}] {result['test_name']}")
            print(f"   原因: {result['message']}")

    print("\n" + "="*80)

    # 保存报告
    with open("debug/media_manager_test_report.json", "w", encoding="utf-8") as f:
        json.dump({
            "summary": {
                "total": total,
                "pass": len(test_results["pass"]),
                "fail": len(test_results["fail"]),
                "warnings": len(test_results["warnings"]),
                "pass_rate": f"{pass_rate:.2f}%"
            },
            "details": test_results
        }, f, ensure_ascii=False, indent=2)

def main():
    print("="*80)
    print("开始媒体管理接口全面测试")
    print("="*80)

    # 1. 获取Token
    print("\n[步骤1] 获取认证Token")
    token = get_token()
    if not token:
        print("[FAIL] 无法获取Token,测试终止")
        return
    print("[PASS] 成功获取Token")

    # 2. 测试媒体源CRUD
    print("\n[步骤2] 测试媒体源CRUD接口")
    sources, cloud115_sources = test_media_sources(token)
    test_create_media_source(token)

    # 3. 测试文件浏览
    print("\n[步骤3] 测试文件浏览接口")
    if cloud115_sources:
        test_file_browsing(token, cloud115_sources)
    else:
        log_test("文件浏览", "跳过测试", "WARN", "没有115数据源")

    # 4. 测试异常场景
    print("\n[步骤4] 测试异常场景")
    test_error_scenarios(token)

    # 5. 生成报告
    print("\n[步骤5] 生成测试报告")
    generate_report()

if __name__ == "__main__":
    main()
