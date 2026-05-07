import fs from "node:fs/promises";
import path from "node:path";
import { chromium, request } from "playwright";

const FRONTEND_URL = process.env.E2E_FRONTEND_URL || "http://localhost:3001";
const BACKEND_API = process.env.E2E_BACKEND_API || "http://localhost:8082";
const ROOT_DIR = path.resolve(process.cwd(), "..");
const DEBUG_DIR = path.join(ROOT_DIR, "debug");
const SCREENSHOT_DIR = path.join(DEBUG_DIR, "e2e-screenshots", "organize-retry-failed-items");
const REPORT_DIR = path.join(ROOT_DIR, "docs");

const now = new Date();
const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, "0")}${String(now.getDate()).padStart(2, "0")}_${String(now.getHours()).padStart(2, "0")}${String(now.getMinutes()).padStart(2, "0")}${String(now.getSeconds()).padStart(2, "0")}`;
const runId = `organize_retry_failed_items_${stamp}`;
const runScreenshotDir = path.join(SCREENSHOT_DIR, runId);
const reportFile = path.join(REPORT_DIR, `整理失败重试测试报告_${stamp}.md`);
const fixtureRoot = path.join(DEBUG_DIR, "e2e-fixtures", runId);
const sourceDir = path.join(fixtureRoot, "source");
const targetDir = path.join(fixtureRoot, "target");

const cases = [];

function addCase(id, name, status, detail = "", artifact = "") {
  cases.push({ id, name, status, detail, artifact });
  const prefix = status === "PASS" ? "[PASS]" : status === "FAIL" ? "[FAIL]" : "[SKIP]";
  console.log(`${prefix} ${id} ${name}${detail ? ` -> ${detail}` : ""}`);
}

async function ensureDirs() {
  await fs.mkdir(runScreenshotDir, { recursive: true });
  await fs.mkdir(REPORT_DIR, { recursive: true });
  await fs.mkdir(sourceDir, { recursive: true });
  await fs.mkdir(targetDir, { recursive: true });
  await fs.writeFile(path.join(sourceDir, "Codex.Retry.Success.Part1.mkv"), "retry file 1\n", "utf8");
  await fs.writeFile(path.join(sourceDir, "Codex.Retry.Fail.Part2.mkv"), "retry file 2\n", "utf8");
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

function unwrapData(payload) {
  if (!payload || typeof payload !== "object") return payload;
  if ("data" in payload) return unwrapData(payload.data);
  return payload;
}

async function waitForMessage(page, text) {
  const locator = page.locator(".el-message").filter({ hasText: text }).last();
  await locator.waitFor({ timeout: 20000 });
}

async function findDialogByTitle(page, title) {
  const dialog = page.locator(".el-dialog").filter({ hasText: title }).last();
  await dialog.waitFor({ timeout: 15000 });
  return dialog;
}

function labeledField(dialog, label) {
  return dialog.locator(".el-form-item").filter({ hasText: label });
}

async function setInputValue(locator, value) {
  await locator.click();
  await locator.press(process.platform === "darwin" ? "Meta+A" : "Control+A");
  await locator.fill(String(value));
  await locator.press("Tab");
}

async function applyManualIdentify(page, organizeDialog, fileName, title, season, episode) {
  const row = organizeDialog.locator(".el-table__row").filter({ hasText: fileName }).first();
  await row.locator(".el-button").filter({ hasText: "手动识别" }).click();
  const manualDialog = await findDialogByTitle(page, "手动识别修正");
  await manualDialog.locator(".el-select").first().click();
  await page.locator(".el-select-dropdown__item").filter({ hasText: "剧集" }).last().click();
  await setInputValue(labeledField(manualDialog, "标题").locator("input").first(), title);
  await setInputValue(labeledField(manualDialog, "年份").locator(".el-input-number input").first(), 2024);
  await setInputValue(labeledField(manualDialog, "季数").locator(".el-input-number input").first(), season);
  await setInputValue(labeledField(manualDialog, "集数").locator(".el-input-number input").first(), episode);
  await manualDialog.locator(".dialog-footer .el-button--primary").filter({ hasText: "应用到预览" }).click();
  await manualDialog.waitFor({ state: "hidden", timeout: 25000 });
  await waitForMessage(page, "预览完成");
}

async function listFilesRecursive(rootDir) {
  const results = [];
  async function walk(currentDir) {
    const entries = await fs.readdir(currentDir, { withFileTypes: true });
    for (const entry of entries) {
      const fullPath = path.join(currentDir, entry.name);
      if (entry.isDirectory()) {
        await walk(fullPath);
      } else {
        results.push(path.relative(rootDir, fullPath).replace(/\\/g, "/"));
      }
    }
  }
  await walk(rootDir);
  return results.sort();
}

async function run() {
  let browser;
  let api;
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
    await page.waitForURL(/\/dashboard(\/|$)/, { timeout: 30000, waitUntil: "commit" });
    addCase("TC-ORG-RETRY-AUTH-001", "登录成功", "PASS", "", await saveShot(page, "01_login"));

    const token = await page.evaluate(() => localStorage.getItem("token"));
    if (!token) throw new Error("登录后未获取 token");
    api = await request.newContext({
      baseURL: BACKEND_API,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` }
    });

    const mediaSourceName = `organize_retry_${stamp}`;
    const createSourceResp = await api.post("/media/sources", {
      data: {
        name: mediaSourceName,
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
    const createSourceData = unwrapData(await apiJson(createSourceResp)) || {};
    mediaSourceId = createSourceData.id || null;
    if (!createSourceResp.ok() || !mediaSourceId) {
      throw new Error(`创建测试媒体源失败: ${JSON.stringify(createSourceData)}`);
    }

    await page.goto(`${FRONTEND_URL}/dashboard/media-manager`, { waitUntil: "networkidle" });
    const sourceRow = page.locator(".el-table__row").filter({ hasText: mediaSourceName }).first();
    await sourceRow.waitFor({ timeout: 15000 });
    await sourceRow.locator(".el-button--primary").first().click();

    const browserDialog = page.locator(".el-dialog").filter({ has: page.locator(".table-wrapper") }).last();
    await browserDialog.waitFor({ timeout: 15000 });
    for (const fileName of ["Codex.Retry.Success.Part1.mkv", "Codex.Retry.Fail.Part2.mkv"]) {
      const fileRow = browserDialog.locator(".el-table__row").filter({ hasText: fileName }).first();
      await fileRow.waitFor({ timeout: 15000 });
      await fileRow.locator(".el-checkbox").click();
    }
    await browserDialog.locator(".el-button").filter({ hasText: "批量整理" }).click();

    const organizeDialog = await findDialogByTitle(page, "批量整理");
    await organizeDialog.locator(".dialog-footer .el-button").filter({ hasText: "刷新预览" }).click();
    await waitForMessage(page, "预览完成");

    await applyManualIdentify(page, organizeDialog, "Codex.Retry.Success.Part1.mkv", "Codex Retry Show", 1, 1);
    await applyManualIdentify(page, organizeDialog, "Codex.Retry.Fail.Part2.mkv", "Codex Retry Show", 1, 2);

    const previewText = (await organizeDialog.textContent()) || "";
    const previewPass = previewText.includes("Codex Retry Show - S01E01") && previewText.includes("Codex Retry Show - S01E02");
    addCase(
      "TC-ORG-RETRY-001",
      "两条手动识别结果均成功回流预览",
      previewPass ? "PASS" : "FAIL",
      previewText.replace(/\s+/g, " ").slice(0, 220),
      await saveShot(page, "02_preview_ready")
    );

    await fs.unlink(path.join(sourceDir, "Codex.Retry.Fail.Part2.mkv"));
    await organizeDialog.locator(".dialog-footer .el-button--primary").filter({ hasText: "执行整理" }).click();
    await waitForMessage(page, "整理完成");

    const resultDialog = await findDialogByTitle(page, "整理结果");
    const firstResultText = (await resultDialog.textContent()) || "";
    const partialFailPass = firstResultText.includes("成功") && firstResultText.includes("失败") && firstResultText.includes("重试失败项 (1)");
    addCase(
      "TC-ORG-RETRY-002",
      "首次执行后结果弹窗展示部分成功且可重试失败项",
      partialFailPass ? "PASS" : "FAIL",
      firstResultText.replace(/\s+/g, " ").slice(0, 260),
      await saveShot(page, "03_partial_failure")
    );

    await fs.writeFile(path.join(sourceDir, "Codex.Retry.Fail.Part2.mkv"), "retry restored file 2\n", "utf8");
    await resultDialog.locator(".el-button").filter({ hasText: "重试失败项" }).click();
    await waitForMessage(page, "重试完成");
    await page.waitForTimeout(1200);

    const secondResultText = (await resultDialog.textContent()) || "";
    const retryPass = !secondResultText.includes("重试失败项 (") && secondResultText.includes("成功") && !secondResultText.includes("失败 1");
    const targetFiles = await listFilesRecursive(targetDir);
    const filePass = targetFiles.some((item) => item.endsWith("Codex Retry Show - S01E01.mkv")) && targetFiles.some((item) => item.endsWith("Codex Retry Show - S01E02.mkv"));
    addCase(
      "TC-ORG-RETRY-003",
      "重试失败项后结果汇总归零失败且缺失文件补齐成功",
      retryPass && filePass ? "PASS" : "FAIL",
      `dialog=${secondResultText.replace(/\s+/g, " ").slice(0, 260)} target=${targetFiles.join(",")}`,
      await saveShot(page, "04_retry_success")
    );

    const summary = {
      runId,
      total: cases.length,
      passCount: cases.filter((item) => item.status === "PASS").length,
      failCount: cases.filter((item) => item.status === "FAIL").length,
      skipCount: cases.filter((item) => item.status === "SKIP").length,
      screenshots: runScreenshotDir,
      report: reportFile
    };

    const lines = [
      "# 整理失败重试测试报告",
      "",
      `- 执行时间: ${new Date().toLocaleString("zh-CN", { hour12: false })}`,
      `- 执行者: Codex`,
      `- 运行标识: ${runId}`,
      "",
      "| 用例ID | 名称 | 结果 | 说明 | 证据 |",
      "| --- | --- | --- | --- | --- |",
      ...cases.map((item) => `| ${item.id} | ${item.name} | ${item.status} | ${item.detail || "-"} | ${item.artifact || "-"} |`),
      "",
      `- 通过: ${summary.passCount}`,
      `- 失败: ${summary.failCount}`,
      `- 跳过: ${summary.skipCount}`,
      `- 截图目录: ${summary.screenshots}`
    ];
    await fs.writeFile(reportFile, `${lines.join("\n")}\n`, "utf8");
    console.log(JSON.stringify(summary, null, 2));
  } catch (error) {
    addCase("TC-ORG-RETRY-RUN-000", "整理失败重试执行异常", "FAIL", `${error.name}: ${error.message}`);
    const lines = [
      "# 整理失败重试测试报告",
      "",
      `- 执行异常: ${error.stack || error.message}`,
      "",
      "| 用例ID | 名称 | 结果 | 说明 | 证据 |",
      "| --- | --- | --- | --- | --- |",
      ...cases.map((item) => `| ${item.id} | ${item.name} | ${item.status} | ${item.detail || "-"} | ${item.artifact || "-"} |`)
    ];
    await fs.mkdir(REPORT_DIR, { recursive: true });
    await fs.writeFile(reportFile, `${lines.join("\n")}\n`, "utf8");
    console.error(JSON.stringify({ report: reportFile, screenshots: runScreenshotDir }, null, 2));
    process.exitCode = 1;
  } finally {
    if (api) {
      if (mediaSourceId) {
        try {
          await api.delete(`/media/sources/${mediaSourceId}`);
        } catch {}
      }
      await api.dispose();
    }
    if (browser) {
      await browser.close();
    }
  }
}

run();
