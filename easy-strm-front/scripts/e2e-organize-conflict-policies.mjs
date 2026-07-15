import fs from "node:fs/promises";
import path from "node:path";
import { chromium, request } from "playwright";

const FRONTEND_URL = process.env.E2E_FRONTEND_URL || "http://localhost:3001";
const BACKEND_API = process.env.E2E_BACKEND_API || "http://localhost:8082";
const ROOT_DIR = path.resolve(process.cwd(), "..");
const DEBUG_DIR = path.join(ROOT_DIR, "debug");
const SCREENSHOT_DIR = path.join(DEBUG_DIR, "e2e-screenshots", "organize-conflict-policies");
const REPORT_DIR = path.join(ROOT_DIR, "docs");

const now = new Date();
const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, "0")}${String(now.getDate()).padStart(2, "0")}_${String(now.getHours()).padStart(2, "0")}${String(now.getMinutes()).padStart(2, "0")}${String(now.getSeconds()).padStart(2, "0")}`;
const runId = `organize_conflict_policies_${stamp}`;
const runScreenshotDir = path.join(SCREENSHOT_DIR, runId);
const reportFile = path.join(REPORT_DIR, `整理冲突策略对比测试报告_${stamp}.md`);
const fixtureRoot = path.join(DEBUG_DIR, "e2e-fixtures", runId);

const cases = [];

function addCase(id, name, status, detail = "", artifact = "") {
  cases.push({ id, name, status, detail, artifact });
  const prefix = status === "PASS" ? "[PASS]" : status === "FAIL" ? "[FAIL]" : "[SKIP]";
  console.log(`${prefix} ${id} ${name}${detail ? ` -> ${detail}` : ""}`);
}

async function ensureDirs() {
  await fs.mkdir(runScreenshotDir, { recursive: true });
  await fs.mkdir(REPORT_DIR, { recursive: true });
  await fs.mkdir(fixtureRoot, { recursive: true });
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
  const locator = page.locator(".n-message").filter({ hasText: text }).last();
  await locator.waitFor({ timeout: 20000 });
}

async function findDialogByTitle(page, title) {
  const dialog = page.locator('[role="dialog"]').filter({ hasText: title }).last();
  await dialog.waitFor({ timeout: 20000 });
  return dialog;
}

function labeledField(dialog, label) {
  return dialog.locator(".n-form-item").filter({ hasText: label });
}

async function setInputValue(locator, value) {
  await locator.click();
  await locator.press(process.platform === "darwin" ? "Meta+A" : "Control+A");
  await locator.fill(String(value));
  await locator.press("Tab");
}

async function applyMovieManualIdentify(page, organizeDialog, fileName, title, year) {
  const row = organizeDialog.locator("tbody tr").filter({ hasText: fileName }).first();
  await row.locator("button").filter({ hasText: "手动识别" }).click();
  const manualDialog = await findDialogByTitle(page, "手动识别修正");
  await manualDialog.locator(".n-select").first().click();
  await page.locator(".n-base-select-option").filter({ hasText: "电影" }).last().click();
  await setInputValue(labeledField(manualDialog, "标题").locator("input").first(), title);
  await setInputValue(labeledField(manualDialog, "年份").locator(".n-input-number input").first(), year);
  await manualDialog.locator("button").filter({ hasText: "应用到预览" }).click();
  await manualDialog.waitFor({ state: "hidden", timeout: 25000 });
  await waitForMessage(page, "预览完成");
}

async function closeDialog(dialog) {
  await dialog.locator("button").filter({ hasText: "关闭" }).click();
  await dialog.waitFor({ state: "hidden", timeout: 15000 });
}

async function listExisting(rootDir) {
  try {
    return await fs.readdir(rootDir);
  } catch {
    return [];
  }
}

async function runPolicy({ page, api, policy, label, sourceDir, targetDir, sourceName }) {
  const categorizedDir = path.join(targetDir, "未分类");
  const existingPath = path.join(categorizedDir, "Conflict Policy Movie (2024).mkv");
  const sourcePath = path.join(sourceDir, sourceName);
  const existingContent = `existing-${policy}`;
  const sourceContent = `source-${policy}`;
  await fs.mkdir(sourceDir, { recursive: true });
  await fs.mkdir(categorizedDir, { recursive: true });
  await fs.writeFile(sourcePath, sourceContent, "utf8");
  await fs.writeFile(existingPath, existingContent, "utf8");

  const createSourceResp = await api.post("/media/sources", {
    data: {
      name: `conflict_${policy}_${stamp}`,
      source_type: "local",
      path: sourceDir,
      organize_target_path: targetDir,
      auto_organize: false,
      watch_enabled: false,
      watch_interval: 300,
      emby_library_id: "",
      enabled: true,
      priority: 10,
      conflict_policy: policy
    }
  });
  const createSourceData = unwrapData(await apiJson(createSourceResp)) || {};
  const sourceId = createSourceData.id || null;
  if (!createSourceResp.ok() || !sourceId) {
    throw new Error(`创建 ${policy} 测试媒体源失败: ${JSON.stringify(createSourceData)}`);
  }

  try {
    await page.goto(`${FRONTEND_URL}/dashboard/media-manager`, { waitUntil: "networkidle" });
    const sourceRow = page.locator("tbody tr").filter({ hasText: `conflict_${policy}_${stamp}` }).first();
    await sourceRow.waitFor({ timeout: 20000 });
    await sourceRow.locator("button").first().click();

    const browserDialog = page.getByRole("dialog").filter({ hasText: "文件浏览" });
    await browserDialog.waitFor({ timeout: 20000 });
    const fileRow = browserDialog.locator("tbody tr").filter({ hasText: sourceName }).first();
    await fileRow.waitFor({ timeout: 20000 });
    await fileRow.locator('[role="checkbox"]').click();
    await browserDialog.locator("button").filter({ hasText: "批量整理" }).click();

    const organizeDialog = await findDialogByTitle(page, "批量整理");
    await organizeDialog.locator(".n-form-item").filter({ hasText: "冲突策略" }).locator(".n-select").click();
    await page.locator(".n-base-select-option").filter({ hasText: label }).last().click();
    await organizeDialog.locator("button").filter({ hasText: "刷新预览" }).click();
    await waitForMessage(page, "预览完成");
    await applyMovieManualIdentify(page, organizeDialog, sourceName, "Conflict Policy Movie", 2024);
    await saveShot(page, `${policy}_preview`);

    await organizeDialog.locator("button").filter({ hasText: "执行整理" }).click();
    await waitForMessage(page, "整理完成");
    const resultDialog = await findDialogByTitle(page, "整理结果");
    const dialogText = ((await resultDialog.textContent()) || "").replace(/\s+/g, " ");
    const shot = await saveShot(page, `${policy}_result`);

    const targetFiles = await listExisting(categorizedDir);
    const existingAfter = await fs.readFile(existingPath, "utf8").catch(() => "");
    const suffixedPath = path.join(categorizedDir, "Conflict Policy Movie (2024) (1).mkv");
    const suffixedContent = await fs.readFile(suffixedPath, "utf8").catch(() => "");
    const sourceStillExists = await fs.access(sourcePath).then(() => true).catch(() => false);

    let pass = false;
    let detail = "";
    if (policy === "skip") {
      pass = dialogText.includes("跳过") && existingAfter === existingContent && sourceStillExists;
      detail = `dialog=${dialogText.slice(0, 220)} target=${targetFiles.join(",")} sourceExists=${sourceStillExists}`;
    } else if (policy === "overwrite") {
      pass = dialogText.includes("成功") && existingAfter === sourceContent && !sourceStillExists && !targetFiles.includes(path.basename(suffixedPath));
      detail = `dialog=${dialogText.slice(0, 220)} overwritten=${existingAfter} sourceExists=${sourceStillExists}`;
    } else if (policy === "suffix") {
      pass = dialogText.includes("成功") && existingAfter === existingContent && suffixedContent === sourceContent && !sourceStillExists;
      detail = `dialog=${dialogText.slice(0, 220)} target=${targetFiles.join(",")} suffixed=${path.basename(suffixedPath)}`;
    }

    addCase(
      `TC-ORG-CONFLICT-${policy.toUpperCase()}-001`,
      `冲突策略 ${label} 行为符合预期`,
      pass ? "PASS" : "FAIL",
      detail,
      shot
    );

    await closeDialog(resultDialog);
    const fileBrowserClose = browserDialog.locator('[aria-label="close"]').last();
    await fileBrowserClose.click().catch(() => {});
  } finally {
    await api.delete(`/media/sources/${sourceId}`).catch(() => {});
  }
}

async function run() {
  let browser;
  let api;

  try {
    await ensureDirs();

    browser = await chromium.launch({ headless: true });
    const context = await browser.newContext({ viewport: { width: 1560, height: 980 } });
    const page = await context.newPage();

    await page.goto(`${FRONTEND_URL}/login`, { waitUntil: "networkidle" });
    await page.locator('input[placeholder*="用户名"], input[type="text"]').first().fill("admin");
    await page.locator('input[type="password"]').first().fill("admin");
    await page.getByRole("button", { name: "登录" }).click();
    await page.waitForURL(/\/dashboard(\/|$)/, { timeout: 30000, waitUntil: "commit" });
    addCase("TC-ORG-CONFLICT-AUTH-001", "登录成功", "PASS", "", await saveShot(page, "01_login"));

    const token = await page.evaluate(() => localStorage.getItem("token"));
    if (!token) throw new Error("登录后未获取 token");
    api = await request.newContext({
      baseURL: BACKEND_API,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` }
    });

    const policies = [
      { policy: "skip", label: "跳过" },
      { policy: "overwrite", label: "覆盖" },
      { policy: "suffix", label: "追加序号" }
    ];

    for (const entry of policies) {
      const baseDir = path.join(fixtureRoot, entry.policy);
      await runPolicy({
        page,
        api,
        policy: entry.policy,
        label: entry.label,
        sourceDir: path.join(baseDir, "source"),
        targetDir: path.join(baseDir, "target"),
        sourceName: `Conflict.${entry.policy}.Release.mkv`
      });
    }

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
      "# 整理冲突策略对比测试报告",
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
    addCase("TC-ORG-CONFLICT-RUN-000", "整理冲突策略执行异常", "FAIL", `${error.name}: ${error.message}`);
    const lines = [
      "# 整理冲突策略对比测试报告",
      "",
      `- 执行异常: ${error.stack || error.message}`,
      "",
      "| 用例ID | 名称 | 结果 | 说明 | 证据 |",
      "| --- | --- | --- | --- | --- |",
      ...cases.map((item) => `| ${item.id} | ${item.name} | ${item.status} | ${item.detail || "-"} | ${item.artifact || "-"} |`)
    ];
    await fs.writeFile(reportFile, `${lines.join("\n")}\n`, "utf8");
    console.error(JSON.stringify({ report: reportFile, screenshots: runScreenshotDir }, null, 2));
    process.exitCode = 1;
  } finally {
    if (api) await api.dispose().catch(() => {});
    if (browser) await browser.close();
  }
}

run();
