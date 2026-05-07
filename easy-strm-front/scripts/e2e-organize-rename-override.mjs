import fs from "node:fs/promises";
import path from "node:path";
import { chromium, request } from "playwright";

const FRONTEND_URL = process.env.E2E_FRONTEND_URL || "http://localhost:3001";
const BACKEND_API = process.env.E2E_BACKEND_API || "http://localhost:8082";
const ROOT_DIR = path.resolve(process.cwd(), "..");
const DEBUG_DIR = path.join(ROOT_DIR, "debug");
const SCREENSHOT_DIR = path.join(DEBUG_DIR, "e2e-screenshots", "organize-rename-override");
const REPORT_DIR = path.join(ROOT_DIR, "docs");

const now = new Date();
const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, "0")}${String(now.getDate()).padStart(2, "0")}_${String(now.getHours()).padStart(2, "0")}${String(now.getMinutes()).padStart(2, "0")}${String(now.getSeconds()).padStart(2, "0")}`;
const runId = `organize_override_${stamp}`;
const runScreenshotDir = path.join(SCREENSHOT_DIR, runId);
const reportFile = path.join(REPORT_DIR, `整理重命名覆盖测试报告_${stamp}.md`);
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
  await fs.writeFile(path.join(sourceDir, "Inception.2010.1080p.mkv"), "override fixture\n", "utf8");
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
  return results;
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
    await page.waitForURL("**/dashboard/**", { timeout: 15000 });
    addCase("TC-ORG-OVR-AUTH-001", "登录成功", "PASS", "", await saveShot(page, "01_login"));

    const token = await page.evaluate(() => localStorage.getItem("token"));
    if (!token) throw new Error("登录后未获取 token");

    api = await request.newContext({
      baseURL: BACKEND_API,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` }
    });

    const mediaSourceName = `organize_override_${stamp}`;
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
    const fileRow = browserDialog.locator(".el-table__row").filter({ hasText: "Inception.2010.1080p.mkv" }).first();
    await fileRow.waitFor({ timeout: 15000 });
    await fileRow.locator(".el-checkbox").click();
    await browserDialog.locator(".el-button").filter({ hasText: "批量整理" }).click();

    const organizeDialog = page.locator(".el-dialog").filter({ hasText: "批量整理" }).last();
    await organizeDialog.waitFor({ timeout: 15000 });
    await organizeDialog.locator(".dialog-footer .el-button").filter({ hasText: "刷新预览" }).click();
    await waitForMessage(page, "预览完成");

    const previewRow = organizeDialog.locator(".el-table__row").first();
    await previewRow.waitFor({ timeout: 15000 });
    await previewRow.locator(".edit-name-btn").click();
    const editInput = previewRow.locator("input").first();
    await editInput.fill("Manual Override Final.mkv");
    await editInput.press("Enter");
    await page.waitForTimeout(500);

    const previewText = await organizeDialog.textContent();
    const previewPass = (previewText || "").includes("Manual Override Final.mkv");
    addCase(
      "TC-ORG-OVR-001",
      "整理预览中可编辑新文件名",
      previewPass ? "PASS" : "FAIL",
      (previewText || "").replace(/\s+/g, " ").slice(0, 200),
      await saveShot(page, "02_preview_edited")
    );

    await organizeDialog.locator(".dialog-footer .el-button--primary").filter({ hasText: "执行整理" }).click();
    await waitForMessage(page, "整理完成");
    await page.waitForTimeout(1200);

    const targetFiles = await listFilesRecursive(targetDir);
    const overrideApplied = targetFiles.some((item) => item.endsWith("/Manual Override Final.mkv") || item === "Manual Override Final.mkv");
    addCase(
      "TC-ORG-OVR-002",
      "执行整理后应使用手动修改后的文件名",
      overrideApplied ? "PASS" : "FAIL",
      `target=${targetFiles.join(",")}`,
      await saveShot(page, "03_execute_done")
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

    const lines = [];
    lines.push("# 整理重命名覆盖测试报告");
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
    addCase("TC-ORG-OVR-RUN-000", "整理重命名覆盖执行异常", "FAIL", String(error));
    const lines = [];
    lines.push("# 整理重命名覆盖测试报告");
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
      await api.dispose().catch(() => {});
    }
    if (browser) {
      await browser.close();
    }
  }
}

run();
