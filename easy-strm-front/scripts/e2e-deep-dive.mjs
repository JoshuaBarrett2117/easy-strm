import fs from "node:fs/promises";
import path from "node:path";
import { chromium, request } from "playwright";

const FRONTEND_URL = process.env.E2E_FRONTEND_URL || "http://localhost:3001";
const BACKEND_API = process.env.E2E_BACKEND_API || "http://localhost:8082";
const ROOT_DIR = path.resolve(process.cwd(), "..");
const DEBUG_DIR = path.join(ROOT_DIR, "debug");
const SCREENSHOT_DIR = path.join(DEBUG_DIR, "e2e-screenshots", "deep-dive");
const REPORT_DIR = path.join(ROOT_DIR, "docs");

const now = new Date();
const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, "0")}${String(now.getDate()).padStart(2, "0")}_${String(now.getHours()).padStart(2, "0")}${String(now.getMinutes()).padStart(2, "0")}${String(now.getSeconds()).padStart(2, "0")}`;
const runId = `deep_dive_${stamp}`;
const runScreenshotDir = path.join(SCREENSHOT_DIR, runId);
const reportFile = path.join(REPORT_DIR, `深挖测试报告_${stamp}.md`);
const fixtureRoot = path.join(DEBUG_DIR, "e2e-fixtures", runId);
const renameSource = path.join(fixtureRoot, "rename-source");

const cases = [];

function addCase(id, name, status, detail = "", artifact = "") {
  cases.push({ id, name, status, detail, artifact });
  const prefix = status === "PASS" ? "[PASS]" : status === "FAIL" ? "[FAIL]" : "[SKIP]";
  console.log(`${prefix} ${id} ${name}${detail ? ` -> ${detail}` : ""}`);
}

async function ensureDirs() {
  await fs.mkdir(runScreenshotDir, { recursive: true });
  await fs.mkdir(REPORT_DIR, { recursive: true });
  await fs.mkdir(renameSource, { recursive: true });
  await fs.writeFile(path.join(renameSource, "Rename.Target.2024.1080p.mkv"), "rename target\n", "utf8");
  await fs.writeFile(path.join(renameSource, "Rename.Target.2024.1080p.copy.mkv"), "rename target copy\n", "utf8");
}

async function saveShot(page, name) {
  const file = path.join(runScreenshotDir, `${name}.png`);
  await page.screenshot({ path: file, fullPage: true });
  return file;
}

async function apiJson(resp) {
  try {
    return await resp.json();
  } catch {
    return null;
  }
}

function normalizeData(payload) {
  if (!payload || typeof payload !== "object") return payload;
  if ("data" in payload) {
    return normalizeData(payload.data);
  }
  return payload;
}

async function confirmPrimaryAction(page) {
  const primary = page.locator(".el-message-box__btns .el-button--primary").last();
  await primary.waitFor({ timeout: 10000 });
  await primary.click();
}

async function clickVisibleDropdownItem(page, text) {
  const item = page.locator(".el-dropdown-menu__item:visible").filter({ hasText: text }).last();
  await item.waitFor({ timeout: 10000 });
  await item.click();
}

async function run() {
  let browser;
  let api;
  let cloudAccountId = null;
  let strmConfigId = null;
  let mediaSourceId = null;

  try {
    await ensureDirs();

    browser = await chromium.launch({ headless: true });
    const context = await browser.newContext({ viewport: { width: 1560, height: 980 } });
    const page = await context.newPage();

    await page.goto(`${FRONTEND_URL}/login`, { waitUntil: "networkidle" });
    await page.locator('input[placeholder*="用户名"], input[type="text"]').first().fill("admin");
    await page.locator('input[type="password"]').first().fill("admin");
    await page.locator(".login-btn").click();
    await page.waitForURL("**/dashboard/**", { timeout: 15000 });

    const token = await page.evaluate(() => localStorage.getItem("token"));
    if (!token) throw new Error("登录后未获取 token");
    addCase("TC-DEEP-AUTH-001", "登录成功", "PASS", "", await saveShot(page, "01_login"));

    api = await request.newContext({
      baseURL: BACKEND_API,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` }
    });

    const cloudName = `deep_cloud_${stamp}`;
    const createCloudResp = await api.post("/cloud115", {
      data: {
        name: cloudName,
        cookie: "UID=deep-test;",
        account_type: "resource",
        priority: 5,
        transfer_method: "115driver"
      }
    });
    const createCloudData = normalizeData(await apiJson(createCloudResp)) || {};
    cloudAccountId = createCloudData.id || null;
    if (!createCloudResp.ok() || !cloudAccountId) {
      throw new Error(`创建测试 115 账号失败: ${JSON.stringify(createCloudData)}`);
    }
    addCase("TC-DEEP-STRM-001", "创建测试 115 账号成功", "PASS", `id=${cloudAccountId}`);

    await page.goto(`${FRONTEND_URL}/dashboard/strm-config`, { waitUntil: "networkidle" });
    await page.locator(".header-title").filter({ hasText: "STRM 文件配置中心" }).waitFor({ timeout: 15000 });
    await page.locator(".card-header .el-button--primary").click();

    const dialog = page.locator(".el-dialog").filter({ hasText: "新增配置" }).last();
    await dialog.waitFor({ timeout: 10000 });
    await dialog.locator(".el-select").first().click();
    await page.locator(".el-select-dropdown__item").filter({ hasText: cloudName }).click();
    await dialog.locator('input[placeholder="请输入网盘目录路径"]').fill(`/deep/${stamp}`);
    await dialog.locator('input[placeholder="请输入本地目录路径"]').fill(`C:/deep/${stamp}`);
    await dialog.locator('input[placeholder*="Cron"]').fill("0 2 * * *");
    await dialog.locator('input[placeholder*="请输入文件后缀名"]').fill(".mkv,.mp4");
    await dialog.locator(".dialog-footer .el-button--primary").click();
    await page.waitForTimeout(1200);

    const configListResp = await api.get("/strm/config");
    const configListData = normalizeData(await apiJson(configListResp)) || [];
    const createdConfig = (Array.isArray(configListData) ? configListData : configListData.data || []).find((item) => item.cloud115_id === cloudAccountId && item.net_disk_path === `/deep/${stamp}`);
    if (!createdConfig) {
      throw new Error("新增 STRM 配置后未在接口中查询到");
    }
    strmConfigId = createdConfig.id;
    addCase("TC-DEEP-STRM-002", "STRM 配置新增成功", "PASS", `id=${strmConfigId}`, await saveShot(page, "02_strm_created"));

    const row = page.locator(".el-table__row").filter({ hasText: `/deep/${stamp}` }).first();
    await row.waitFor({ timeout: 10000 });
    await row.locator(".el-button--primary").first().click();
    const editDialog = page.locator(".el-dialog").filter({ hasText: "编辑配置" }).last();
    await editDialog.waitFor({ timeout: 10000 });
    await editDialog.locator('input[placeholder*="请输入文件后缀名"]').fill(".strm");
    await editDialog.locator('input[placeholder*="Cron"]').fill("15 3 * * *");
    await editDialog.locator(".dialog-footer .el-button--primary").click();
    await page.waitForTimeout(1200);

    const configDetailResp = await api.get(`/strm/config/${strmConfigId}`);
    const configDetail = normalizeData(await apiJson(configDetailResp)) || {};
    const updatePass = configDetail.extension === ".strm" && configDetail.cron === "15 3 * * *";
    addCase("TC-DEEP-STRM-003", "STRM 配置编辑成功", updatePass ? "PASS" : "FAIL", `ext=${configDetail.extension} cron=${configDetail.cron}`, await saveShot(page, "03_strm_updated"));

    const cronDropdown = row.locator(".el-button--info").first();
    await cronDropdown.click();
    await clickVisibleDropdownItem(page, "禁用定时任务");
    await page.waitForTimeout(1200);
    const cronTasksResp = await api.get("/cron/tasks");
    const cronTasks = normalizeData(await apiJson(cronTasksResp)) || [];
    const disabledTask = (cronTasks || []).find((item) => item.strm_config_id === strmConfigId);
    addCase(
      "TC-DEEP-STRM-004",
      "定时任务禁用成功",
      disabledTask?.status === "disabled" ? "PASS" : "FAIL",
      disabledTask ? `status=${disabledTask.status}` : "未找到关联 cron 任务",
      await saveShot(page, "04_cron_disabled")
    );

    await cronDropdown.click();
    await clickVisibleDropdownItem(page, "启用定时任务");
    await page.waitForTimeout(1200);
    const cronTasksResp2 = await api.get("/cron/tasks");
    const cronTasks2 = normalizeData(await apiJson(cronTasksResp2)) || [];
    const enabledTask = (cronTasks2 || []).find((item) => item.strm_config_id === strmConfigId);
    addCase(
      "TC-DEEP-STRM-005",
      "定时任务启用成功",
      enabledTask?.status === "enabled" ? "PASS" : "FAIL",
      enabledTask ? `status=${enabledTask.status}` : "未找到关联 cron 任务",
      await saveShot(page, "05_cron_enabled")
    );

    await cronDropdown.click();
    await clickVisibleDropdownItem(page, "查看详情");
    await page.locator(".el-message-box").waitFor({ timeout: 10000 });
    addCase("TC-DEEP-STRM-006", "定时任务详情弹窗可打开", "PASS", "", await saveShot(page, "06_cron_detail"));
    await page.locator(".el-message-box__btns .el-button--primary").click();

    await row.locator(".el-button--danger").first().click();
    await confirmPrimaryAction(page);
    await page.waitForTimeout(1200);
    const configListAfterDeleteResp = await api.get("/strm/config");
    const configListAfterDelete = normalizeData(await apiJson(configListAfterDeleteResp)) || [];
    const stillExists = (Array.isArray(configListAfterDelete) ? configListAfterDelete : configListAfterDelete.data || []).some((item) => item.id === strmConfigId);
    addCase("TC-DEEP-STRM-007", "STRM 配置删除成功", stillExists ? "FAIL" : "PASS", "", await saveShot(page, "07_strm_deleted"));
    if (!stillExists) {
      strmConfigId = null;
    }

    const mediaSourceName = `deep_local_${stamp}`;
    const createSourceResp = await api.post("/media/sources", {
      data: {
        name: mediaSourceName,
        source_type: "local",
        path: renameSource,
        organize_target_path: renameSource,
        auto_organize: false,
        watch_enabled: false,
        watch_interval: 300,
        emby_library_id: "",
        enabled: true,
        priority: 10
      }
    });
    const createSourceData = normalizeData(await apiJson(createSourceResp)) || {};
    mediaSourceId = createSourceData.id || null;
    if (!createSourceResp.ok() || !mediaSourceId) {
      throw new Error(`创建测试媒体源失败: ${JSON.stringify(createSourceData)}`);
    }

    await page.goto(`${FRONTEND_URL}/dashboard/media-manager`, { waitUntil: "networkidle" });
    const sourceRow = page.locator(".el-table__row").filter({ hasText: mediaSourceName }).first();
    await sourceRow.waitFor({ timeout: 10000 });
    await sourceRow.locator(".el-button--primary").first().click();
    const browserDialog = page.locator(".el-dialog").filter({ has: page.locator(".table-wrapper") }).last();
    await browserDialog.waitFor({ timeout: 10000 });
    await browserDialog.locator(".el-table__row").filter({ hasText: "Rename.Target.2024.1080p.mkv" }).first().locator(".el-checkbox").click();
    await browserDialog.locator(".el-table__row").filter({ hasText: "Rename.Target.2024.1080p.copy.mkv" }).first().locator(".el-checkbox").click();
    await browserDialog.locator(".el-button").filter({ hasText: "批量重命名" }).click();

    const renameDialog = page.locator(".el-dialog").filter({ hasText: "重命名预览" }).last();
    await renameDialog.waitFor({ timeout: 10000 });
    addCase("TC-DEEP-REN-001", "批量重命名预览弹窗可打开", "PASS", "", await saveShot(page, "08_rename_preview"));
    await renameDialog.locator(".dialog-footer .el-button--primary").click();
    await page.waitForTimeout(1500);

    const filesResp = await api.get(`/media/files?source_id=${mediaSourceId}&path=/&page=1&page_size=100`);
    const filesData = normalizeData(await apiJson(filesResp)) || {};
    const renamedFiles = filesData.files || [];
    const originalNames = new Set(["Rename.Target.2024.1080p.mkv", "Rename.Target.2024.1080p.copy.mkv"]);
    const renamedNames = renamedFiles.map((item) => item.name || "");
    const renamedPass = renamedNames.length === 2
      && renamedNames.every((name) => !originalNames.has(name))
      && renamedNames.every((name) => !name.includes(".."));
    addCase("TC-DEEP-REN-002", "批量重命名执行后文件名实际变更", renamedPass ? "PASS" : "FAIL", `files=${renamedNames.join(",")}`, await saveShot(page, "09_rename_done"));

    const summary = {
      runId,
      total: cases.length,
      passCount: cases.filter((item) => item.status === "PASS").length,
      failCount: cases.filter((item) => item.status === "FAIL").length,
      skipCount: cases.filter((item) => item.status === "SKIP").length,
      screenshots: runScreenshotDir,
      report: reportFile
    };

    const lines = [];
    lines.push("# 深挖测试报告");
    lines.push("");
    lines.push(`- 执行时间：${new Date().toLocaleString("zh-CN", { hour12: false })}`);
    lines.push(`- 执行者：Codex`);
    lines.push(`- 前端地址：${FRONTEND_URL}`);
    lines.push(`- 后端地址：${BACKEND_API}`);
    lines.push(`- 截图目录：${runScreenshotDir}`);
    lines.push("");
    lines.push("## 汇总");
    lines.push("");
    lines.push(`- 总用例：${summary.total}`);
    lines.push(`- 通过：${summary.passCount}`);
    lines.push(`- 失败：${summary.failCount}`);
    lines.push(`- 跳过：${summary.skipCount}`);
    lines.push("");
    lines.push("## 明细");
    lines.push("");
    lines.push("| 用例ID | 名称 | 结果 | 说明 | 证据 |");
    lines.push("| --- | --- | --- | --- | --- |");
    for (const item of cases) {
      lines.push(`| ${item.id} | ${item.name} | ${item.status} | ${item.detail || "-"} | ${item.artifact || "-"} |`);
    }
    await fs.writeFile(reportFile, `${lines.join("\n")}\n`, "utf8");

    console.log(JSON.stringify(summary, null, 2));
  } catch (error) {
    addCase("TC-DEEP-RUN-000", "深挖执行异常", "FAIL", String(error));
    const lines = [];
    lines.push("# 深挖测试报告");
    lines.push("");
    lines.push(`- 执行异常：${String(error)}`);
    lines.push("");
    lines.push("| 用例ID | 名称 | 结果 | 说明 | 证据 |");
    lines.push("| --- | --- | --- | --- | --- |");
    for (const item of cases) {
      lines.push(`| ${item.id} | ${item.name} | ${item.status} | ${item.detail || "-"} | ${item.artifact || "-"} |`);
    }
    await fs.writeFile(reportFile, `${lines.join("\n")}\n`, "utf8");
    console.log(JSON.stringify({ report: reportFile, screenshots: runScreenshotDir }, null, 2));
    process.exitCode = 1;
  } finally {
    if (api) {
      if (mediaSourceId) {
        await api.delete(`/media/sources/${mediaSourceId}`).catch(() => {});
      }
      if (strmConfigId) {
        await api.delete(`/strm/config/${strmConfigId}`).catch(() => {});
      }
      if (cloudAccountId) {
        await api.delete(`/cloud115/${cloudAccountId}`).catch(() => {});
      }
      await api.dispose().catch(() => {});
    }
    if (browser) {
      await browser.close();
    }
  }
}

run();
