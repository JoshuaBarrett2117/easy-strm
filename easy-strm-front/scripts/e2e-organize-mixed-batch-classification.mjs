import fs from "node:fs/promises";
import path from "node:path";
import { chromium, request } from "playwright";

const FRONTEND_URL = process.env.E2E_FRONTEND_URL || "http://localhost:3001";
const BACKEND_API = process.env.E2E_BACKEND_API || "http://localhost:8082";
const ROOT_DIR = path.resolve(process.cwd(), "..");
const DEBUG_DIR = path.join(ROOT_DIR, "debug");
const SCREENSHOT_DIR = path.join(DEBUG_DIR, "e2e-screenshots", "organize-mixed-batch");
const REPORT_DIR = path.join(ROOT_DIR, "docs");

const now = new Date();
const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, "0")}${String(now.getDate()).padStart(2, "0")}_${String(now.getHours()).padStart(2, "0")}${String(now.getMinutes()).padStart(2, "0")}${String(now.getSeconds()).padStart(2, "0")}`;
const runId = `organize_mixed_batch_${stamp}`;
const runScreenshotDir = path.join(SCREENSHOT_DIR, runId);
const reportFile = path.join(REPORT_DIR, `整理混合批量分类测试报告_${stamp}.md`);
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
  await fs.writeFile(path.join(sourceDir, "Codex.Mixed.Movie.Release.mkv"), "mixed movie fixture\n", "utf8");
  await fs.writeFile(path.join(sourceDir, "Codex.Mixed.Show.Release.mkv"), "mixed tv fixture\n", "utf8");
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

async function setInputValue(locator, value) {
  await locator.click();
  await locator.press(process.platform === "darwin" ? "Meta+A" : "Control+A");
  await locator.fill(String(value));
  await locator.press("Tab");
}

function labeledField(dialog, label) {
  return dialog.locator(".el-form-item").filter({ hasText: label });
}

async function searchAndSelectTmdb(page, manualDialog, keyword, type) {
  await manualDialog.locator(".dialog-footer .el-button").filter({ hasText: "从 TMDB 选择" }).click();
  const tmdbDialog = await findDialogByTitle(page, "TMDB 手动搜索");
  await setInputValue(tmdbDialog.locator(".tmdb-search input").first(), keyword);
  await tmdbDialog.locator(".search-type-select").click();
  await page.locator(".el-select-dropdown__item").filter({ hasText: type === "tv" ? "剧集" : "电影" }).last().click();
  await tmdbDialog.locator(".search-btn").click();
  await page.waitForTimeout(2500);
  const resultCards = tmdbDialog.locator(".result-item");
  const resultCount = await resultCards.count();
  if (resultCount === 0) {
    throw new Error(`TMDB 搜索无结果: keyword=${keyword} type=${type}`);
  }
  await resultCards.first().click();
  await tmdbDialog.waitFor({ state: "hidden", timeout: 15000 });
}

async function applyMovieManualIdentify(page, organizeDialog, fileName, keyword) {
  const row = organizeDialog.locator(".el-table__row").filter({ hasText: fileName }).first();
  await row.locator(".el-button").filter({ hasText: "手动识别" }).click();
  const manualDialog = await findDialogByTitle(page, "手动识别修正");
  await searchAndSelectTmdb(page, manualDialog, keyword, "movie");
  const title = await labeledField(manualDialog, "标题").locator("input").first().inputValue();
  const tmdbId = await labeledField(manualDialog, "TMDB ID").locator(".el-input-number input").first().inputValue();
  await manualDialog.locator(".dialog-footer .el-button--primary").filter({ hasText: "应用到预览" }).click();
  await manualDialog.waitFor({ state: "hidden", timeout: 25000 });
  await waitForMessage(page, "预览完成");
  return { title, tmdbId };
}

async function applyTvManualIdentify(page, organizeDialog, fileName, keyword, season, episode) {
  const row = organizeDialog.locator(".el-table__row").filter({ hasText: fileName }).first();
  await row.locator(".el-button").filter({ hasText: "手动识别" }).click();
  const manualDialog = await findDialogByTitle(page, "手动识别修正");
  await searchAndSelectTmdb(page, manualDialog, keyword, "tv");
  await setInputValue(labeledField(manualDialog, "季数").locator(".el-input-number input").first(), season);
  await setInputValue(labeledField(manualDialog, "集数").locator(".el-input-number input").first(), episode);
  const title = await labeledField(manualDialog, "标题").locator("input").first().inputValue();
  const tmdbId = await labeledField(manualDialog, "TMDB ID").locator(".el-input-number input").first().inputValue();
  await manualDialog.locator(".dialog-footer .el-button--primary").filter({ hasText: "应用到预览" }).click();
  await manualDialog.waitFor({ state: "hidden", timeout: 25000 });
  await waitForMessage(page, "预览完成");
  return { title, tmdbId };
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
    await page.locator(".login-btn").click();
    await page.waitForURL(/\/dashboard(\/|$)/, { timeout: 30000, waitUntil: "commit" });
    addCase("TC-ORG-MIX-AUTH-001", "登录成功", "PASS", "", await saveShot(page, "01_login"));

    const token = await page.evaluate(() => localStorage.getItem("token"));
    if (!token) throw new Error("登录后未获取 token");
    api = await request.newContext({
      baseURL: BACKEND_API,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` }
    });

    for (const category of [
      {
        name: `Codex电影_${stamp}`,
        media_type: "movie",
        target_path: "/电影/科幻电影",
        match_rules: JSON.stringify({ keywords: ["inception"], default: false }),
        enabled: true
      },
      {
        name: `Codex电影兜底_${stamp}`,
        media_type: "movie",
        target_path: "/电影/未分类",
        match_rules: JSON.stringify({ default: true }),
        enabled: true
      },
      {
        name: `Codex剧集_${stamp}`,
        media_type: "tv",
        target_path: "/电视剧/欧美剧",
        match_rules: JSON.stringify({ keywords: ["绝命毒师"], default: false }),
        enabled: true
      },
      {
        name: `Codex剧集兜底_${stamp}`,
        media_type: "tv",
        target_path: "/电视剧/未分类",
        match_rules: JSON.stringify({ default: true }),
        enabled: true
      }
    ]) {
      const resp = await api.post("/media/categories", { data: category });
      const data = unwrapData(await apiJson(resp)) || {};
      if (!resp.ok() || !data.id) {
        throw new Error(`创建分类失败: ${JSON.stringify(data)}`);
      }
      createdCategoryIds.push(data.id);
    }

    const mediaSourceName = `organize_mixed_${stamp}`;
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
    for (const fileName of ["Codex.Mixed.Movie.Release.mkv", "Codex.Mixed.Show.Release.mkv"]) {
      const fileRow = browserDialog.locator(".el-table__row").filter({ hasText: fileName }).first();
      await fileRow.waitFor({ timeout: 15000 });
      await fileRow.locator(".el-checkbox").click();
    }
    await browserDialog.locator(".el-button").filter({ hasText: "批量整理" }).click();

    const organizeDialog = await findDialogByTitle(page, "批量整理");
    await organizeDialog.locator(".dialog-footer .el-button").filter({ hasText: "刷新预览" }).click();
    await waitForMessage(page, "预览完成");

    const movie = await applyMovieManualIdentify(page, organizeDialog, "Codex.Mixed.Movie.Release.mkv", "Inception");
    const tv = await applyTvManualIdentify(page, organizeDialog, "Codex.Mixed.Show.Release.mkv", "Breaking Bad", 1, 2);
    const tmdbPass = movie.tmdbId === "27205" && tv.tmdbId === "1396";
    addCase(
      "TC-ORG-MIX-001",
      "电影与剧集 TMDB 识别均能正确回写",
      tmdbPass ? "PASS" : "FAIL",
      `movie=${movie.title}/${movie.tmdbId} tv=${tv.title}/${tv.tmdbId}`,
      await saveShot(page, "02_tmdb_backfill")
    );

    const previewText = (await organizeDialog.textContent()) || "";
    const previewPass = previewText.includes("盗梦空间") && previewText.includes("绝命毒师") && previewText.includes("科幻电影") && previewText.includes("欧美剧");
    addCase(
      "TC-ORG-MIX-002",
      "混合批量预览能按电影和剧集分别命中分类目录",
      previewPass ? "PASS" : "FAIL",
      previewText.replace(/\s+/g, " ").slice(0, 280),
      await saveShot(page, "03_preview_mixed")
    );

    await organizeDialog.locator(".dialog-footer .el-button--primary").filter({ hasText: "执行整理" }).click();
    await waitForMessage(page, "整理完成");
    await page.waitForTimeout(1500);

    const targetFiles = await listFilesRecursive(targetDir);
    const executePass =
      targetFiles.some((item) => item.includes("科幻电影/盗梦空间")) &&
      targetFiles.some((item) => item.includes("欧美剧/绝命毒师 - S01E02.mkv"));
    addCase(
      "TC-ORG-MIX-003",
      "混合批量执行后电影与剧集分别落盘到正确分类目录",
      executePass ? "PASS" : "FAIL",
      `target=${targetFiles.join(",")}`,
      await saveShot(page, "04_execute_mixed")
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
      "# 整理混合批量分类测试报告",
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
    addCase("TC-ORG-MIX-RUN-000", "整理混合批量分类执行异常", "FAIL", `${error.name}: ${error.message}`);
    const lines = [
      "# 整理混合批量分类测试报告",
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
