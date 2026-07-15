import fs from "node:fs/promises";
import path from "node:path";
import { chromium, request } from "playwright";

const FRONTEND_URL = process.env.E2E_FRONTEND_URL || "http://localhost:3001";
const BACKEND_API = process.env.E2E_BACKEND_API || "http://localhost:8082";
const ROOT_DIR = path.resolve(process.cwd(), "..");
const DEBUG_DIR = path.join(ROOT_DIR, "debug");
const SCREENSHOT_DIR = path.join(DEBUG_DIR, "e2e-screenshots", "organize-category-conflict-suffix");
const REPORT_DIR = path.join(ROOT_DIR, "docs");

const now = new Date();
const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, "0")}${String(now.getDate()).padStart(2, "0")}_${String(now.getHours()).padStart(2, "0")}${String(now.getMinutes()).padStart(2, "0")}${String(now.getSeconds()).padStart(2, "0")}`;
const runId = `organize_category_conflict_suffix_${stamp}`;
const runScreenshotDir = path.join(SCREENSHOT_DIR, runId);
const reportFile = path.join(REPORT_DIR, `整理分类冲突追加序号测试报告_${stamp}.md`);
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
  await fs.writeFile(path.join(sourceDir, "Codex.Category.Conflict.Release.mkv"), "category conflict fixture\n", "utf8");
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
  await dialog.waitFor({ timeout: 15000 });
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

async function waitForPreviewReady(organizeDialog, expectedText) {
  const deadline = Date.now() + 25000;
  while (Date.now() < deadline) {
    const text = (await organizeDialog.textContent()) || "";
    const ready = !text.includes("待刷新") && text.includes("预览状态已生成");
    const matched = !expectedText || text.includes(expectedText);
    if (ready && matched) {
      return text;
    }
    await organizeDialog.page().waitForTimeout(300);
  }
  throw new Error(`等待整理预览完成超时: expectedText=${expectedText || "<none>"}`);
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
  const createdCategoryIds = [];

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
    addCase("TC-ORG-CAT-AUTH-001", "登录成功", "PASS", "", await saveShot(page, "01_login"));

    const token = await page.evaluate(() => localStorage.getItem("token"));
    if (!token) throw new Error("登录后未获取 token");
    api = await request.newContext({
      baseURL: BACKEND_API,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` }
    });

    const exactCategoryResp = await api.post("/media/categories", {
      data: {
        name: `Codex欧美剧_${stamp}`,
        media_type: "tv",
        target_path: "/电视剧/欧美剧",
        match_rules: JSON.stringify({ keywords: ["绝命毒师"], default: false }),
        enabled: true
      }
    });
    const exactCategory = unwrapData(await apiJson(exactCategoryResp)) || {};
    if (!exactCategoryResp.ok() || !exactCategory.id) {
      throw new Error(`创建关键词分类失败: ${JSON.stringify(exactCategory)}`);
    }
    createdCategoryIds.push(exactCategory.id);

    const fallbackCategoryResp = await api.post("/media/categories", {
      data: {
        name: `Codex未分类_${stamp}`,
        media_type: "tv",
        target_path: "/电视剧/未分类",
        match_rules: JSON.stringify({ default: true }),
        enabled: true
      }
    });
    const fallbackCategory = unwrapData(await apiJson(fallbackCategoryResp)) || {};
    if (!fallbackCategoryResp.ok() || !fallbackCategory.id) {
      throw new Error(`创建兜底分类失败: ${JSON.stringify(fallbackCategory)}`);
    }
    createdCategoryIds.push(fallbackCategory.id);

    const mediaSourceName = `organize_category_${stamp}`;
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
        priority: 10,
        conflict_policy: "suffix"
      }
    });
    const createSourceData = unwrapData(await apiJson(createSourceResp)) || {};
    mediaSourceId = createSourceData.id || null;
    if (!createSourceResp.ok() || !mediaSourceId) {
      throw new Error(`创建测试媒体源失败: ${JSON.stringify(createSourceData)}`);
    }

    await page.goto(`${FRONTEND_URL}/dashboard/media-manager`, { waitUntil: "networkidle" });
    const sourceRow = page.locator("tbody tr").filter({ hasText: mediaSourceName }).first();
    await sourceRow.waitFor({ timeout: 15000 });
    await sourceRow.locator("button").first().click();

    const browserDialog = page.getByRole("dialog").filter({ hasText: "文件浏览" });
    await browserDialog.waitFor({ timeout: 15000 });
    const fileRow = browserDialog.locator("tbody tr").filter({ hasText: "Codex.Category.Conflict.Release.mkv" }).first();
    await fileRow.waitFor({ timeout: 15000 });
    await fileRow.locator('[role="checkbox"]').click();
    await browserDialog.locator("button").filter({ hasText: "批量整理" }).click();

    const organizeDialog = await findDialogByTitle(page, "批量整理");
    await organizeDialog.locator("button").filter({ hasText: "刷新预览" }).click();
    await waitForMessage(page, "预览完成");

    await organizeDialog.locator("tbody tr button").filter({ hasText: "手动识别" }).first().click();
    const manualDialog = await findDialogByTitle(page, "手动识别修正");
    await manualDialog.locator("button").filter({ hasText: "从 TMDB 选择" }).click();

    const tmdbDialog = await findDialogByTitle(page, "TMDB 手动搜索");
    await setInputValue(tmdbDialog.locator('input[placeholder="输入电影或剧集名称搜索"]').first(), "Breaking Bad");
    await tmdbDialog.locator(".n-select").click();
    await page.locator(".n-base-select-option").filter({ hasText: "剧集" }).last().click();
    await tmdbDialog.getByRole("button", { name: "搜索" }).click();
    await page.waitForTimeout(2500);

    const resultCards = tmdbDialog.getByTestId("tmdb-result-item");
    const resultCount = await resultCards.count();
    if (resultCount === 0) {
      throw new Error("TMDB 搜索未返回 Breaking Bad 结果");
    }
    await resultCards.first().click();
    await tmdbDialog.waitFor({ state: "hidden", timeout: 15000 });
    await manualDialog.waitFor({ state: "visible", timeout: 15000 });

    const titleInputValue = await labeledField(manualDialog, "标题").locator("input").first().inputValue();
    const tmdbIdValue = await labeledField(manualDialog, "TMDB ID").locator(".n-input-number input").first().inputValue();
    const tmdbBackfillPass = titleInputValue.trim() !== "" && tmdbIdValue.trim() === "1396";
    addCase(
      "TC-ORG-CAT-001",
      "TMDB 结果会回写为绝命毒师条目",
      tmdbBackfillPass ? "PASS" : "FAIL",
      `title=${titleInputValue} tmdbId=${tmdbIdValue}`,
      await saveShot(page, "02_tmdb_selected")
    );

    await setInputValue(labeledField(manualDialog, "季数").locator(".n-input-number input").first(), 1);
    await setInputValue(labeledField(manualDialog, "集数").locator(".n-input-number input").first(), 2);
    await manualDialog.locator("button").filter({ hasText: "应用到预览" }).click();
    await manualDialog.waitFor({ state: "hidden", timeout: 25000 });
    await waitForMessage(page, "预览完成");
    const previewText = await waitForPreviewReady(organizeDialog, "绝命毒师");

    const categoryPreviewPass = previewText.includes("欧美剧") && previewText.includes("绝命毒师 - S01E02");
    addCase(
      "TC-ORG-CAT-002",
      "整理预览命中分类目录并生成剧集命名",
      categoryPreviewPass ? "PASS" : "FAIL",
      previewText.replace(/\s+/g, " ").slice(0, 240),
      await saveShot(page, "03_category_preview")
    );

    const conflictDir = path.join(targetDir, "欧美剧");
    await fs.mkdir(conflictDir, { recursive: true });
    const conflictFile = path.join(conflictDir, "绝命毒师 - S01E02.mkv");
    await fs.writeFile(conflictFile, "existing conflict\n", "utf8");

    await organizeDialog.locator(".n-form-item").filter({ hasText: "冲突策略" }).locator(".n-select").click();
    await page.locator(".n-base-select-option").filter({ hasText: "追加序号" }).last().click();
    await organizeDialog.locator("button").filter({ hasText: "刷新预览" }).click();
    await waitForMessage(page, "预览完成");
    const conflictPreviewText = await waitForPreviewReady(organizeDialog, "绝命毒师");
    const conflictPreviewPass = conflictPreviewText.includes("存在冲突");
    addCase(
      "TC-ORG-CAT-003",
      "冲突文件存在时预览能提示冲突",
      conflictPreviewPass ? "PASS" : "FAIL",
      conflictPreviewText.replace(/\s+/g, " ").slice(0, 240),
      await saveShot(page, "04_conflict_preview")
    );

    await organizeDialog.locator("button").filter({ hasText: "执行整理" }).click();
    await waitForMessage(page, "整理完成");
    await page.waitForTimeout(1200);

    const targetFiles = await listFilesRecursive(targetDir);
    const suffixPass = targetFiles.includes("欧美剧/绝命毒师 - S01E02 (1).mkv");
    addCase(
      "TC-ORG-CAT-004",
      "冲突策略为追加序号时执行结果会生成带后缀的新文件",
      suffixPass ? "PASS" : "FAIL",
      `target=${targetFiles.join(",")}`,
      await saveShot(page, "05_execute_result")
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
      "# 整理分类冲突追加序号测试报告",
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
    addCase("TC-ORG-CAT-RUN-000", "整理分类冲突追加序号执行异常", "FAIL", `${error.name}: ${error.message}`);
    const lines = [
      "# 整理分类冲突追加序号测试报告",
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
      for (const categoryId of createdCategoryIds.reverse()) {
        try {
          await api.delete(`/media/categories/${categoryId}`);
        } catch {}
      }
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
