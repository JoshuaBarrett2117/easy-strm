import fs from "node:fs/promises";
import path from "node:path";
import playwright from "../easy-strm-front/node_modules/playwright/index.js";

const { chromium, request } = playwright;

const FRONTEND_URL = "http://127.0.0.1:3001";
const BACKEND_API = "http://127.0.0.1:8082";
const USERNAME = "codex_button_20260527";
const PASSWORD = "admin";
const PASSWORD_MD5 = "21232f297a57a5a743894a0e4a801fc3";
const DEBUG_DIR = path.resolve("debug", "button-matrix-20260527");

const result = {
  ok: false,
  passed: [],
  warnings: [],
  failed: [],
  skipped: [],
  cleanup: [],
  screenshots: {},
  consoleErrors: [],
  pageErrors: []
};

function pass(name, detail = "") {
  result.passed.push({ name, detail });
}

function warn(name, detail = "") {
  result.warnings.push({ name, detail });
}

function fail(name, error) {
  result.failed.push({ name, error: error?.message || String(error) });
}

function skip(name, reason) {
  result.skipped.push({ name, reason });
}

async function step(name, fn) {
  try {
    const detail = await fn();
    pass(name, detail || "");
  } catch (error) {
    fail(name, error);
  }
}

async function loginApi() {
  const api = await request.newContext({ baseURL: BACKEND_API });
  const response = await api.post("/login", {
    data: { name: USERNAME, password: PASSWORD_MD5 }
  });
  const data = await response.json();
  if (!response.ok() || !data.token) {
    throw new Error(`登录失败: ${JSON.stringify(data)}`);
  }
  return {
    token: data.token,
    userId: String(data.user_id || "")
  };
}

async function createAuthedApi(token) {
  return request.newContext({
    baseURL: BACKEND_API,
    extraHTTPHeaders: { Authorization: `Bearer ${token}` }
  });
}

async function responseJson(response) {
  try {
    return await response.json();
  } catch {
    return {};
  }
}

async function createFixtureSource(api) {
  const fixtureRoot = path.join(DEBUG_DIR, `fixture-${Date.now()}`);
  const sourceDir = path.join(fixtureRoot, "source");
  const targetDir = path.join(fixtureRoot, "target");
  await fs.mkdir(sourceDir, { recursive: true });
  await fs.mkdir(targetDir, { recursive: true });
  await fs.writeFile(path.join(sourceDir, "Button.Matrix.Movie.2026.1080p.mkv"), "button matrix fixture\n", "utf8");

  const name = `codex_button_source_${Date.now()}`;
  const response = await api.post("/media/sources", {
    data: {
      name,
      source_type: "local",
      path: sourceDir,
      organize_target_path: targetDir,
      auto_organize: false,
      watch_enabled: false,
      watch_interval: 300,
      emby_library_id: "",
      enabled: true,
      priority: 10
    }
  });
  const data = await responseJson(response);
  if (!response.ok()) {
    throw new Error(`创建临时媒体源失败: ${JSON.stringify(data)}`);
  }
  const id = data?.data?.id || data?.data?.data?.id;
  if (!id) {
    throw new Error(`创建临时媒体源响应缺少 id: ${JSON.stringify(data)}`);
  }
  return { id, name, fixtureRoot, sourceDir, targetDir };
}

async function getFirstCloudAccount(api) {
  const response = await api.get("/cloud115");
  const data = await responseJson(response);
  const rows = data?.data?.data || data?.data || [];
  return Array.isArray(rows) ? rows[0] : null;
}

async function gotoAndWait(page, route, title) {
  await page.goto(`${FRONTEND_URL}${route}`, { waitUntil: "networkidle", timeout: 30000 });
  await page.locator(".shell-header .header-title").waitFor({ timeout: 15000 });
  const actual = (await page.locator(".shell-header .header-title").innerText()).trim();
  if (title && actual !== title) {
    throw new Error(`标题不匹配: want=${title}, got=${actual}`);
  }
}

async function clickUnique(page, locator, name) {
  const count = await locator.count();
  if (count !== 1) {
    throw new Error(`${name} locator count=${count}`);
  }
  await locator.click();
}

async function closeVisibleDialogs(page) {
  const closeButtons = await page.locator(".el-dialog__headerbtn:visible").count();
  for (let i = 0; i < closeButtons; i += 1) {
    await page.locator(".el-dialog__headerbtn:visible").first().click().catch(() => {});
    await page.waitForTimeout(300);
  }
}

async function hasToastOrDialog(page) {
  await page.waitForTimeout(600);
  const toastCount = await page.locator(".el-message:visible").count();
  const dialogCount = await page.locator(".el-dialog:visible").count();
  return { toastCount, dialogCount };
}

async function main() {
  await fs.mkdir(DEBUG_DIR, { recursive: true });
  const { token, userId } = await loginApi();
  const api = await createAuthedApi(token);
  const fixture = await createFixtureSource(api);
  let browser;
  let tempCategoryId = null;
  let tempCloudId = null;
  let tempStrmConfigId = null;
  let tempPendingId = null;

  try {
    const pendingResponse = await api.post("/media/pending", {
      data: {
        source_kind: "local",
        source_id: fixture.id,
        source_path: path.join(fixture.sourceDir, "Button.Matrix.Pending.2026.mkv"),
        title: "Button Matrix Pending",
        year: 2026,
        media_type: "movie",
        reason: "按钮矩阵临时项"
      }
    });
    if (pendingResponse.ok()) {
      const pendingData = await responseJson(pendingResponse);
      tempPendingId = pendingData?.data?.id || pendingData?.data?.data?.id;
    } else {
      warn("待处理临时项创建", JSON.stringify(await responseJson(pendingResponse)));
    }

    const firstCloud = await getFirstCloudAccount(api);

    browser = await chromium.launch({ headless: true, args: ["--no-proxy-server"] });
    const context = await browser.newContext({ viewport: { width: 1520, height: 980 } });
    const page = await context.newPage();
    page.on("console", (message) => {
      if (message.type() === "error") {
        result.consoleErrors.push(message.text());
      }
    });
    page.on("pageerror", (error) => result.pageErrors.push(error.message));

    await step("登录页按钮", async () => {
      await page.goto(`${FRONTEND_URL}/login`, { waitUntil: "networkidle" });
      await page.locator('input[type="text"]').first().fill(USERNAME);
      await page.locator('input[type="password"]').first().fill(PASSWORD);
      await clickUnique(page, page.locator(".login-btn"), "登录系统");
      await page.waitForURL("**/dashboard/**", { timeout: 15000 });
      return page.url();
    });

    await step("顶部快捷与主题按钮", async () => {
      await gotoAndWait(page, "/dashboard/home", "仪表盘");
      await page.locator(".shell-header").getByRole("link", { name: "资产台账", exact: true }).click();
      await page.waitForURL("**/dashboard/media-library", { timeout: 15000 });
      await gotoAndWait(page, "/dashboard/home", "仪表盘");
      await page.locator(".shell-header").getByRole("link", { name: "任务中心", exact: true }).click();
      await page.waitForURL("**/dashboard/tasks", { timeout: 15000 });
      await gotoAndWait(page, "/dashboard/home", "仪表盘");
      await page.getByRole("button", { name: "深色" }).click();
      await page.getByRole("button", { name: "浅色" }).click();
      return "快捷跳转和主题切换完成";
    });

    await step("首页快捷入口按钮", async () => {
      const pairs = [
        ["打开资产台账", "**/dashboard/media-library"],
        ["同步入库", "**/dashboard/sync-tasks"],
        ["处理失败项", "**/dashboard/pending-media"],
        ["查看任务中心", "**/dashboard/tasks"]
      ];
      for (const [label, url] of pairs) {
        await gotoAndWait(page, "/dashboard/home", "仪表盘");
        await page.getByRole("link", { name: label }).click();
        await page.waitForURL(url, { timeout: 15000 });
      }
      return "4 个首页主入口完成";
    });

    await step("同步入库按钮组", async () => {
      await gotoAndWait(page, `/dashboard/sync-tasks?source_id=${fixture.id}`, "同步入库");
      await page.getByText(fixture.name).first().waitFor({ timeout: 15000 });
      await page.getByRole("button", { name: "刷新媒体源" }).click();
      await page.waitForTimeout(1000);
      await page.getByRole("button", { name: "全量同步" }).click();
      await page.waitForTimeout(2200);
      await page.getByRole("button", { name: "增量同步" }).click();
      await page.waitForTimeout(2200);
      await page.getByRole("button", { name: "执行入库" }).click();
      await page.waitForTimeout(2200);
      await page.getByRole("button", { name: "查看资产台账" }).first().click();
      await page.waitForURL("**/dashboard/media-library?source_id=*", { timeout: 15000 });
      return "刷新/全量/增量/入库/跳台账完成";
    });

    await step("资产台账按钮组", async () => {
      await gotoAndWait(page, `/dashboard/media-library?source_id=${fixture.id}`, "媒体资产台账");
      await page.waitForTimeout(1200);
      await page.locator(".shell-content").getByRole("button", { name: "同步入库", exact: true }).click();
      await page.waitForURL("**/dashboard/sync-tasks?source_id=*", { timeout: 15000 });
      await gotoAndWait(page, `/dashboard/media-library?source_id=${fixture.id}`, "媒体资产台账");
      await page.locator(".shell-content").getByRole("button", { name: "待处理", exact: true }).click();
      await page.waitForURL("**/dashboard/pending-media?source_id=*", { timeout: 15000 });
      await gotoAndWait(page, `/dashboard/media-library?source_id=${fixture.id}`, "媒体资产台账");
      const row = page.locator(".el-table__row").filter({ hasText: "Button.Matrix.Movie.2026" }).first();
      await row.waitFor({ timeout: 15000 });
      await row.getByRole("button", { name: "入库" }).click();
      await page.waitForTimeout(1800);
      await row.getByRole("button", { name: "STRM" }).click();
      await page.waitForTimeout(1800);
      await row.getByRole("button", { name: "刷新库" }).click();
      await page.waitForTimeout(1800);
      return "跳转与行内动作完成";
    });

    await step("待处理按钮组", async () => {
      if (!tempPendingId) {
        skip("待处理按钮组", "临时待处理项未创建");
        return "skipped";
      }
      await gotoAndWait(page, `/dashboard/pending-media?source_id=${fixture.id}`, "待处理");
      await page.waitForTimeout(1200);
      await page.getByRole("button", { name: "查看资产台账" }).click();
      await page.waitForURL("**/dashboard/media-library?source_id=*", { timeout: 15000 });
      await gotoAndWait(page, `/dashboard/pending-media?source_id=${fixture.id}`, "待处理");
      const row = page.locator(".el-table__row").filter({ hasText: "Button Matrix Pending" }).first();
      await row.waitFor({ timeout: 15000 });
      await row.getByRole("button", { name: "修正" }).click();
      await page.locator(".el-dialog:visible").last().waitFor({ timeout: 10000 });
      await page.locator(".el-dialog:visible").last().getByRole("button", { name: "保存", exact: true }).click();
      await page.waitForTimeout(1000);
      await closeVisibleDialogs(page);
      const ignoreButtons = row.getByRole("button", { name: "忽略" });
      if ((await ignoreButtons.count()) > 0) {
        await ignoreButtons.first().click();
        await page.waitForTimeout(1000);
        if ((await page.locator(".el-message-box:visible").count()) > 0) {
          await page.locator(".el-message-box:visible").getByRole("button", { name: "取消" }).click();
        } else if ((await page.locator(".el-message:visible").count()) === 0) {
          warn("待处理忽略", "点击后未出现确认框或消息，可能该状态不允许忽略");
        }
      } else {
        warn("待处理忽略", "当前行没有可见忽略按钮");
      }
      return "跳台账/修正弹窗保存路径/忽略路径触达";
    });

    await step("任务中心按钮组", async () => {
      await gotoAndWait(page, "/dashboard/tasks", "任务中心");
      await page.locator(".shell-content").getByRole("button", { name: "刷新", exact: true }).click();
      await page.waitForTimeout(1000);
      const detailButtons = page.getByRole("button", { name: "详情" });
      if ((await detailButtons.count()) > 0) {
        await detailButtons.first().click();
        await page.waitForTimeout(1800);
        if ((await page.locator(".el-drawer:visible").count()) > 0) {
          await page.locator(".el-drawer:visible .el-drawer__close-btn").click();
        } else {
          warn("任务中心详情", "详情按钮可点击，但未观察到详情抽屉打开");
        }
      } else {
        warn("任务中心详情", "当前无任务卡片可点详情");
      }
      return "刷新与详情完成";
    });

    await step("文件工作台媒体源按钮", async () => {
      await gotoAndWait(page, "/dashboard/media-manager", "文件工作台");
      await page.locator(".shell-content").getByRole("button", { name: "新增媒体源" }).first().click();
      await page.locator(".el-dialog:visible").waitFor({ timeout: 10000 });
      await closeVisibleDialogs(page);
      const row = page.locator(".el-table__row").filter({ hasText: fixture.name }).first();
      await row.waitFor({ timeout: 15000 });
      await row.getByRole("button", { name: "浏览" }).click();
      await page.locator(".el-dialog:visible").waitFor({ timeout: 15000 });
      await page.getByRole("button", { name: "刷新" }).last().click();
      await page.waitForTimeout(1000);
      await closeVisibleDialogs(page);
      return "新增弹窗/浏览弹窗/刷新完成";
    });

    await step("115 云管理按钮组", async () => {
      await gotoAndWait(page, "/dashboard/cloud115", "115 云管理");
      await page.getByRole("button", { name: "新增账号" }).click();
      await page.locator(".el-dialog:visible").waitFor({ timeout: 10000 });
      await closeVisibleDialogs(page);
      await page.getByRole("button", { name: "扫码登录" }).click();
      await page.locator(".el-dialog:visible").waitFor({ timeout: 15000 });
      await closeVisibleDialogs(page);
      if (firstCloud) {
        const row = page.locator(".el-table__row").filter({ hasText: firstCloud.name }).first();
        await row.getByRole("button", { name: "编辑" }).click();
        await page.locator(".el-dialog:visible").waitFor({ timeout: 10000 });
        await closeVisibleDialogs(page);
        await row.getByRole("button", { name: "测试" }).click();
        await page.waitForTimeout(3000);
      } else {
        warn("115 行内按钮", "无 115 账号，跳过编辑/测试");
      }
      return "新增/扫码/编辑/测试完成或触达";
    });

    await step("整理规则 CRUD 按钮", async () => {
      const categoryName = `按钮矩阵${Date.now()}`;
      const createResponse = await api.post("/media/categories", {
        data: {
          name: categoryName,
          media_type: "movie",
          target_path: `/电影/${categoryName}`,
          match_rules: { keywords: [categoryName] },
          enabled: true
        }
      });
      const createData = await responseJson(createResponse);
      tempCategoryId = createData?.data?.id || createData?.data?.data?.id;
      if (!createResponse.ok() || !tempCategoryId) {
        throw new Error(`创建临时分类失败: ${JSON.stringify(createData)}`);
      }
      await gotoAndWait(page, "/dashboard/category-strategy", "整理规则");
      await page.locator(".shell-content").getByRole("button", { name: "刷新", exact: true }).click();
      await page.waitForTimeout(1000);
      await page.getByRole("button", { name: "新增分类" }).click();
      await page.getByRole("button", { name: "取消" }).click();
      await page.getByText(categoryName).click();
      await page.getByRole("button", { name: "删除" }).click();
      await page.locator(".el-message-box:visible").waitFor({ timeout: 10000 });
      await page.locator(".el-message-box:visible").getByRole("button", { name: "取消" }).click();
      return "刷新/新增取消/选择/删除取消完成";
    });

    await step("STRM 配置按钮组", async () => {
      await gotoAndWait(page, "/dashboard/strm-config", "STRM 配置");
      await page.getByRole("button", { name: "新增配置" }).click();
      await page.locator(".el-dialog:visible").waitFor({ timeout: 10000 });
      await closeVisibleDialogs(page);
      if (!firstCloud) {
        warn("STRM 临时配置", "无 115 账号，跳过临时配置创建");
        return "新增弹窗完成";
      }
      const response = await api.post("/strm/config", {
        data: {
          cloud115_id: firstCloud.id,
          net_disk_path: `/codex-button-${Date.now()}`,
          local_path: path.join(DEBUG_DIR, "strm-output"),
          cron: "0 0 * * *",
          extension: ".strm",
          sync_mode: "manual",
          source_account: 0,
          target_account: 0,
          target_directory: "",
          auto_cleanup: false,
          cleanup_threshold: 0,
          cleanup_policy: "",
          max_concurrency: 1
        }
      });
      const data = await responseJson(response);
      tempStrmConfigId = data?.data?.id || data?.data?.data?.id;
      if (!response.ok() || !tempStrmConfigId) {
        warn("STRM 临时配置创建", JSON.stringify(data));
        return "新增弹窗完成，临时配置 API 未创建";
      }
      await page.reload({ waitUntil: "networkidle" });
      const row = page.locator(".el-table__row").filter({ hasText: String(tempStrmConfigId) }).first();
      await row.waitFor({ timeout: 15000 });
      await row.getByRole("button", { name: "编辑" }).click();
      await page.locator(".el-dialog:visible").waitFor({ timeout: 10000 });
      await closeVisibleDialogs(page);
      await row.getByRole("button", { name: "删除" }).click();
      await page.locator(".el-message-box:visible").waitFor({ timeout: 10000 });
      await page.getByRole("button", { name: "取消" }).click();
      return "新增/编辑/删除取消完成";
    });

    await step("系统设置按钮组", async () => {
      await gotoAndWait(page, "/dashboard/settings", "系统设置");
      await page.locator(".shell-content").getByRole("button", { name: "重置", exact: true }).first().click();
      await page.waitForTimeout(800);
      await page.getByRole("button", { name: "保存 TMDB 配置" }).click();
      await hasToastOrDialog(page);
      await page.locator(".shell-content").getByRole("button", { name: "测试连接", exact: true }).click();
      await page.waitForTimeout(2000);
      return "重置/TMDB 保存/Emby 测试触达";
    });

    await step("系统日志按钮组", async () => {
      await gotoAndWait(page, "/dashboard/system-logs", "系统日志");
      await page.locator(".shell-content").getByRole("button", { name: "刷新", exact: true }).click();
      await page.waitForTimeout(1000);
      const auto = page.getByText("自动刷新");
      if ((await auto.count()) > 0) {
        await auto.first().click();
        await page.waitForTimeout(500);
        await auto.first().click();
      }
      return "刷新/自动刷新触达";
    });

    await step("网络测试按钮组", async () => {
      await gotoAndWait(page, "/dashboard/network", "网络测试");
      await page.getByRole("button", { name: "重新探测" }).click();
      await page.waitForTimeout(3500);
      return "重新探测完成";
    });

    await step("缓存管理按钮组", async () => {
      await gotoAndWait(page, "/dashboard/cache", "缓存管理");
      await page.getByRole("button", { name: "刷新概览" }).click();
      await page.waitForTimeout(1000);
      const clearAll = page.getByRole("button", { name: "清理全部缓存" });
      if ((await clearAll.count()) > 0) {
        await clearAll.click();
        await page.locator(".el-message-box:visible").waitFor({ timeout: 10000 });
        await page.getByRole("button", { name: "取消" }).click();
      }
      return "刷新概览/清理全部取消完成";
    });

    result.screenshots.final = path.join(DEBUG_DIR, "button-matrix-final.png");
    await page.screenshot({ path: result.screenshots.final, fullPage: true });
  } finally {
    if (tempStrmConfigId) {
      const response = await api.delete(`/strm/config/${tempStrmConfigId}`).catch(() => null);
      result.cleanup.push({ item: "strm_config", id: tempStrmConfigId, ok: Boolean(response?.ok?.()) });
    }
    if (tempCategoryId) {
      const response = await api.delete(`/media/categories/${tempCategoryId}`).catch(() => null);
      result.cleanup.push({ item: "category", id: tempCategoryId, ok: Boolean(response?.ok?.()) });
    }
    if (fixture?.id) {
      const response = await api.delete(`/media/sources/${fixture.id}`).catch(() => null);
      result.cleanup.push({ item: "media_source", id: fixture.id, ok: Boolean(response?.ok?.()) });
    }
    if (browser) {
      await browser.close();
    }
  }

  result.ok = result.failed.length === 0;
  console.log(JSON.stringify(result, null, 2));
  if (!result.ok) {
    process.exit(1);
  }
}

main().catch((error) => {
  fail("脚本执行", error);
  console.error(JSON.stringify(result, null, 2));
  process.exit(1);
});
