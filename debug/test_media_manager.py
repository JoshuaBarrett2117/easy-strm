# -*- coding: utf-8 -*-
"""
媒体管理接口全面测试脚本
测试范围:
1. 媒体源CRUD接口测试
2. 115数据源连接测试
3. 文件浏览接口测试
4. 异常场景测试
"""

import requests
import json
import psycopg2
import hashlib
from datetime import datetime

# 测试配置
BASE_URL = "http://localhost:8082"
DB_CONFIG = {
    "host": "192.168.31.12",
    "port": 15432,
    "database": "easy_strm",
    "user": "joshua",
    "password": "dwwzbuwioalvb"
}

# 测试结果记录
test_results = {
    "pass": [],
    "fail": [],
    "warnings": []
}

def log_test(category, test_name, status, message="", details=None):
    """记录测试结果"""
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

def get_auth_token():
    """获取认证Token"""
    try:
        # 密码需要发送MD5哈希值
        password_md5 = hashlib.md5("admin".encode()).hexdigest()
        response = requests.post(
            f"{BASE_URL}/login",
            json={"name": "admin", "password": password_md5}
        )

        if response.status_code == 200:
            data = response.json()
            # 登录接口返回格式: {"message":"Login successful","token":"...","user_id":1,"name":"admin"}
            token = data.get("token")
            if token:
                log_test("认证", "获取Token", "PASS", "成功获取认证Token")
                return token

        log_test("认证", "获取Token", "FAIL", f"登录失败: {response.text}")
        return None
    except Exception as e:
        log_test("认证", "获取Token", "FAIL", f"异常: {str(e)}")
        return None

def query_115_sources_from_db():
    """从数据库查询115数据源"""
    try:
        conn = psycopg2.connect(**DB_CONFIG)
        cursor = conn.cursor()

        # 查询所有115数据源
        cursor.execute("""
            SELECT ms.id, ms.name, ms.source_type, ms.path, ms.cloud115_id, ms.enabled,
                   c.name as cloud115_name, c.cookie
            FROM t_media_source ms
            LEFT JOIN t_cloud_115 c ON ms.cloud115_id = c.id
            WHERE ms.source_type = 'cloud115'
            ORDER BY ms.id
        """)

        sources = cursor.fetchall()

        # 查询所有115账号
        cursor.execute("""
            SELECT id, name, cookie
            FROM t_cloud_115
            ORDER BY id
        """)
        accounts = cursor.fetchall()

        cursor.close()
        conn.close()

        log_test("数据库", "查询115数据源", "PASS",
                f"找到 {len(sources)} 个115数据源, {len(accounts)} 个115账号")

        return sources, accounts
    except Exception as e:
        log_test("数据库", "查询115数据源", "FAIL", f"数据库查询失败: {str(e)}")
        return [], []

def test_media_source_apis(token):
    """测试媒体源CRUD接口"""
    headers = {"Authorization": f"Bearer {token}"}

    # 1. 获取媒体源列表
    print("\n========== 测试获取媒体源列表 ==========")
    try:
        response = requests.get(f"{BASE_URL}/media/sources", headers=headers)
        data = response.json()

        if response.status_code == 200 and data.get("state"):
            sources = data.get("data", {}).get("data", [])
            log_test("媒体源API", "获取媒体源列表", "PASS",
                    f"成功获取 {len(sources)} 个媒体源",
                    {"total": len(sources), "sources": sources[:3]})
        else:
            log_test("媒体源API", "获取媒体源列表", "FAIL",
                    f"接口返回失败: {data.get('message', 'Unknown error')}")
    except Exception as e:
        log_test("媒体源API", "获取媒体源列表", "FAIL", f"请求异常: {str(e)}")

    # 2. 创建本地媒体源
    print("\n========== 测试创建本地媒体源 ==========")
    create_data = {
        "name": f"测试本地媒体源_{datetime.now().strftime('%H%M%S')}",
        "source_type": "local",
        "path": "D:\\Media\\Test",
        "priority": 99,
        "enabled": True
    }

    try:
        response = requests.post(
            f"{BASE_URL}/api/media/sources",
            headers=headers,
            json=create_data
        )
        data = response.json()

        if response.status_code == 200 and data.get("state"):
            source_id = data.get("data", {}).get("data", {}).get("id")
            log_test("媒体源API", "创建本地媒体源", "PASS",
                    f"成功创建, ID: {source_id}",
                    {"source_id": source_id, "name": create_data["name"]})

            # 3. 更新媒体源
            print("\n========== 测试更新媒体源 ==========")
            update_data = {
                "name": f"已更新_{create_data['name']}",
                "priority": 88
            }
            response = requests.put(
                f"{BASE_URL}/api/media/sources/{source_id}",
                headers=headers,
                json=update_data
            )
            data = response.json()

            if response.status_code == 200 and data.get("state"):
                log_test("媒体源API", "更新媒体源", "PASS",
                        f"成功更新媒体源 ID: {source_id}")
            else:
                log_test("媒体源API", "更新媒体源", "FAIL",
                        f"更新失败: {data.get('message', 'Unknown error')}")

            # 4. 删除媒体源
            print("\n========== 测试删除媒体源 ==========")
            response = requests.delete(
                f"{BASE_URL}/api/media/sources/{source_id}",
                headers=headers
            )
            data = response.json()

            if response.status_code == 200 and data.get("state"):
                log_test("媒体源API", "删除媒体源", "PASS",
                        f"成功删除媒体源 ID: {source_id}")
            else:
                log_test("媒体源API", "删除媒体源", "FAIL",
                        f"删除失败: {data.get('message', 'Unknown error')}")
        else:
            log_test("媒体源API", "创建本地媒体源", "FAIL",
                    f"创建失败: {data.get('message', 'Unknown error')}",
                    {"response": data})
    except Exception as e:
        log_test("媒体源API", "创建本地媒体源", "FAIL", f"请求异常: {str(e)}")

    # 5. 测试创建115媒体源(需要有效的cloud115_id)
    print("\n========== 测试创建115媒体源 ==========")
    # 先获取115账号列表
    try:
        response = requests.get(f"{BASE_URL}/api/cloud115/list", headers=headers)
        data = response.json()

        if response.status_code == 200 and data.get("state"):
            accounts = data.get("data", {}).get("data", [])
            if accounts:
                cloud115_id = accounts[0].get("id")
                create_data_115 = {
                    "name": f"测试115媒体源_{datetime.now().strftime('%H%M%S')}",
                    "source_type": "cloud115",
                    "path": "0",  # 根目录
                    "cloud115_id": cloud115_id,
                    "priority": 98,
                    "enabled": True
                }

                response = requests.post(
                    f"{BASE_URL}/api/media/sources",
                    headers=headers,
                    json=create_data_115
                )
                data = response.json()

                if response.status_code == 200 and data.get("state"):
                    source_id_115 = data.get("data", {}).get("data", {}).get("id")
                    log_test("媒体源API", "创建115媒体源", "PASS",
                            f"成功创建, ID: {source_id_115}",
                            {"source_id": source_id_115, "cloud115_id": cloud115_id})

                    # 清理: 删除测试创建的115媒体源
                    requests.delete(
                        f"{BASE_URL}/api/media/sources/{source_id_115}",
                        headers=headers
                    )
                else:
                    log_test("媒体源API", "创建115媒体源", "FAIL",
                            f"创建失败: {data.get('message', 'Unknown error')}")
            else:
                log_test("媒体源API", "创建115媒体源", "WARN", "没有可用的115账号")
        else:
            log_test("媒体源API", "获取115账号列表", "WARN",
                    f"获取失败: {data.get('message', 'Unknown error')}")
    except Exception as e:
        log_test("媒体源API", "创建115媒体源", "FAIL", f"请求异常: {str(e)}")

def test_file_browsing_apis(token, sources):
    """测试文件浏览接口"""
    headers = {"Authorization": f"Bearer {token}"}

    print("\n========== 测试文件浏览接口 ==========")

    # 测试每个115数据源
    for source in sources:
        source_id = source[0]
        source_name = source[1]
        cloud115_id = source[4]

        print(f"\n--- 测试数据源: {source_name} (ID: {source_id}) ---")

        # 1. 浏览根目录
        try:
            response = requests.get(
                f"{BASE_URL}/api/media/files",
                headers=headers,
                params={
                    "source_id": source_id,
                    "path": "",  # 使用数据源配置的根目录
                    "page": 1,
                    "page_size": 20
                }
            )
            data = response.json()

            if response.status_code == 200 and data.get("state"):
                result = data.get("data", {})
                files = result.get("files", [])
                total = result.get("total", 0)

                log_test("文件浏览", f"浏览根目录-{source_name}", "PASS",
                        f"成功获取 {total} 个文件/目录",
                        {"total": total, "sample_files": files[:3] if files else []})
            else:
                log_test("文件浏览", f"浏览根目录-{source_name}", "FAIL",
                        f"获取失败: {data.get('message', 'Unknown error')}",
                        {"response": data})
        except Exception as e:
            log_test("文件浏览", f"浏览根目录-{source_name}", "FAIL",
                    f"请求异常: {str(e)}")

        # 2. 测试搜索功能
        try:
            response = requests.get(
                f"{BASE_URL}/api/media/files/search",
                headers=headers,
                params={
                    "source_id": source_id,
                    "keyword": "mp4",
                    "page": 1,
                    "page_size": 10
                }
            )
            data = response.json()

            if response.status_code == 200 and data.get("state"):
                result = data.get("data", {})
                files = result.get("files", [])

                log_test("文件搜索", f"搜索文件-{source_name}", "PASS",
                        f"找到 {len(files)} 个匹配文件")
            else:
                log_test("文件搜索", f"搜索文件-{source_name}", "FAIL",
                        f"搜索失败: {data.get('message', 'Unknown error')}")
        except Exception as e:
            log_test("文件搜索", f"搜索文件-{source_name}", "FAIL",
                    f"请求异常: {str(e)}")

def test_error_scenarios(token):
    """测试异常场景"""
    headers = {"Authorization": f"Bearer {token}"}

    print("\n========== 测试异常场景 ==========")

    # 1. 创建媒体源时缺少必填字段
    print("\n--- 测试缺少必填字段 ---")
    test_cases = [
        {
            "name": "缺少name字段",
            "data": {"source_type": "local", "path": "D:\\Test"},
            "expected": "参数校验失败"
        },
        {
            "name": "缺少source_type字段",
            "data": {"name": "测试", "path": "D:\\Test"},
            "expected": "参数校验失败"
        },
        {
            "name": "缺少path字段",
            "data": {"name": "测试", "source_type": "local"},
            "expected": "参数校验失败"
        },
        {
            "name": "115类型缺少cloud115_id",
            "data": {"name": "测试", "source_type": "cloud115", "path": "0"},
            "expected": "115账号ID不能为空"
        },
        {
            "name": "无效的source_type",
            "data": {"name": "测试", "source_type": "invalid", "path": "D:\\Test"},
            "expected": "无效的媒体源类型"
        }
    ]

    for case in test_cases:
        try:
            response = requests.post(
                f"{BASE_URL}/api/media/sources",
                headers=headers,
                json=case["data"]
            )
            data = response.json()

            # 应该返回错误
            if response.status_code != 200 or not data.get("state"):
                log_test("异常测试", case["name"], "PASS",
                        f"正确拦截: {data.get('message', 'Unknown error')}")
            else:
                log_test("异常测试", case["name"], "FAIL",
                        f"未正确拦截异常, 返回成功: {data}")
        except Exception as e:
            log_test("异常测试", case["name"], "FAIL", f"请求异常: {str(e)}")

    # 2. 访问不存在的资源
    print("\n--- 测试访问不存在的资源 ---")
    test_cases_404 = [
        {
            "name": "获取不存在的媒体源",
            "method": "GET",
            "url": f"{BASE_URL}/api/media/sources/99999"
        },
        {
            "name": "更新不存在的媒体源",
            "method": "PUT",
            "url": f"{BASE_URL}/api/media/sources/99999",
            "data": {"name": "测试"}
        },
        {
            "name": "删除不存在的媒体源",
            "method": "DELETE",
            "url": f"{BASE_URL}/api/media/sources/99999"
        }
    ]

    for case in test_cases_404:
        try:
            if case["method"] == "GET":
                response = requests.get(case["url"], headers=headers)
            elif case["method"] == "PUT":
                response = requests.put(case["url"], headers=headers, json=case.get("data", {}))
            else:
                response = requests.delete(case["url"], headers=headers)

            data = response.json()

            if response.status_code == 404 or "不存在" in data.get("message", ""):
                log_test("异常测试", case["name"], "PASS",
                        f"正确处理: {data.get('message', 'Unknown error')}")
            else:
                log_test("异常测试", case["name"], "FAIL",
                        f"未正确处理404: {data}")
        except Exception as e:
            log_test("异常测试", case["name"], "FAIL", f"请求异常: {str(e)}")

    # 3. 文件浏览异常场景
    print("\n--- 测试文件浏览异常场景 ---")
    try:
        # 无效的source_id
        response = requests.get(
            f"{BASE_URL}/api/media/files",
            headers=headers,
            params={"source_id": 99999, "path": "", "page": 1, "page_size": 10}
        )
        data = response.json()

        if "不存在" in data.get("message", "") or response.status_code == 404:
            log_test("异常测试", "浏览不存在的数据源", "PASS",
                    f"正确处理: {data.get('message', 'Unknown error')}")
        else:
            log_test("异常测试", "浏览不存在的数据源", "FAIL",
                    f"未正确处理: {data}")
    except Exception as e:
        log_test("异常测试", "浏览不存在的数据源", "FAIL", f"请求异常: {str(e)}")

def generate_report():
    """生成测试报告"""
    print("\n" + "="*80)
    print("媒体管理接口测试报告")
    print("="*80)
    print(f"测试时间: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print(f"测试环境: {BASE_URL}")
    print(f"数据库: {DB_CONFIG['host']}:{DB_CONFIG['port']}/{DB_CONFIG['database']}")
    print("\n" + "-"*80)

    # 统计
    total = len(test_results["pass"]) + len(test_results["fail"]) + len(test_results["warnings"])
    pass_rate = (len(test_results["pass"]) / total * 100) if total > 0 else 0

    print(f"\n测试统计:")
    print(f"  [PASS] 通过: {len(test_results['pass'])} 个")
    print(f"  [FAIL] 失败: {len(test_results['fail'])} 个")
    print(f"  [WARN] 警告: {len(test_results['warnings'])} 个")
    print(f"  [INFO] 通过率: {pass_rate:.2f}%")

    # 失败详情
    if test_results["fail"]:
        print("\n" + "-"*80)
        print("[FAIL] 失败用例详情:")
        print("-"*80)
        for i, result in enumerate(test_results["fail"], 1):
            print(f"\n{i}. [{result['category']}] {result['test_name']}")
            print(f"   时间: {result['timestamp']}")
            print(f"   原因: {result['message']}")
            if result.get("details"):
                print(f"   详情: {json.dumps(result['details'], ensure_ascii=False, indent=6)}")

    # 警告详情
    if test_results["warnings"]:
        print("\n" + "-"*80)
        print("[WARN] 警告用例详情:")
        print("-"*80)
        for i, result in enumerate(test_results["warnings"], 1):
            print(f"\n{i}. [{result['category']}] {result['test_name']}")
            print(f"   时间: {result['timestamp']}")
            print(f"   说明: {result['message']}")

    print("\n" + "="*80)

    # 保存报告到文件
    report_file = "debug/media_manager_test_report.json"
    with open(report_file, "w", encoding="utf-8") as f:
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

    print(f"详细报告已保存至: {report_file}")

def main():
    """主测试流程"""
    print("="*80)
    print("开始媒体管理接口全面测试")
    print("="*80)

    # 1. 获取认证Token
    print("\n[步骤1] 获取认证Token")
    token = get_auth_token()
    if not token:
        print("[FAIL] 无法获取Token,测试终止")
        return

    # 2. 查询数据库中的115数据源
    print("\n[步骤2] 查询数据库中的115数据源")
    sources, accounts = query_115_sources_from_db()

    # 3. 测试媒体源CRUD接口
    print("\n[步骤3] 测试媒体源CRUD接口")
    test_media_source_apis(token)

    # 4. 测试文件浏览接口
    print("\n[步骤4] 测试文件浏览接口")
    if sources:
        test_file_browsing_apis(token, sources)
    else:
        log_test("文件浏览", "跳过测试", "WARN", "数据库中没有115数据源")

    # 5. 测试异常场景
    print("\n[步骤5] 测试异常场景")
    test_error_scenarios(token)

    # 6. 生成测试报告
    print("\n[步骤6] 生成测试报告")
    generate_report()

if __name__ == "__main__":
    main()
