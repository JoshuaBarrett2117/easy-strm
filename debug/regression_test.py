# -*- coding: utf-8 -*-
"""
回归测试脚本 - P0 Bug修复验证
测试目标: 验证115客户端动态初始化修复效果
"""
import requests
import json
import psycopg2
from datetime import datetime

# 测试配置
BASE_URL = "http://localhost:8082"
DB_CONFIG = {
    "host": "192.168.31.12",
    "port": 15432,
    "database": "easystrm",
    "user": "joshua",
    "password": "dwwzbuwioalvb"
}

# 测试结果收集
test_results = {
    "测试时间": datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
    "测试环境": "Windows本地",
    "修复内容": "115客户端动态初始化",
    "测试用例": [],
    "问题列表": [],
    "统计数据": {
        "总计": 0,
        "通过": 0,
        "失败": 0,
        "阻塞": 0
    }
}

def log_test(module, case_name, status, expected, actual, error_msg=""):
    """记录测试结果"""
    test_case = {
        "模块": module,
        "用例名称": case_name,
        "状态": status,
        "预期结果": expected,
        "实际结果": actual,
        "错误信息": error_msg
    }
    test_results["测试用例"].append(test_case)
    test_results["统计数据"]["总计"] += 1
    if status == "通过":
        test_results["统计数据"]["通过"] += 1
        print(f"✅ [{module}] {case_name} - 通过")
    elif status == "失败":
        test_results["统计数据"]["失败"] += 1
        print(f"❌ [{module}] {case_name} - 失败: {error_msg}")
        test_results["问题列表"].append({
            "模块": module,
            "用例": case_name,
            "问题描述": error_msg
        })
    else:
        test_results["统计数据"]["阻塞"] += 1
        print(f"⚠️  [{module}] {case_name} - 阻塞: {error_msg}")

def query_115_sources():
    """查询数据库中的115数据源"""
    print("\n" + "="*80)
    print("【步骤1】查询数据库中的115数据源")
    print("="*80)

    try:
        conn = psycopg2.connect(**DB_CONFIG)
        cursor = conn.cursor()

        # 查询115账号
        cursor.execute("""
            SELECT id, name, cookie, created_at
            FROM cloud115_accounts
            ORDER BY id
        """)
        accounts = cursor.fetchall()

        if accounts:
            print(f"\n找到 {len(accounts)} 个115账号:")
            for acc in accounts:
                print(f"  - ID: {acc[0]}, 名称: {acc[1]}, Cookie长度: {len(acc[2]) if acc[2] else 0}, 创建时间: {acc[3]}")
        else:
            print("\n⚠️  未找到任何115账号")

        # 查询115媒体源
        cursor.execute("""
            SELECT id, name, source_type, path, cloud115_id, priority, enabled
            FROM media_sources
            WHERE source_type = 'cloud115'
            ORDER BY id
        """)
        sources = cursor.fetchall()

        if sources:
            print(f"\n找到 {len(sources)} 个115媒体源:")
            for src in sources:
                print(f"  - ID: {src[0]}, 名称: {src[1]}, 路径: {src[3]}, 115账号ID: {src[4]}, 优先级: {src[5]}, 启用: {src[6]}")
        else:
            print("\n⚠️  未找到任何115媒体源")

        cursor.close()
        conn.close()

        return accounts, sources

    except Exception as e:
        print(f"❌ 数据库查询失败: {e}")
        return [], []

def login():
    """登录获取token"""
    print("\n" + "="*80)
    print("【步骤2】用户登录")
    print("="*80)

    try:
        response = requests.post(
            f"{BASE_URL}/api/auth/login",
            json={
                "username": "admin",
                "password": "admin123"
            }
        )

        if response.status_code == 200:
            data = response.json()
            token = data.get("data", {}).get("token")
            if token:
                print(f"✅ 登录成功, Token长度: {len(token)}")
                return token
            else:
                print(f"❌ 登录响应中未找到token: {data}")
                return None
        else:
            print(f"❌ 登录失败: {response.status_code} - {response.text}")
            return None

    except Exception as e:
        print(f"❌ 登录异常: {e}")
        return None

def test_media_source_apis(token):
    """测试媒体源CRUD接口"""
    print("\n" + "="*80)
    print("【步骤3】测试媒体源CRUD接口")
    print("="*80)

    headers = {"Authorization": f"Bearer {token}"}

    # 3.1 查询媒体源列表
    print("\n--- 3.1 查询媒体源列表 ---")
    try:
        response = requests.get(
            f"{BASE_URL}/api/media/sources",
            headers=headers
        )

        if response.status_code == 200:
            data = response.json()
            sources = data.get("data", {}).get("data", [])
            log_test("媒体源管理", "查询媒体源列表", "通过",
                    "返回媒体源列表", f"返回{len(sources)}个媒体源")
            return sources
        else:
            log_test("媒体源管理", "查询媒体源列表", "失败",
                    "HTTP 200", f"HTTP {response.status_code}", response.text)
            return []

    except Exception as e:
        log_test("媒体源管理", "查询媒体源列表", "阻塞",
                "返回媒体源列表", "请求异常", str(e))
        return []

def test_file_browse(token, source_id, path=""):
    """测试文件浏览接口 - 核心修复验证"""
    print("\n" + "="*80)
    print(f"【步骤4】测试文件浏览接口 (source_id={source_id}, path={path or '根目录'})")
    print("="*80)

    headers = {"Authorization": f"Bearer {token}"}

    try:
        params = {"source_id": source_id}
        if path:
            params["path"] = path

        response = requests.get(
            f"{BASE_URL}/api/media/files",
            headers=headers,
            params=params
        )

        if response.status_code == 200:
            data = response.json()
            result = data.get("data", {})
            files = result.get("files", [])
            total = result.get("total", 0)

            log_test("文件浏览", f"浏览115目录(path={path or '根目录'})", "通过",
                    f"返回文件列表", f"返回{total}个文件/目录")

            # 验证返回的数据结构
            if files:
                sample = files[0]
                required_fields = ["id", "name", "type", "is_directory"]
                missing_fields = [f for f in required_fields if f not in sample]

                if not missing_fields:
                    log_test("文件浏览", "验证文件数据结构", "通过",
                            "包含所有必需字段", "数据结构完整")
                else:
                    log_test("文件浏览", "验证文件数据结构", "失败",
                            "包含所有必需字段", f"缺少字段: {missing_fields}")
            else:
                log_test("文件浏览", "验证文件数据结构", "失败",
                        "包含所有必需字段", "返回空列表")

            return True

        else:
            error_msg = response.text
            log_test("文件浏览", f"浏览115目录(path={path or '根目录'})", "失败",
                    "HTTP 200", f"HTTP {response.status_code}", error_msg)
            return False

    except Exception as e:
        log_test("文件浏览", f"浏览115目录(path={path or '根目录'})", "阻塞",
                "返回文件列表", "请求异常", str(e))
        return False

def test_file_search(token, source_id, keyword):
    """测试文件搜索接口 - 核心修复验证"""
    print("\n" + "="*80)
    print(f"【步骤5】测试文件搜索接口 (keyword={keyword})")
    print("="*80)

    headers = {"Authorization": f"Bearer {token}"}

    try:
        response = requests.get(
            f"{BASE_URL}/api/media/files/search",
            headers=headers,
            params={
                "source_id": source_id,
                "keyword": keyword
            }
        )

        if response.status_code == 200:
            data = response.json()
            result = data.get("data", {})
            files = result.get("files", [])
            total = result.get("total", 0)

            log_test("文件搜索", f"搜索关键词'{keyword}'", "通过",
                    f"返回匹配文件", f"找到{total}个匹配文件")
            return True

        else:
            error_msg = response.text
            log_test("文件搜索", f"搜索关键词'{keyword}'", "失败",
                    "HTTP 200", f"HTTP {response.status_code}", error_msg)
            return False

    except Exception as e:
        log_test("文件搜索", f"搜索关键词'{keyword}'", "阻塞",
                "返回匹配文件", "请求异常", str(e))
        return False

def test_create_115_source(token, cloud115_id):
    """测试创建新的115数据源"""
    print("\n" + "="*80)
    print("【步骤6】测试创建新的115数据源")
    print("="*80)

    headers = {"Authorization": f"Bearer {token}"}

    try:
        # 创建新的115媒体源
        response = requests.post(
            f"{BASE_URL}/api/media/sources",
            headers=headers,
            json={
                "name": f"回归测试-115数据源-{datetime.now().strftime('%H%M%S')}",
                "source_type": "cloud115",
                "path": "0",  # 根目录
                "cloud115_id": cloud115_id,
                "priority": 99,
                "enabled": True
            }
        )

        if response.status_code == 200:
            data = response.json()
            source = data.get("data", {}).get("data", {})
            source_id = source.get("id")

            log_test("媒体源管理", "创建115数据源", "通过",
                    "创建成功并返回ID", f"创建成功, ID={source_id}")

            # 立即测试新创建的数据源
            if source_id:
                test_file_browse(token, source_id, "")

            # 清理: 删除测试数据源
            delete_response = requests.delete(
                f"{BASE_URL}/api/media/sources/{source_id}",
                headers=headers
            )
            if delete_response.status_code == 200:
                print(f"  🧹 已清理测试数据源 ID={source_id}")

            return True

        else:
            error_msg = response.text
            log_test("媒体源管理", "创建115数据源", "失败",
                    "HTTP 200", f"HTTP {response.status_code}", error_msg)
            return False

    except Exception as e:
        log_test("媒体源管理", "创建115数据源", "阻塞",
                "创建成功", "请求异常", str(e))
        return False

def test_edge_cases(token, source_id):
    """测试边界情况"""
    print("\n" + "="*80)
    print("【步骤7】测试边界情况")
    print("="*80)

    headers = {"Authorization": f"Bearer {token}"}

    # 7.1 无效的source_id
    print("\n--- 7.1 测试无效的source_id ---")
    try:
        response = requests.get(
            f"{BASE_URL}/api/media/files",
            headers=headers,
            params={"source_id": 99999}
        )

        if response.status_code == 404:
            log_test("边界测试", "无效source_id返回404", "通过",
                    "HTTP 404", f"HTTP {response.status_code}")
        else:
            log_test("边界测试", "无效source_id返回404", "失败",
                    "HTTP 404", f"HTTP {response.status_code}", response.text)

    except Exception as e:
        log_test("边界测试", "无效source_id返回404", "阻塞",
                "HTTP 404", "请求异常", str(e))

    # 7.2 空关键词搜索
    print("\n--- 7.2 测试空关键词搜索 ---")
    try:
        response = requests.get(
            f"{BASE_URL}/api/media/files/search",
            headers=headers,
            params={
                "source_id": source_id,
                "keyword": ""
            }
        )

        if response.status_code == 400:
            log_test("边界测试", "空关键词返回400", "通过",
                    "HTTP 400", f"HTTP {response.status_code}")
        else:
            log_test("边界测试", "空关键词返回400", "失败",
                    "HTTP 400", f"HTTP {response.status_code}", response.text)

    except Exception as e:
        log_test("边界测试", "空关键词返回400", "阻塞",
                "HTTP 400", "请求异常", str(e))

    # 7.3 分页参数测试
    print("\n--- 7.3 测试分页参数 ---")
    try:
        response = requests.get(
            f"{BASE_URL}/api/media/files",
            headers=headers,
            params={
                "source_id": source_id,
                "page": 1,
                "page_size": 10
            }
        )

        if response.status_code == 200:
            data = response.json()
            result = data.get("data", {})
            page = result.get("page")
            page_size = result.get("page_size")

            if page == 1 and page_size == 10:
                log_test("边界测试", "分页参数正确处理", "通过",
                        "page=1, page_size=10", f"page={page}, page_size={page_size}")
            else:
                log_test("边界测试", "分页参数正确处理", "失败",
                        "page=1, page_size=10", f"page={page}, page_size={page_size}")
        else:
            log_test("边界测试", "分页参数正确处理", "失败",
                    "HTTP 200", f"HTTP {response.status_code}", response.text)

    except Exception as e:
        log_test("边界测试", "分页参数正确处理", "阻塞",
                "返回分页数据", "请求异常", str(e))

def generate_report():
    """生成测试报告"""
    print("\n" + "="*80)
    print("【测试报告】")
    print("="*80)

    stats = test_results["统计数据"]
    print(f"\n📊 统计数据:")
    print(f"  总计: {stats['总计']}")
    print(f"  ✅ 通过: {stats['通过']}")
    print(f"  ❌ 失败: {stats['失败']}")
    print(f"  ⚠️  阻塞: {stats['阻塞']}")

    pass_rate = (stats['通过'] / stats['总计'] * 100) if stats['总计'] > 0 else 0
    print(f"\n📈 通过率: {pass_rate:.1f}%")

    if test_results["问题列表"]:
        print(f"\n❌ 发现问题 ({len(test_results['问题列表'])}个):")
        for i, issue in enumerate(test_results["问题列表"], 1):
            print(f"  {i}. [{issue['模块']}] {issue['用例']}: {issue['问题描述']}")

    # 保存报告到文件
    report_file = "debug/regression_test_report.json"
    with open(report_file, "w", encoding="utf-8") as f:
        json.dump(test_results, f, ensure_ascii=False, indent=2)

    print(f"\n📄 详细报告已保存至: {report_file}")

    # 最终结论
    print("\n" + "="*80)
    print("【最终结论】")
    print("="*80)

    if stats['失败'] == 0 and stats['阻塞'] == 0:
        print("✅ 所有测试通过！P0 Bug修复验证成功！")
        print("✅ 115客户端动态初始化机制工作正常")
        print("✅ 文件浏览和搜索接口功能正常")
        print("✅ 可以安全上线")
    elif stats['失败'] > 0:
        print(f"❌ 发现 {stats['失败']} 个失败用例，需要修复后再上线")
    else:
        print(f"⚠️  发现 {stats['阻塞']} 个阻塞用例，需要排查环境问题")

def main():
    """主测试流程"""
    print("\n" + "="*80)
    print("【回归测试】P0 Bug修复验证 - 115客户端动态初始化")
    print("="*80)

    # 步骤1: 查询数据库中的115数据源
    accounts, sources = query_115_sources()

    if not accounts:
        print("\n❌ 未找到115账号，无法继续测试")
        return

    if not sources:
        print("\n⚠️  未找到115媒体源，将创建测试数据源")

    # 步骤2: 登录获取token
    token = login()
    if not token:
        print("\n❌ 登录失败，无法继续测试")
        return

    # 步骤3: 测试媒体源CRUD接口
    all_sources = test_media_source_apis(token)

    # 获取115媒体源ID
    cloud115_sources = [s for s in all_sources if s.get("source_type") == "cloud115"]

    if cloud115_sources:
        # 步骤4-5: 测试核心功能（文件浏览和搜索）
        for source in cloud115_sources:
            source_id = source.get("id")
            cloud115_id = source.get("cloud115_id")

            # 测试文件浏览
            test_file_browse(token, source_id, "")  # 根目录
            test_file_browse(token, source_id, "0")  # 明确指定根目录

            # 测试文件搜索
            test_file_search(token, source_id, "test")

            # 步骤6: 测试创建新的115数据源
            test_create_115_source(token, cloud115_id)

            # 步骤7: 测试边界情况
            test_edge_cases(token, source_id)

            # 只测试第一个115数据源
            break
    else:
        print("\n⚠️  未找到115媒体源，跳过核心功能测试")

    # 生成测试报告
    generate_report()

if __name__ == "__main__":
    main()
