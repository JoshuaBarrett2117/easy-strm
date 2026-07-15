import fs from "node:fs/promises";
import path from "node:path";
import playwright from "../easy-strm-front/node_modules/playwright/index.js";

const { chromium, request } = playwright;

const FRONTEND_URL = "http://127.0.0.1:3001";
const BACKEND_API = "http://127.0.0.1:8082";
const USERNAME = "codex_exhaustive_20260626";
const PASSWORD = "admin";
const PASSWORD_MD5 = "21232f297a57a5a743894a0e4a801fc3";
const DEBUG_DIR = path.resolve("debug", "exhaustive-button-20260626");
const REPORT_PATH = path.join(DEBUG_DIR, "report.json");

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

const report = {
  ok: false,
  startedAt: new Date().toISOString(),
  summary: {
    routes: routes.length,
    enumerated: 0,
    clicked: 0,
    skipped: 0,
    failed: 0,
    warnings: 0
  },
  fixtures: {},
  routeResults: [],
  cleanup: [],
  consoleErrors: [],
  pageErrors: [],
  screenshots: {}
};

function normalizeText(value) {
  return String(value || "").replace(/\s+/g, " ").trim();
}

function safeSlug(value) {
  return normalizeText(value).replace(/[^\w\u4e00-\u9fa5.-]+/g, "_").slice(0, 80) || "item";
}

function isHighRisk(label) {
  return /删除|清理|退出|确定|保存|执行|生成|入库|同步|刷新库|忽略|取消任务|恢复任务|重命名|整理|扫码|登录|重新获取|全量|增量|测试连接|重新探测/.test(label);
}

function shouldSkipClick(candidate) {
  const label = normalizeText(candidate.label || candidate.href || candidate.className);
  const className = normalizeText(candidate.className);
  if (/sidebar-link|source-item|source-card|category-card|recent-task-item/.test(className)) return "导航/选择卡片已在路由与列表验证中覆盖";
  if (/全量同步|增量同步|执行入库|入库|STRM|刷新库|全量生成|立即运行|恢复任务|取消任务|重新探测|测试连接/.test(label)) {
    return "长耗时或会产生后端任务的动作，记录可见但不直接执行";
  }
  if (/保存并入库|执行整理|执行重命名|确定/.test(label)) {
    return "提交型动作只验证弹层打开，不确认提交";
  }
  return "";
}

function actionSignature(candidate) {
  const rawLabel = normalizeText(candidate.label || candidate.href || candidate.className);
  const label = rawLabel
    .replace(/codex_[\w-]+/g, "codex_fixture")
    .replace(/\d{4,}/g, "N")
    .replace(/[A-Z]:\\[^ ]+/g, "<path>");
  const className = normalizeText(candidate.className)
    .replace(/\bis-[\w-]+\b/g, "")
    .replace(/\s+/g, " ")
    .trim();
  return `${candidate.tag}|${candidate.href || ""}|${className}|${label}`;
}

function dedupeCandidates(candidates) {
  const seen = new Set();
  const result = [];
  const duplicates = [];
  for (const candidate of candidates) {
    const signature = actionSignature(candidate);
    if (seen.has(signature)) {
      duplicates.push({
        route: "",
        mode: "page",
        label: candidate.label,
        tag: candidate.tag,
        href: candidate.href,
        className: candidate.className,
        status: "skipped",
        reason: "同页面同类按钮已覆盖"
      });
      continue;
    }
    seen.add(signature);
    result.push(candidate);
  }
  return { result, duplicates };
}

async function responseJson(response) {
  try {
    return await response.json();
  } catch {
    return {};
  }
}

async function loginApi() {
  const api = await request.newContext({ baseURL: BACKEND_API });
  const response = await api.post("/login", {
    data: { name: USERNAME, password: PASSWORD_MD5 }
  });
  const data = await responseJson(response);
  await api.dispose();
  if (!response.ok() || !data.token) {
    throw new Error(`登录失败: ${JSON.stringify(data)}`);
  }
  return { token: data.token, userId: String(data.user_id || "") };
}

async function createAuthedApi(token) {
  return request.newContext({
    baseURL: BACKEND_API,
    extraHTTPHeaders: { Authorization: `Bearer ${token}` }
  });
}

async function createFixtures(api) {
  const fixtureRoot = path.join(DEBUG_DIR, `fixture-${Date.now()}`);
  const sourceDir = path.join(fixtureRoot, "source");
  const targetDir = path.join(fixtureRoot, "target");
  await fs.mkdir(sourceDir, { recursive: true });
  await fs.mkdir(targetDir, { recursive: true });
  await fs.writeFile(path.join(sourceDir, "Codex.Exhaustive.Button.2026.1080p.mkv"), "fixture\n", "utf8");
  await fs.writeFile(path.join(sourceDir, "Codex.Exhaustive.Pending.2026.mkv"), "pending fixture\n", "utf8");

  const sourceName = `codex_exhaustive_source_${Date.now()}`;
  const sourceResponse = await api.post("/media/sources", {
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
  const sourceData = await responseJson(sourceResponse);
  const sourceId = sourceData?.data?.id || sourceData?.data?.data?.id;
  if (!sourceResponse.ok() || !sourceId) {
    throw new Error(`创建临时媒体源失败: ${JSON.stringify(sourceData)}`);
  }

  let pendingId = null;
  const pendingResponse = await api.post("/media/pending", {
    data: {
      source_kind: "local",
      source_id: sourceId,
      source_path: path.join(sourceDir, "Codex.Exhaustive.Pending.2026.mkv"),
      title: "Codex Exhaustive Pending",
      year: 2026,
      media_type: "movie",
      reason: "全量按钮穷举临时项"
    }
  });
  if (pendingResponse.ok()) {
    const pendingData = await responseJson(pendingResponse);
    pendingId = pendingData?.data?.id || pendingData?.data?.data?.id;
  }

  let categoryId = null;
  const categoryName = `按钮穷举${Date.now()}`;
  const categoryResponse = await api.post("/media/categories", {
    data: {
      name: categoryName,
      media_type: "movie",
      target_path: `/电影/${categoryName}`,
      match_rules: { keywords: [categoryName] },
      enabled: true
    }
  });
  if (categoryResponse.ok()) {
    const categoryData = await responseJson(categoryResponse);
    categoryId = categoryData?.data?.id || categoryData?.data?.data?.id;
  }

  const cloudResponse = await api.get("/cloud115");
  const cloudData = await responseJson(cloudResponse);
  const cloudRows = cloudData?.data?.data || cloudData?.data || [];
  const firstCloud = Array.isArray(cloudRows) ? cloudRows[0] : null;

  let strmConfigId = null;
  if (firstCloud) {
    const strmResponse = await api.post("/strm/config", {
      data: {
        cloud115_id: firstCloud.id,
        net_disk_path: `/codex-exhaustive-${Date.now()}`,
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
    if (strmResponse.ok()) {
      const strmData = await responseJson(strmResponse);
      strmConfigId = strmData?.data?.id || strmData?.data?.data?.id;
    }
  }

  return {
    fixtureRoot,
    sourceDir,
    targetDir,
    sourceId,
    sourceName,
    pendingId,
    categoryId,
    categoryName,
    firstCloudId: firstCloud?.id || null,
    firstCloudName: firstCloud?.name || null,
    strmConfigId
  };
}

async function cleanupFixtures(api, fixtures) {
  if (fixtures.strmConfigId) {
    const response = await api.delete(`/strm/config/${fixtures.strmConfigId}`).catch(() => null);
    report.cleanup.push({ item: "strm_config", id: fixtures.strmConfigId, ok: Boolean(response?.ok?.()) });
  }
  if (fixtures.categoryId) {
    const response = await api.delete(`/media/categories/${fixtures.categoryId}`).catch(() => null);
    report.cleanup.push({ item: "category", id: fixtures.categoryId, ok: Boolean(response?.ok?.()) });
  }
  if (fixtures.sourceId) {
    const response = await api.delete(`/media/sources/${fixtures.sourceId}`).catch(() => null);
    report.cleanup.push({ item: "media_source", id: fixtures.sourceId, ok: Boolean(response?.ok?.()) });
  }
}

async function gotoRoute(page, route) {
  await page.goto(`${FRONTEND_URL}${route}`, { waitUntil: "domcontentloaded", timeout: 30000 });
  await page.locator(".shell-header .header-title").waitFor({ timeout: 15000 });
}

async function loginUi(page, token, userId) {
  await page.goto(`${FRONTEND_URL}/login`, { waitUntil: "domcontentloaded", timeout: 30000 });
  await page.locator('input[type="text"]').first().fill(USERNAME);
  await page.locator('input[type="password"]').first().fill(PASSWORD);
  await page.locator(".login-btn").click();
  await page.waitForURL("**/dashboard/**", { timeout: 15000 });
  await page.evaluate(({ token, userId }) => {
    localStorage.setItem("token", token);
    localStorage.setItem("user_id", userId);
  }, { token, userId });
}

async function dismissOverlays(page) {
  for (let i = 0; i < 5; i += 1) {
    const messageBox = page.locator(".el-message-box:visible");
    if (await messageBox.count()) {
      const cancel = messageBox.getByRole("button", { name: "取消" });
      const close = messageBox.locator(".el-message-box__headerbtn");
      if (await cancel.count()) {
        await cancel.first().click().catch(() => {});
      } else if (await close.count()) {
        await close.first().click().catch(() => {});
      } else {
        await page.keyboard.press("Escape").catch(() => {});
      }
      await page.waitForTimeout(250);
      continue;
    }
    const dialogs = page.locator(".el-dialog:visible");
    if (await dialogs.count()) {
      const close = dialogs.first().locator(".el-dialog__headerbtn");
      const cancel = dialogs.first().getByRole("button", { name: "取消" });
      const closeText = dialogs.first().getByRole("button", { name: "关闭" });
      if (await close.count()) {
        await close.first().click().catch(() => {});
      } else if (await cancel.count()) {
        await cancel.first().click().catch(() => {});
      } else if (await closeText.count()) {
        await closeText.first().click().catch(() => {});
      } else {
        await page.keyboard.press("Escape").catch(() => {});
      }
      await page.waitForTimeout(250);
      continue;
    }
    const drawers = page.locator(".el-drawer:visible");
    if (await drawers.count()) {
      const close = drawers.first().locator(".el-drawer__close-btn");
      if (await close.count()) {
        await close.first().click().catch(() => {});
      } else {
        await page.keyboard.press("Escape").catch(() => {});
      }
      await page.waitForTimeout(250);
      continue;
    }
    const dropdowns = page.locator(".el-popper:visible");
    if (await dropdowns.count()) {
      await page.keyboard.press("Escape").catch(() => {});
      await page.waitForTimeout(250);
      continue;
    }
    break;
  }
}

async function enumerateClickables(page, scope = "page") {
  return page.evaluate(({ scope }) => {
    const root = scope === "overlay"
      ? document.querySelector(".el-dialog:where(:not([style*='display: none'])), .el-drawer, .el-message-box, .el-popper")
      : document.querySelector(".shell-layout") || document.body;
    if (!root) return [];
    const selector = [
      "button",
      "a[href]",
      "[role='button']",
      ".el-switch",
      ".el-dropdown",
      ".el-dropdown-menu__item",
      ".el-select",
      ".el-checkbox",
      ".el-radio",
      ".breadcrumb-link",
      ".command-button",
      ".surface-link",
      ".hero-action",
      ".header-chip",
      ".nav-item"
    ].join(",");
    const seen = new Set();
    return Array.from(root.querySelectorAll(selector)).map((element, index) => {
      const rect = element.getBoundingClientRect();
      const style = window.getComputedStyle(element);
      const disabled = element.disabled || element.getAttribute("aria-disabled") === "true" || element.classList.contains("is-disabled");
      const visible = rect.width > 0 && rect.height > 0 && style.visibility !== "hidden" && style.display !== "none";
      const label = (element.innerText || element.getAttribute("aria-label") || element.getAttribute("title") || element.getAttribute("href") || element.className || element.tagName || "").replace(/\s+/g, " ").trim();
      const signature = `${element.tagName.toLowerCase()}|${label}|${element.getAttribute("href") || ""}|${index}`;
      if (!visible || disabled || seen.has(signature)) return null;
      seen.add(signature);
      element.setAttribute("data-codex-click-id", `${scope}-${index}`);
      return {
        id: `${scope}-${index}`,
        tag: element.tagName.toLowerCase(),
        role: element.getAttribute("role") || "",
        label,
        href: element.getAttribute("href") || "",
        className: String(element.className || ""),
        x: Math.round(rect.left + rect.width / 2),
        y: Math.round(rect.top + rect.height / 2)
      };
    }).filter(Boolean);
  }, { scope });
}

async function observeAfterClick(page, beforeUrl) {
  await page.waitForTimeout(700);
  const url = page.url();
  const messageTexts = await page.locator(".el-message:visible").allTextContents().catch(() => []);
  const notificationTexts = await page.locator(".el-notification:visible").allTextContents().catch(() => []);
  const messageBoxCount = await page.locator(".el-message-box:visible").count().catch(() => 0);
  const dialogCount = await page.locator(".el-dialog:visible").count().catch(() => 0);
  const drawerCount = await page.locator(".el-drawer:visible").count().catch(() => 0);
  const dropdownCount = await page.locator(".el-popper:visible").count().catch(() => 0);
  return {
    urlChanged: url !== beforeUrl,
    url,
    messages: [...messageTexts, ...notificationTexts].map(normalizeText).filter(Boolean),
    messageBoxCount,
    dialogCount,
    drawerCount,
    dropdownCount
  };
}

async function clickCandidate(page, route, candidate, mode) {
  const record = {
    route,
    mode,
    label: candidate.label,
    tag: candidate.tag,
    href: candidate.href,
    className: candidate.className,
    status: "unknown",
    observation: null
  };
  const label = normalizeText(candidate.label || candidate.href || candidate.className);
  const skipReason = shouldSkipClick(candidate);
  if (skipReason) {
    record.status = "skipped";
    record.reason = skipReason;
    return record;
  }
  if (!label && !candidate.className) {
    record.status = "skipped";
    record.reason = "缺少可识别标签";
    return record;
  }

  const locator = page.locator(`[data-codex-click-id="${candidate.id}"]`);
  if ((await locator.count()) !== 1) {
    record.status = "skipped";
    record.reason = "点击前元素已消失或不唯一";
    return record;
  }

  const beforeUrl = page.url();
  try {
    await locator.scrollIntoViewIfNeeded();
    await locator.click({ timeout: 2500 });
    record.observation = await observeAfterClick(page, beforeUrl);

    const openedConfirmation = record.observation.messageBoxCount > 0;
    const openedOverlay = record.observation.dialogCount > 0 || record.observation.drawerCount > 0 || record.observation.dropdownCount > 0;
    const hadMessage = record.observation.messages.length > 0;
    const navigated = record.observation.urlChanged;

    if (isHighRisk(label) && openedConfirmation) {
      record.status = "guarded";
      record.reason = "高风险按钮已触达确认框，未确认执行";
    } else if (openedOverlay) {
      record.status = "clicked";
      record.reason = "打开弹层或菜单";
    } else if (hadMessage) {
      record.status = "clicked";
      record.reason = "出现消息反馈";
    } else if (navigated) {
      record.status = "clicked";
      record.reason = "发生路由跳转";
    } else {
      record.status = "clicked";
      record.reason = "点击完成，无明显 UI 变化";
    }
  } catch (error) {
    record.status = "failed";
    record.error = error.message;
  }
  return record;
}

async function testCandidate(page, route, candidate, mode) {
  await gotoRoute(page, route);
  await dismissOverlays(page);

  if (route === "/dashboard/sync-tasks" || route === "/dashboard/media-library" || route === "/dashboard/pending-media") {
    const url = new URL(`${FRONTEND_URL}${route}`);
    url.searchParams.set("source_id", String(report.fixtures.sourceId));
    await page.goto(url.toString(), { waitUntil: "domcontentloaded", timeout: 30000 });
  }

  const candidates = await enumerateClickables(page, mode);
  const fresh = candidates.find((item) => item.id === candidate.id && item.label === candidate.label)
    || candidates.find((item) => item.label === candidate.label && item.tag === candidate.tag && item.href === candidate.href);
  if (!fresh) {
    return {
      route,
      mode,
      label: candidate.label,
      tag: candidate.tag,
      href: candidate.href,
      status: "skipped",
      reason: "重置页面后元素不可见"
    };
  }
  const record = await Promise.race([
    clickCandidate(page, route, fresh, mode),
    new Promise((resolve) => setTimeout(() => resolve({
      route,
      mode,
      label: fresh.label,
      tag: fresh.tag,
      href: fresh.href,
      className: fresh.className,
      status: "failed",
      error: "单按钮点击超过 8 秒"
    }), 8000))
  ]);
  await dismissOverlays(page);
  return record;
}

async function testOverlayCandidates(page, route, opener) {
  await gotoRoute(page, route);
  await dismissOverlays(page);
  const pageCandidates = await enumerateClickables(page, "page");
  const freshOpener = pageCandidates.find((item) => item.label === opener.label && item.tag === opener.tag && item.href === opener.href);
  if (!freshOpener) return [];
  const openerLocator = page.locator(`[data-codex-click-id="${freshOpener.id}"]`);
  if ((await openerLocator.count()) !== 1) return [];
  await openerLocator.scrollIntoViewIfNeeded();
  await openerLocator.click({ timeout: 5000 }).catch(() => {});
  await page.waitForTimeout(600);
  const overlayCandidates = await enumerateClickables(page, "overlay");
  const records = [];
  for (const candidate of overlayCandidates.slice(0, 20)) {
    if (isHighRisk(candidate.label) && !/取消|关闭|重新获取|搜索|生成|确定|保存/.test(candidate.label)) {
      records.push({
        route,
        mode: "overlay",
        label: candidate.label,
        tag: candidate.tag,
        href: candidate.href,
        status: "skipped",
        reason: "弹层内高风险动作不执行"
      });
      continue;
    }
    const record = await clickCandidate(page, route, candidate, "overlay");
    records.push(record);
    if (record.observation?.urlChanged || record.observation?.messageBoxCount || record.observation?.dialogCount === 0) {
      break;
    }
  }
  await dismissOverlays(page);
  return records;
}

async function main() {
  await fs.mkdir(DEBUG_DIR, { recursive: true });
  const { token, userId } = await loginApi();
  const api = await createAuthedApi(token);
  const fixtures = await createFixtures(api);
  report.fixtures = fixtures;

  let browser;
  try {
    browser = await chromium.launch({ headless: true, args: ["--no-proxy-server"] });
    const context = await browser.newContext({ viewport: { width: 1520, height: 980 } });
    const page = await context.newPage();
    page.on("console", (message) => {
      if (message.type() === "error") report.consoleErrors.push(message.text());
    });
    page.on("pageerror", (error) => report.pageErrors.push(error.message));

    await loginUi(page, token, userId);
    report.screenshots.login = path.join(DEBUG_DIR, "login.png");
    await page.screenshot({ path: report.screenshots.login, fullPage: true });

    for (const [route, expectedTitle] of routes) {
      const routeResult = {
        route,
        expectedTitle,
        title: "",
        initialCount: 0,
        records: [],
        screenshot: ""
      };
      await gotoRoute(page, route);
      if (route === "/dashboard/sync-tasks" || route === "/dashboard/media-library" || route === "/dashboard/pending-media") {
        await page.goto(`${FRONTEND_URL}${route}?source_id=${fixtures.sourceId}`, { waitUntil: "domcontentloaded", timeout: 30000 });
      }
      await page.waitForTimeout(800);
      routeResult.title = normalizeText(await page.locator(".shell-header .header-title").innerText());
      const candidates = await enumerateClickables(page, "page");
      routeResult.initialCount = candidates.length;
      report.summary.enumerated += candidates.length;

      routeResult.screenshot = path.join(DEBUG_DIR, `${safeSlug(route)}.png`);
      await page.screenshot({ path: routeResult.screenshot, fullPage: true });
      report.screenshots[route] = routeResult.screenshot;

      const { result: uniqueCandidates, duplicates } = dedupeCandidates(candidates);
      for (const duplicate of duplicates) {
        duplicate.route = route;
        routeResult.records.push(duplicate);
        report.summary.skipped += 1;
      }

      for (const candidate of uniqueCandidates.slice(0, 80)) {
        const record = await testCandidate(page, route, candidate, "page");
        routeResult.records.push(record);
        if (record.status === "failed") report.summary.failed += 1;
        else if (record.status === "skipped") report.summary.skipped += 1;
        else report.summary.clicked += 1;

        const opensOverlay = record.observation && (record.observation.dialogCount > 0 || record.observation.drawerCount > 0 || record.observation.dropdownCount > 0);
        if (opensOverlay) {
          const overlayRecords = await testOverlayCandidates(page, route, candidate).catch((error) => [{
            route,
            mode: "overlay",
            label: candidate.label,
            status: "failed",
            error: error.message
          }]);
          for (const overlayRecord of overlayRecords) {
            routeResult.records.push(overlayRecord);
            report.summary.enumerated += 1;
            if (overlayRecord.status === "failed") report.summary.failed += 1;
            else if (overlayRecord.status === "skipped") report.summary.skipped += 1;
            else report.summary.clicked += 1;
          }
        }
      }
      report.routeResults.push(routeResult);
      await fs.writeFile(REPORT_PATH, JSON.stringify(report, null, 2), "utf8");
    }
  } finally {
    await cleanupFixtures(api, fixtures).catch((error) => {
      report.cleanup.push({ item: "cleanup_error", ok: false, error: error.message });
    });
    await api.dispose().catch(() => {});
    if (browser) await browser.close();
  }

  const actionableConsoleErrors = report.consoleErrors.filter((item) => !/403|favicon|net::ERR_FAILED/.test(item));
  report.summary.warnings = report.routeResults.reduce((count, route) => count + route.records.filter((record) => record.status === "guarded").length, 0);
  report.ok = report.summary.failed === 0 && report.pageErrors.length === 0 && actionableConsoleErrors.length === 0;
  report.finishedAt = new Date().toISOString();
  await fs.writeFile(REPORT_PATH, JSON.stringify(report, null, 2), "utf8");
  console.log(JSON.stringify({
    ok: report.ok,
    summary: report.summary,
    pageErrors: report.pageErrors,
    consoleErrors: report.consoleErrors.length,
    reportPath: REPORT_PATH
  }, null, 2));
  if (!report.ok) process.exit(1);
}

main().catch(async (error) => {
  report.ok = false;
  report.fatal = error.message;
  await fs.mkdir(DEBUG_DIR, { recursive: true }).catch(() => {});
  await fs.writeFile(REPORT_PATH, JSON.stringify(report, null, 2), "utf8").catch(() => {});
  console.error(JSON.stringify({ ok: false, fatal: error.message, reportPath: REPORT_PATH }, null, 2));
  process.exit(1);
});
