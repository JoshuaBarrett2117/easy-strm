import fs from "node:fs/promises";
import path from "node:path";
import playwright from "../easy-strm-front/node_modules/playwright/index.js";

const { chromium, request } = playwright;

const FRONTEND_URL = "http://127.0.0.1:3001";
const BACKEND_API = "http://127.0.0.1:8082";
const USERNAME = "codex_e2e_20260527";
const PASSWORD = "admin";
const PASSWORD_MD5 = "21232f297a57a5a743894a0e4a801fc3";
const ROOT_DIR = path.resolve(".");
const DEBUG_DIR = path.join(ROOT_DIR, "debug", "real-backend-browser-20260527");

const routes = [
  ["/dashboard/home", "仪表盘"],
  ["/dashboard/media-library", "媒体资产台账"],
  ["/dashboard/sync-tasks", "同步入库"],
  ["/dashboard/pending-media", "待处理"],
  ["/dashboard/tasks", "任务中心"],
  ["/dashboard/media-manager", "文件工作台"],
  ["/dashboard/strm-config", "STRM 配置"],
  ["/dashboard/cloud115", "115 云管理"],
  ["/dashboard/category-strategy", "整理规则"],
  ["/dashboard/settings", "系统设置"],
  ["/dashboard/system-logs", "系统日志"],
  ["/dashboard/network", "网络测试"],
  ["/dashboard/cache", "缓存管理"]
];

async function loginViaApi() {
  const api = await request.newContext({ baseURL: BACKEND_API });
  const response = await api.post("/login", {
    data: { name: USERNAME, password: PASSWORD_MD5 }
  });
  const data = await response.json();
  if (!response.ok() || !data.token) {
    throw new Error(`登录失败: ${JSON.stringify(data)}`);
  }
  return { api, token: data.token, userId: String(data.user_id || "") };
}

async function createFixtureSource(api, token) {
  const fixtureRoot = path.join(DEBUG_DIR, `fixture-${Date.now()}`);
  const sourceDir = path.join(fixtureRoot, "source");
  const targetDir = path.join(fixtureRoot, "target");
  await fs.mkdir(sourceDir, { recursive: true });
  await fs.mkdir(targetDir, { recursive: true });
  await fs.writeFile(path.join(sourceDir, "Codex.Real.Backend.2026.1080p.mkv"), "codex fixture\n", "utf8");

  const authApi = await request.newContext({
    baseURL: BACKEND_API,
    extraHTTPHeaders: { Authorization: `Bearer ${token}` }
  });
  const sourceName = `codex_real_backend_${Date.now()}`;
  const response = await authApi.post("/media/sources", {
    data: {
      name: sourceName,
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
  const data = await response.json();
  if (!response.ok()) {
    throw new Error(`创建临时媒体源失败: ${JSON.stringify(data)}`);
  }
  const sourceId = data?.data?.id || data?.data?.data?.id;
  if (!sourceId) {
    throw new Error(`创建临时媒体源响应缺少 id: ${JSON.stringify(data)}`);
  }
  return { authApi, sourceId, sourceName, fixtureRoot };
}

async function main() {
  await fs.mkdir(DEBUG_DIR, { recursive: true });
  const { token, userId } = await loginViaApi();
  let browser;
  let authApi;
  let sourceId;
  const report = {
    ok: false,
    routes: [],
    coreFlow: {},
    consoleErrors: [],
    pageErrors: [],
    screenshots: {}
  };

  try {
    const fixture = await createFixtureSource(null, token);
    authApi = fixture.authApi;
    sourceId = fixture.sourceId;
    report.coreFlow.sourceId = fixture.sourceId;
    report.coreFlow.sourceName = fixture.sourceName;

    browser = await chromium.launch({ headless: true, args: ["--no-proxy-server"] });
    const context = await browser.newContext({ viewport: { width: 1500, height: 960 } });
    const page = await context.newPage();
    page.on("console", (message) => {
      if (message.type() === "error") {
        report.consoleErrors.push(message.text());
      }
    });
    page.on("pageerror", (error) => {
      report.pageErrors.push(error.message);
    });

    await page.goto(`${FRONTEND_URL}/login`, { waitUntil: "networkidle" });
    await page.locator('input[type="text"]').first().fill(USERNAME);
    await page.locator('input[type="password"]').first().fill(PASSWORD);
    await page.locator(".login-btn").click();
    await page.waitForURL("**/dashboard/**", { timeout: 15000 });
    report.screenshots.login = path.join(DEBUG_DIR, "login-dashboard.png");
    await page.screenshot({ path: report.screenshots.login, fullPage: true });

    for (const [route, expectedTitle] of routes) {
      await page.goto(`${FRONTEND_URL}${route}`, { waitUntil: "networkidle", timeout: 30000 });
      await page.locator(".shell-header .header-title").waitFor({ timeout: 15000 });
      const title = (await page.locator(".shell-header .header-title").innerText()).trim();
      const body = await page.locator("body").innerText({ timeout: 5000 });
      report.routes.push({
        route,
        expectedTitle,
        title,
        ok: title === expectedTitle,
        hasNetworkError: body.includes("网络错误") || body.includes("登录已过期")
      });
      if (route === "/dashboard/home" || route === "/dashboard/media-manager" || route === "/dashboard/cloud115") {
        const name = route.replaceAll("/", "_").replace(/^_/, "") + ".png";
        const file = path.join(DEBUG_DIR, name);
        await page.screenshot({ path: file, fullPage: true });
        report.screenshots[route] = file;
      }
    }

    await page.goto(`${FRONTEND_URL}/dashboard/sync-tasks?source_id=${sourceId}`, { waitUntil: "networkidle" });
    await page.getByRole("heading", { name: "同步入库工作台" }).waitFor({ timeout: 15000 });
    await page.getByText(fixture.sourceName).first().waitFor({ timeout: 15000 });
    await page.getByRole("button", { name: "全量同步" }).click();
    await page.waitForTimeout(2500);
    const syncBody = await page.locator("body").innerText({ timeout: 5000 });
    report.coreFlow.syncPageContainsSource = syncBody.includes(fixture.sourceName);
    report.coreFlow.syncTriggered = syncBody.includes("任务") || syncBody.includes("同步") || syncBody.includes("扫描");

    await page.goto(`${FRONTEND_URL}/dashboard/media-library?source_id=${sourceId}`, { waitUntil: "networkidle" });
    await page.getByRole("heading", { name: "媒体资产台账" }).waitFor({ timeout: 15000 });
    await page.waitForTimeout(1500);
    const libraryBody = await page.locator("body").innerText({ timeout: 5000 });
    report.coreFlow.libraryShowsFixture = libraryBody.includes("Codex.Real.Backend.2026.1080p.mkv")
      || libraryBody.includes("Codex.Real.Backend.2026");
    report.screenshots.coreFlow = path.join(DEBUG_DIR, "core-flow-library.png");
    await page.screenshot({ path: report.screenshots.coreFlow, fullPage: true });

    report.ok = report.routes.every((item) => item.ok) && report.coreFlow.syncPageContainsSource;
    console.log(JSON.stringify(report, null, 2));
  } finally {
    if (authApi && sourceId) {
      await authApi.delete(`/media/sources/${sourceId}`).catch(() => {});
    }
    if (browser) {
      await browser.close();
    }
  }
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
