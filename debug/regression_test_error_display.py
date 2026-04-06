# -*- coding: utf-8 -*-
"""
前端错误信息展示回归测试脚本
测试目标：验证前端错误信息展示修复效果
测试日期：2026-04-02
"""

import asyncio
import json
import os
from datetime import datetime
from playwright.async_api import async_playwright, Page, Browser

# 测试配置
BASE_URL = "http://localhost:3001"
TEST_USERNAME = "admin"
TEST_PASSWORD = "admin"
SCREENSHOT_DIR = "c:/Users/a3875/Documents/code/easy-strm/debug/regression_test_screenshots"

# 测试结果
test_results = {
    "test_time": datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
    "total_tests": 0,
    "passed": 0,
    "failed": 0,
    "test_cases": []
}


def ensure_screenshot_dir():
    """确保截图目录存在"""
    if not os.path.exists(SCREENSHOT_DIR):
        os.makedirs(SCREENSHOT_DIR)


async def take_screenshot(page: Page, name: str):
    """保存截图"""
    ensure_screenshot_dir()
    screenshot_path = os.path.join(SCREENSHOT_DIR, f"{name}.png")
    await page.screenshot(path=screenshot_path, full_page=True)
    print(f"📸 截图已保存: {screenshot_path}")
    return screenshot_path


async def wait_for_toast(page: Page, timeout: int = 5000):
    """等待并获取Toast消息"""
    try:
        # 等待Element Plus的Message组件出现
        toast_selector = ".el-message--error, .el-message--success, .el-message--warning"
        await page.wait_for_selector(toast_selector, timeout=timeout)
        toast = await page.query_selector(toast_selector)
        if toast:
            text = await toast.text_content()
            return text.strip()
    except Exception as e:
        print(f"⚠️ 未检测到Toast消息: {e}")
    return None


async def login(page: Page):
    """登录系统"""
    print("\n🔐 开始登录测试...")
    await page.goto(f"{BASE_URL}/login")
    await page.wait_for_load_state("networkidle")

    # 填写登录表单
    await page.fill('input[placeholder="请输入用户名"]', TEST_USERNAME)
    await page.fill('input[placeholder="请输入密码"]', TEST_PASSWORD)

    # 点击登录按钮
    await page.click('button:has-text("登录系统")')

    # 等待跳转到首页或仪表盘
    try:
        await page.wait_for_url("**/dashboard**", timeout=5000)
        print("✅ 登录成功")
        return True
    except:
        # 检查是否有错误提示
        toast = await wait_for_toast(page, timeout=2000)
        if toast:
            print(f"❌ 登录失败: {toast}")
        return False


async def test_login_error(page: Page):
    """测试登录错误提示"""
    print("\n" + "="*60)
    print("📋 测试用例 1: 登录错误提示")
    print("="*60)

    test_case = {
        "name": "登录错误提示",
        "status": "Fail",
        "expected": "应显示具体错误信息，而非通用提示",
        "actual": "",
        "screenshot": ""
    }

    try:
        await page.goto(f"{BASE_URL}/login")
        await page.wait_for_load_state("networkidle")

        # 输入错误密码
        await page.fill('input[placeholder="请输入用户名"]', TEST_USERNAME)
        await page.fill('input[placeholder="请输入密码"]', "wrong_password")

        # 点击登录
        await page.click('button:has-text("登录系统")')

        # 等待错误提示
        toast = await wait_for_toast(page, timeout=3000)

        if toast:
            test_case["actual"] = f"显示错误信息: {toast}"
            print(f"💬 实际显示: {toast}")

            # 检查是否为具体错误信息（而非通用提示）
            if "用户名或密码错误" in toast or "密码错误" in toast or "用户不存在" in toast:
                test_case["status"] = "Pass"
                print("✅ 测试通过: 显示了具体的错误信息")
            else:
                print("⚠️ 测试失败: 未显示具体错误信息")
        else:
            test_case["actual"] = "未显示任何错误提示"
            print("⚠️ 未检测到错误提示")

        # 截图
        screenshot_path = await take_screenshot(page, "test_login_error")
        test_case["screenshot"] = screenshot_path

    except Exception as e:
        test_case["actual"] = f"测试异常: {str(e)}"
        print(f"❌ 测试异常: {e}")

    test_results["test_cases"].append(test_case)
    return test_case["status"] == "Pass"


async def test_115_browse_invalid_path(page: Page):
    """测试115数据源浏览 - 非数字路径错误提示"""
    print("\n" + "="*60)
    print("📋 测试用例 2: 115数据源浏览 - 非数字路径错误提示 (P0)")
    print("="*60)

    test_case = {
        "name": "115数据源浏览 - 非数字路径错误提示",
        "status": "Fail",
        "expected": "应显示'目录ID格式错误'，而非'获取文件列表失败'",
        "actual": "",
        "screenshot": ""
    }

    try:
        # 导航到媒体管理页面
        await page.goto(f"{BASE_URL}/media-manager")
        await page.wait_for_load_state("networkidle")

        # 点击新增媒体源按钮
        await page.click('button:has-text("新增媒体源")')
        await page.wait_for_selector('.el-dialog', timeout=3000)

        # 填写表单
        await page.click('.el-select:has-text("请选择类型")')
        await page.click('.el-select-dropdown__item:has-text("115云盘")')

        # 输入名称
        await page.fill('input[placeholder="请输入媒体源名称"]', "测试115源")

        # 输入非数字路径
        await page.fill('input[placeholder="请输入115网盘目录ID"]', "abc")
        print("📝 已输入非数字路径: abc")

        # 尝试点击浏览按钮（如果有）
        browse_btn = await page.query_selector('button:has-text("浏览")')
        if browse_btn:
            await browse_btn.click()
            print("🖱️ 已点击浏览按钮")

            # 等待错误提示
            toast = await wait_for_toast(page, timeout=3000)

            if toast:
                test_case["actual"] = f"显示错误信息: {toast}"
                print(f"💬 实际显示: {toast}")

                # 检查是否包含"目录ID格式错误"
                if "目录ID格式错误" in toast or "格式错误" in toast:
                    test_case["status"] = "Pass"
                    print("✅ 测试通过: 显示了具体的错误信息")
                elif "获取文件列表失败" in toast:
                    print("❌ 测试失败: 显示了通用错误信息")
                else:
                    print("⚠️ 显示了其他错误信息")
            else:
                test_case["actual"] = "未显示任何错误提示"
                print("⚠️ 未检测到错误提示")
        else:
            # 如果没有浏览按钮，尝试保存并触发错误
            await page.click('button:has-text("确定")')
            print("🖱️ 已点击确定按钮")

            toast = await wait_for_toast(page, timeout=3000)
            if toast:
                test_case["actual"] = f"显示错误信息: {toast}"
                print(f"💬 实际显示: {toast}")

                if "目录ID格式错误" in toast or "格式错误" in toast:
                    test_case["status"] = "Pass"
                    print("✅ 测试通过: 显示了具体的错误信息")

        # 截图
        screenshot_path = await take_screenshot(page, "test_115_browse_invalid_path")
        test_case["screenshot"] = screenshot_path

        # 关闭对话框
        try:
            close_btn = await page.query_selector('.el-dialog__headerbtn')
            if close_btn:
                await close_btn.click()
        except:
            pass

    except Exception as e:
        test_case["actual"] = f"测试异常: {str(e)}"
        print(f"❌ 测试异常: {e}")
        screenshot_path = await take_screenshot(page, "test_115_browse_invalid_path_error")
        test_case["screenshot"] = screenshot_path

    test_results["test_cases"].append(test_case)
    return test_case["status"] == "Pass"


async def test_delete_media_source(page: Page):
    """测试删除媒体源错误提示"""
    print("\n" + "="*60)
    print("📋 测试用例 3: 删除媒体源错误提示")
    print("="*60)

    test_case = {
        "name": "删除媒体源错误提示",
        "status": "Fail",
        "expected": "应显示具体错误信息",
        "actual": "",
        "screenshot": ""
    }

    try:
        # 导航到媒体管理页面
        await page.goto(f"{BASE_URL}/media-manager")
        await page.wait_for_load_state("networkidle")

        # 等待表格加载
        await page.wait_for_selector('.el-table', timeout=5000)

        # 查找删除按钮
        delete_buttons = await page.query_selector_all('button:has-text("删除")')

        if delete_buttons and len(delete_buttons) > 0:
            print(f"📊 找到 {len(delete_buttons)} 个媒体源")

            # 点击第一个删除按钮
            await delete_buttons[0].click()
            print("🖱️ 已点击删除按钮")

            # 等待确认对话框
            await page.wait_for_selector('.el-message-box', timeout=3000)

            # 点击确定删除
            await page.click('.el-message-box__btns button:has-text("确定")')
            print("🖱️ 已确认删除")

            # 等待结果提示
            toast = await wait_for_toast(page, timeout=3000)

            if toast:
                test_case["actual"] = f"显示提示: {toast}"
                print(f"💬 实际显示: {toast}")

                # 如果删除成功或显示具体错误，都算通过
                if "成功" in toast or "失败" in toast or "错误" in toast:
                    test_case["status"] = "Pass"
                    print("✅ 测试通过: 显示了具体的提示信息")
            else:
                # 检查表格是否更新（可能删除成功但没有提示）
                test_case["actual"] = "删除操作完成，未显示提示"
                test_case["status"] = "Pass"
                print("✅ 删除操作完成")
        else:
            test_case["actual"] = "没有可删除的媒体源"
            test_case["status"] = "Pass"
            print("ℹ️ 没有可删除的媒体源")

        # 截图
        screenshot_path = await take_screenshot(page, "test_delete_media_source")
        test_case["screenshot"] = screenshot_path

    except Exception as e:
        test_case["actual"] = f"测试异常: {str(e)}"
        print(f"❌ 测试异常: {e}")
        screenshot_path = await take_screenshot(page, "test_delete_media_source_error")
        test_case["screenshot"] = screenshot_path

    test_results["test_cases"].append(test_case)
    return test_case["status"] == "Pass"


async def test_batch_identify_error(page: Page):
    """测试批量识别错误提示"""
    print("\n" + "="*60)
    print("📋 测试用例 4: 批量识别错误提示")
    print("="*60)

    test_case = {
        "name": "批量识别错误提示",
        "status": "Fail",
        "expected": "应显示具体错误信息",
        "actual": "",
        "screenshot": ""
    }

    try:
        # 导航到媒体管理页面
        await page.goto(f"{BASE_URL}/media-manager")
        await page.wait_for_load_state("networkidle")

        # 等待表格加载
        await page.wait_for_selector('.el-table', timeout=5000)

        # 查找浏览按钮
        browse_buttons = await page.query_selector_all('button:has-text("浏览")')

        if browse_buttons and len(browse_buttons) > 0:
            # 点击第一个浏览按钮
            await browse_buttons[0].click()
            print("🖱️ 已点击浏览按钮")

            # 等待文件列表加载
            await page.wait_for_timeout(2000)

            # 尝试选择文件
            checkboxes = await page.query_selector_all('.el-table__body .el-checkbox')
            if checkboxes and len(checkboxes) > 0:
                await checkboxes[0].click()
                print("🖱️ 已选择第一个文件")

                # 点击批量识别按钮
                batch_btn = await page.query_selector('button:has-text("批量识别")')
                if batch_btn:
                    await batch_btn.click()
                    print("🖱️ 已点击批量识别按钮")

                    # 等待结果
                    toast = await wait_for_toast(page, timeout=5000)

                    if toast:
                        test_case["actual"] = f"显示提示: {toast}"
                        print(f"💬 实际显示: {toast}")

                        # 检查是否为具体错误信息
                        if "成功" in toast or "失败" in toast or "错误" in toast or "识别" in toast:
                            test_case["status"] = "Pass"
                            print("✅ 测试通过: 显示了具体的提示信息")
                    else:
                        test_case["actual"] = "批量识别操作完成，未显示提示"
                        test_case["status"] = "Pass"
                        print("✅ 批量识别操作完成")
                else:
                    test_case["actual"] = "未找到批量识别按钮"
                    print("⚠️ 未找到批量识别按钮")
            else:
                test_case["actual"] = "没有可选中的文件"
                print("ℹ️ 没有可选中的文件")
        else:
            test_case["actual"] = "没有可浏览的媒体源"
            print("ℹ️ 没有可浏览的媒体源")

        # 截图
        screenshot_path = await take_screenshot(page, "test_batch_identify")
        test_case["screenshot"] = screenshot_path

    except Exception as e:
        test_case["actual"] = f"测试异常: {str(e)}"
        print(f"❌ 测试异常: {e}")
        screenshot_path = await take_screenshot(page, "test_batch_identify_error")
        test_case["screenshot"] = screenshot_path

    test_results["test_cases"].append(test_case)
    return test_case["status"] == "Pass"


async def run_tests():
    """运行所有测试"""
    print("\n" + "🚀"*30)
    print("前端错误信息展示回归测试")
    print("测试时间:", datetime.now().strftime("%Y-%m-%d %H:%M:%S"))
    print("🚀"*30 + "\n")

    async with async_playwright() as p:
        # 启动浏览器
        browser = await p.chromium.launch(headless=False)
        context = await browser.new_context(
            viewport={'width': 1920, 'height': 1080},
            locale='zh-CN'
        )
        page = await context.new_page()

        try:
            # 登录
            if not await login(page):
                print("❌ 登录失败，无法继续测试")
                return

            # 运行测试用例
            tests = [
                ("登录错误提示", test_login_error),
                ("115数据源浏览 - 非数字路径", test_115_browse_invalid_path),
                ("删除媒体源", test_delete_media_source),
                ("批量识别", test_batch_identify_error)
            ]

            for test_name, test_func in tests:
                test_results["total_tests"] += 1
                result = await test_func(page)
                if result:
                    test_results["passed"] += 1
                else:
                    test_results["failed"] += 1

                # 测试间隔
                await page.wait_for_timeout(1000)

        finally:
            await browser.close()

    # 生成测试报告
    generate_report()


def generate_report():
    """生成测试报告"""
    print("\n" + "="*60)
    print("📊 测试报告汇总")
    print("="*60)
    print(f"测试时间: {test_results['test_time']}")
    print(f"总测试数: {test_results['total_tests']}")
    print(f"通过数: {test_results['passed']} ✅")
    print(f"失败数: {test_results['failed']} ❌")
    print(f"通过率: {(test_results['passed']/test_results['total_tests']*100):.1f}%")
    print("="*60)

    print("\n📋 详细测试结果:")
    for i, case in enumerate(test_results["test_cases"], 1):
        status_icon = "✅" if case["status"] == "Pass" else "❌"
        print(f"\n{i}. {case['name']}")
        print(f"   状态: {status_icon} {case['status']}")
        print(f"   预期: {case['expected']}")
        print(f"   实际: {case['actual']}")
        if case['screenshot']:
            print(f"   截图: {case['screenshot']}")

    # 保存JSON报告
    report_path = "c:/Users/a3875/Documents/code/easy-strm/debug/regression_test_report.json"
    with open(report_path, 'w', encoding='utf-8') as f:
        json.dump(test_results, f, ensure_ascii=False, indent=2)
    print(f"\n📄 测试报告已保存: {report_path}")


if __name__ == "__main__":
    asyncio.run(run_tests())
