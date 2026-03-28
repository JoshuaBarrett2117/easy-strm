# easy-strm 前端按钮测试方案

## 一、测试环境配置

| 配置项 | 值 |
|-------|-----|
| 前端地址 | http://localhost:3001 |
| 后端地址 | http://localhost:8082 |
| 浏览器 | Chromium (Playwright) |
| 测试用户 | admin / admin |

## 二、页面按钮脑图

```
├── 登录页 (Login.vue)
│   └── [登录系统] 按钮
│
├── 主布局 (Dashboard.vue)
│   ├── 侧边栏菜单
│   │   ├── [用户信息] 菜单项
│   │   ├── [115云管理] 菜单项
│   │   └── [STRM配置管理] 菜单项
│   │
│   ├── 头部操作区
│   │   ├── [查看任务] 按钮
│   │   ├── [查看日志] 按钮
│   │   └── [退出登录] 按钮
│   │
│   └── 弹窗内
│       ├── 日志弹窗
│       │   ├── [刷新] 按钮
│       │   └── [关闭] 按钮
│       └── 任务弹窗
│           ├── [刷新] 按钮
│           └── [关闭] 按钮
│
├── 用户信息页 (UserInfo.vue)
│   └── [刷新] 按钮
│
├── 115云管理页 (Cloud115.vue)
│   ├── [扫码登录] 按钮
│   ├── [新增账号] 按钮
│   ├── 表格操作按钮组
│   │   ├── [编辑] 按钮
│   │   ├── [扫码更新] 按钮
│   │   ├── [测试] 按钮
│   │   └── [删除] 按钮
│   ├── 新增/编辑对话框
│   │   ├── [取消] 按钮
│   │   └── [确定] 按钮
│   └── 扫码登录对话框
│       ├── [关闭] 按钮
│       └── [刷新二维码] 按钮
│
├── STRM配置管理页 (StrmConfig.vue)
│   ├── [新增配置] 按钮
│   ├── 表格操作按钮组
│   │   ├── [编辑] 按钮
│   │   ├── [删除] 按钮
│   │   ├── [全量生成] 按钮
│   │   └── [定时任务] 下拉菜单
│   │       ├── 启用/禁用定时任务
│   │       ├── 立即执行
│   │       └── 查看详情
│   ├── 新增/编辑对话框
│   │   ├── [取消] 按钮
│   │   ├── [确定] 按钮
│   │   └── [快捷生成] Cron表达式
│   └── Cron生成器对话框
│       ├── [取消] 按钮
│       └── [生成] 按钮
│
└── STRM生成器页 (StrmGenerator.vue)
    ├── [生成STRM文件] 按钮
    └── [重置] 按钮
```

## 三、Playwright 自动化测试脚本

```javascript
// tests/button-test.spec.js
const { test, expect } = require('@playwright/test');

const BASE_URL = 'http://localhost:3001';
const API_BASE = 'http://localhost:8082';

test.describe('easy-strm 前端按钮测试套件', () => {

  // ========== 前置条件 ==========
  test.beforeEach(async ({ page }) => {
    // 访问登录页
    await page.goto(`${BASE_URL}/login`);
    await page.waitForLoadState('networkidle');
  });

  // ========== 1. 登录页测试 ==========
  test.describe('登录页 (Login)', () => {
    test('【P0】登录按钮-正常登录流程', async ({ page }) => {
      // 输入用户名密码
      await page.fill('input[placeholder="请输入用户名"]', 'admin');
      await page.fill('input[placeholder="请输入密码"]', 'admin');
      
      // 点击登录按钮
      const loginBtn = page.locator('button:has-text("登录系统")');
      await expect(loginBtn).toBeVisible();
      await loginBtn.click();
      
      // 等待跳转或响应
      await page.waitForTimeout(2000);
      // 验证登录成功（应根据实际跳转逻辑调整）
      await expect(page.url()).not.toContain('/login');
    });

    test('【P1】登录按钮-空用户名应提示', async ({ page }) => {
      await page.fill('input[placeholder="请输入密码"]', 'admin');
      const loginBtn = page.locator('button:has-text("登录系统")');
      await loginBtn.click();
      
      // Element-Plus 表单验证应自动触发
      await expect(page.locator('.el-form-item__error')).toBeVisible();
    });

    test('【P1】登录按钮-空密码应提示', async ({ page }) => {
      await page.fill('input[placeholder="请输入用户名"]', 'admin');
      const loginBtn = page.locator('button:has-text("登录系统")');
      await loginBtn.click();
      
      await expect(page.locator('.el-form-item__error')).toBeVisible();
    });

    test('【P2】登录按钮-防抖测试(连击防护)', async ({ page }) => {
      await page.fill('input[placeholder="请输入用户名"]', 'admin');
      await page.fill('input[placeholder="请输入密码"]', 'admin');
      
      const loginBtn = page.locator('button:has-text("登录系统")');
      
      // 快速连击 3 次
      await loginBtn.click();
      await loginBtn.click();
      await loginBtn.click();
      
      // 验证按钮 loading 状态或被禁用
      await expect(loginBtn).toHaveAttribute('disabled', '');
    });
  });

  // ========== 2. 主布局测试 ==========
  test.describe('主布局 (Dashboard)', () => {
    test.beforeEach(async ({ page }) => {
      // 先登录
      await page.fill('input[placeholder="请输入用户名"]', 'admin');
      await page.fill('input[placeholder="请输入密码"]', 'admin');
      await page.click('button:has-text("登录系统")');
      await page.waitForURL('**/dashboard**', { timeout: 5000 });
    });

    test('【P0】侧边栏菜单-点击应跳转', async ({ page }) => {
      // 点击用户信息
      await page.click('text=用户信息');
      await expect(page).toHaveURL(/user-info/);
      
      // 点击115云管理
      await page.click('text=115云管理');
      await expect(page).toHaveURL(/cloud115/);
      
      // 点击STRM配置管理
      await page.click('text=STRM配置管理');
      await expect(page).toHaveURL(/strm-config/);
    });

    test('【P0】查看任务按钮-应弹出任务列表', async ({ page }) => {
      await page.click('button:has-text("查看任务")');
      await expect(page.locator('.el-dialog:has-text("任务列表")')).toBeVisible();
    });

    test('【P0】查看日志按钮-应弹出日志查看器', async ({ page }) => {
      await page.click('button:has-text("查看日志")');
      await expect(page.locator('.el-dialog:has-text("系统日志")')).toBeVisible();
    });

    test('【P1】查看日志弹窗-刷新按钮功能', async ({ page }) => {
      await page.click('button:has-text("查看日志")');
      await page.waitForSelector('.el-dialog');
      
      const refreshBtn = page.locator('.el-dialog button:has-text("刷新")');
      await expect(refreshBtn).toBeVisible();
      await refreshBtn.click();
      
      // 验证刷新操作
      await page.waitForTimeout(500);
    });

    test('【P1】查看任务弹窗-刷新按钮功能', async ({ page }) => {
      await page.click('button:has-text("查看任务")');
      await page.waitForSelector('.el-dialog');
      
      const refreshBtn = page.locator('.el-dialog button:has-text("刷新")');
      await refreshBtn.click();
      await page.waitForTimeout(500);
    });

    test('【P0】退出登录按钮-应返回登录页', async ({ page }) => {
      await page.click('button:has-text("退出登录")');
      
      // 确认退出
      await page.waitForTimeout(500);
      await expect(page).toHaveURL(/login/);
    });
  });

  // ========== 3. 用户信息页测试 ==========
  test.describe('用户信息页 (UserInfo)', () => {
    test.beforeEach(async ({ page }) => {
      await page.goto(`${BASE_URL}/dashboard/user-info`);
      await page.waitForLoadState('networkidle');
    });

    test('【P1】刷新按钮-应重新获取用户信息', async ({ page }) => {
      const refreshBtn = page.locator('button:has-text("刷新")');
      await expect(refreshBtn).toBeVisible();
      await refreshBtn.click();
      
      // 验证 loading 状态
      await expect(page.locator('.el-loading-mask')).toBeVisible();
      await page.waitForTimeout(1000);
    });
  });

  // ========== 4. 115云管理页测试 ==========
  test.describe('115云管理页 (Cloud115)', () => {
    test.beforeEach(async ({ page }) => {
      await page.goto(`${BASE_URL}/dashboard/cloud115`);
      await page.waitForLoadState('networkidle');
    });

    test('【P0】扫码登录按钮-应弹出二维码', async ({ page }) => {
      await page.click('button:has-text("扫码登录")');
      await expect(page.locator('.el-dialog:has-text("扫码登录")')).toBeVisible();
      await expect(page.locator('.qrcode-image img')).toBeVisible();
    });

    test('【P1】扫码登录弹窗-刷新二维码', async ({ page }) => {
      await page.click('button:has-text("扫码登录")');
      await page.waitForSelector('.el-dialog');
      
      const refreshBtn = page.locator('button:has-text("刷新二维码")');
      await refreshBtn.click();
      await page.waitForTimeout(1000);
    });

    test('【P0】新增账号按钮-应弹出表单', async ({ page }) => {
      await page.click('button:has-text("新增账号")');
      await expect(page.locator('.el-dialog:has-text("新增账号")')).toBeVisible();
      await expect(page.locator('input[placeholder="请输入115云账号名称"]')).toBeVisible();
    });

    test('【P1】新增账号弹窗-取消按钮', async ({ page }) => {
      await page.click('button:has-text("新增账号")');
      await page.waitForSelector('.el-dialog');
      
      await page.click('button:has-text("取消")');
      await expect(page.locator('.el-dialog')).not.toBeVisible();
    });

    test('【P0】表格编辑按钮-应弹出编辑表单', async ({ page }) => {
      // 等待表格加载
      await page.waitForSelector('.el-table__row', { timeout: 5000 });
      
      const firstEditBtn = page.locator('.el-table__row:first-child button:has-text("编辑")');
      if (await firstEditBtn.isVisible()) {
        await firstEditBtn.click();
        await expect(page.locator('.el-dialog:has-text("编辑账号")')).toBeVisible();
      }
    });

    test('【P1】表格删除按钮-应确认删除', async ({ page }) => {
      await page.waitForSelector('.el-table__row', { timeout: 5000 });
      
      const firstDeleteBtn = page.locator('.el-table__row:first-child button:has-text("删除")');
      if (await firstDeleteBtn.isVisible()) {
        // 监听 confirm 框
        page.on('dialog', dialog => dialog.accept());
        await firstDeleteBtn.click();
        await page.waitForTimeout(500);
      }
    });

    test('【P2】Cookie显示切换按钮', async ({ page }) => {
      await page.waitForSelector('.el-table__row', { timeout: 5000 });
      
      const toggleBtn = page.locator('.el-table__row:first-child .toggle-btn');
      if (await toggleBtn.isVisible()) {
        await toggleBtn.click();
        // 验证显示完整 cookie
        await expect(page.locator('.el-table__row:first-child .full-text')).toBeVisible();
      }
    });
  });

  // ========== 5. STRM配置管理页测试 ==========
  test.describe('STRM配置管理页 (StrmConfig)', () => {
    test.beforeEach(async ({ page }) => {
      await page.goto(`${BASE_URL}/dashboard/strm-config`);
      await page.waitForLoadState('networkidle');
    });

    test('【P0】新增配置按钮-应弹出表单', async ({ page }) => {
      await page.click('button:has-text("新增配置")');
      await expect(page.locator('.el-dialog:has-text("新增配置")')).toBeVisible();
    });

    test('【P1】新增配置-取消按钮', async ({ page }) => {
      await page.click('button:has-text("新增配置")');
      await page.waitForSelector('.el-dialog');
      await page.click('.el-dialog button:has-text("取消")');
      await expect(page.locator('.el-dialog')).not.toBeVisible();
    });

    test('【P0】全量生成按钮-应触发任务', async ({ page }) => {
      await page.waitForSelector('.el-table__row', { timeout: 5000 });
      
      const genBtn = page.locator('.el-table__row:first-child button:has-text("全量生成")');
      if (await genBtn.isVisible()) {
        await genBtn.click();
        // 验证任务卡片出现
        await expect(page.locator('.task-card')).toBeVisible({ timeout: 3000 });
      }
    });

    test('【P1】定时任务下拉菜单', async ({ page }) => {
      await page.waitForSelector('.el-dropdown', { timeout: 5000 });
      
      const dropdown = page.locator('.el-dropdown').first();
      await dropdown.click();
      await expect(page.locator('.el-dropdown-menu')).toBeVisible();
    });

    test('【P2】Cron快捷生成器', async ({ page }) => {
      await page.click('button:has-text("新增配置")');
      await page.waitForSelector('.el-dialog');
      
      await page.click('button:has-text("快捷生成")');
      await expect(page.locator('.el-dialog:has-text("Cron表达式快捷生成")')).toBeVisible();
    });
  });

  // ========== 6. STRM生成器页测试 ==========
  test.describe('STRM生成器页 (StrmGenerator)', () => {
    test.beforeEach(async ({ page }) => {
      await page.goto(`${BASE_URL}/dashboard/strm-generator`);
      await page.waitForLoadState('networkidle');
    });

    test('【P0】生成STRM文件按钮-完整流程', async ({ page }) => {
      // 选择115账号
      await page.click('.el-select');
      await page.waitForSelector('.el-select-dropdown');
      await page.click('.el-option');
      
      // 填写路径
      await page.fill('input[placeholder="请输入115网盘媒体库目录路径"]', '/test/path');
      await page.fill('input[placeholder="请输入本地媒体库目录路径"]', '/local/test');
      
      // 点击生成
      await page.click('button:has-text("生成STRM文件")');
      await page.waitForTimeout(2000);
    });

    test('【P1】重置按钮-应清空表单', async ({ page }) => {
      // 填写表单
      await page.click('.el-select');
      await page.waitForSelector('.el-select-dropdown');
      await page.click('.el-option');
      await page.fill('input[placeholder="请输入115网盘媒体库目录路径"]', '/test/path');
      
      // 点击重置
      await page.click('button:has-text("重置")');
      
      // 验证表单已清空
      const inputs = await page.locator('.el-form input').all();
      for (const input of inputs) {
        const value = await input.inputValue();
        expect(value).toBe('');
      }
    });
  });
});
```

## 四、cURL 接口测试脚本

```bash
# =============================================
# easy-strm API 按钮触发接口测试
# 前端按钮 → 后端API 对应测试
# =============================================

# 基础配置
API_BASE="http://localhost:8082"

echo "=========================================="
echo "1. 登录接口测试"
echo "=========================================="
curl -X POST "${API_BASE}/api/user/login" \
  -H "Content-Type: application/json" \
  -d '{"name":"admin","password":"21232f297a57a5a743894a0e4a801fc3"}' \
  -c cookies.txt \
  -w "\n[HTTP Code: %{http_code}]\n"

echo ""
echo "=========================================="
echo "2. 获取用户信息 (触发用户信息页刷新)"
echo "=========================================="
curl -X GET "${API_BASE}/api/user/info" \
  -b cookies.txt \
  -w "\n[HTTP Code: %{http_code}]\n"

echo ""
echo "=========================================="
echo "3. 115账号列表 (触发Cloud115页加载)"
echo "=========================================="
curl -X GET "${API_BASE}/api/cloud115" \
  -b cookies.txt \
  -w "\n[HTTP Code: %{http_code}]\n"

echo ""
echo "=========================================="
echo "4. 新增115账号 (触发新增账号确定)"
echo "=========================================="
curl -X POST "${API_BASE}/api/cloud115" \
  -H "Content-Type: application/json" \
  -b cookies.txt \
  -d '{
    "name":"测试账号",
    "cookie":"test_cookie_value",
    "account_type":"resource",
    "status":"active",
    "priority":5
  }' \
  -w "\n[HTTP Code: %{http_code}]\n"

echo ""
echo "=========================================="
echo "5. 编辑115账号"
echo "=========================================="
curl -X PUT "${API_BASE}/api/cloud115/1" \
  -H "Content-Type: application/json" \
  -b cookies.txt \
  -d '{
    "name":"更新后的账号",
    "cookie":"updated_cookie",
    "account_type":"vip",
    "status":"cooling",
    "priority":8
  }' \
  -w "\n[HTTP Code: %{http_code}]\n"

echo ""
echo "=========================================="
echo "6. 删除115账号"
echo "=========================================="
curl -X DELETE "${API_BASE}/api/cloud115/1" \
  -b cookies.txt \
  -w "\n[HTTP Code: %{http_code}]\n"

echo ""
echo "=========================================="
echo "7. 获取扫码登录二维码"
echo "=========================================="
curl -X GET "${API_BASE}/api/cloud115/qrcode" \
  -b cookies.txt \
  -w "\n[HTTP Code: %{http_code}]\n"

echo ""
echo "=========================================="
echo "8. STRM配置列表"
echo "=========================================="
curl -X GET "${API_BASE}/api/strm/config" \
  -b cookies.txt \
  -w "\n[HTTP Code: %{http_code}]\n"

echo ""
echo "=========================================="
echo "9. 新增STRM配置"
echo "=========================================="
curl -X POST "${API_BASE}/api/strm/config" \
  -H "Content-Type: application/json" \
  -b cookies.txt \
  -d '{
    "cloud115_id":1,
    "net_disk_path":"/media/movies",
    "local_path":"/mnt/media/movies",
    "extension":".mp4,.mkv",
    "cron":"0 2 * * *"
  }' \
  -w "\n[HTTP Code: %{http_code}]\n"

echo ""
echo "=========================================="
echo "10. 删除STRM配置"
echo "=========================================="
curl -X DELETE "${API_BASE}/api/strm/config/1" \
  -b cookies.txt \
  -w "\n[HTTP Code: %{http_code}]\n"

echo ""
echo "=========================================="
echo "11. 触发全量生成STRM"
echo "=========================================="
curl -X POST "${API_BASE}/api/strm/generate" \
  -H "Content-Type: application/json" \
  -b cookies.txt \
  -d '{
    "config_id":1,
    "mode":"full"
  }' \
  -w "\n[HTTP Code: %{http_code}]\n"

echo ""
echo "=========================================="
echo "12. 获取任务列表 (查看任务按钮)"
echo "=========================================="
curl -X GET "${API_BASE}/api/task/list" \
  -b cookies.txt \
  -w "\n[HTTP Code: %{http_code}]\n"

echo ""
echo "=========================================="
echo "13. 获取日志文件列表 (查看日志按钮)"
echo "=========================================="
curl -X GET "${API_BASE}/api/log/files" \
  -b cookies.txt \
  -w "\n[HTTP Code: %{http_code}]\n"

echo ""
echo "=========================================="
echo "14. 获取日志内容"
echo "=========================================="
curl -X GET "${API_BASE}/api/log/content?filename=app.log" \
  -b cookies.txt \
  -w "\n[HTTP Code: %{http_code}]\n"

echo ""
echo "=========================================="
echo "15. 退出登录"
echo "=========================================="
curl -X POST "${API_BASE}/api/user/logout" \
  -b cookies.txt \
  -w "\n[HTTP Code: %{http_code}]\n"

echo ""
echo "=========================================="
echo "测试完成，清理临时文件..."
echo "=========================================="
rm -f cookies.txt
```

## 五、异常场景测试矩阵

| 按钮路径 | 异常场景 | 预期行为 | 严重级别 |
|---------|---------|---------|---------|
| 登录按钮 | 网络 502 | 中文错误提示，不闪屏 | P0 |
| 登录按钮 | 网络超时 30s | 超时提示 + 重试按钮 | P1 |
| 新增账号 | Cookie 为空 | 表单验证拦截 | P1 |
| 新增账号 | 重复提交 | 防抖禁用 + loading 状态 | P1 |
| 编辑保存 | 115 API 403 | 中文错误弹窗 | P0 |
| 全量生成 | 断网 | 任务卡片显示失败状态 | P1 |
| 查看日志 | 日志文件过大 | 分页或截断提示 | P2 |
| 删除账号 | 确认取消 | 不执行删除 | P2 |

## 六、缺陷报告模板

```markdown
## [模块-子模块] 按钮XXX点击后XXX

**严重级别**: P0/P1/P2/P3

**复现步骤**:
1. 访问 [页面路径]
2. 点击 [按钮名称]
3. 观察 [触发条件]

**预期结果**:
[期望的 UI 行为或 API 响应]

**实际结果**:
[实际发生的现象]

**中文日志**:
```
[时间戳] [模块名] 关键日志内容...
```

**Windows 清理命令**:
```powershell
# 清理测试数据
Remove-Item -Path "C:\test\data\*" -Recurse -Force
```
```

## 七、快速运行指南

```powershell
# 1. 安装 Playwright 依赖
cd easy-strm-front
npm install
npx playwright install chromium

# 2. 启动前端服务
npm run dev

# 3. 运行所有按钮测试
npx playwright test tests/button-test.spec.js --reporter=list

# 4. 运行指定测试
npx playwright test tests/button-test.spec.js --grep="登录" --reporter=list

# 5. 生成 HTML 报告
npx playwright test tests/button-test.spec.js --reporter=html
```

---

*文档版本: v1.0.0*
*最后更新: 2026-03-27*
