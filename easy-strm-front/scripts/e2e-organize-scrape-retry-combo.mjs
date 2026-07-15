import fs from "node:fs/promises";
import path from "node:path";
import { chromium, request } from "playwright";

const FRONTEND_URL = process.env.E2E_FRONTEND_URL || "http://localhost:3001";
const BACKEND_API = process.env.E2E_BACKEND_API || "http://localhost:8082";
const ROOT_DIR = path.resolve(process.cwd(), "..");
const DEBUG_DIR = path.join(ROOT_DIR, "debug");
const SCREENSHOT_DIR = path.join(DEBUG_DIR, "e2e-screenshots", "organize-scrape-retry-combo");
const REPORT_DIR = path.join(ROOT_DIR, "docs");

const now = new Date();
const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, "0")}${String(now.getDate()).padStart(2, "0")}_${String(now.getHours()).padStart(2, "0")}${String(now.getMinutes()).padStart(2, "0")}${String(now.getSeconds()).padStart(2, "0")}`;
const runId = `organize_scrape_retry_combo_${stamp}`;
const runScreenshotDir = path.join(SCREENSHOT_DIR, runId);
const reportFile = path.join(REPORT_DIR, `整理刮削重试组合测试报告_${stamp}.md`);
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
  await fs.writeFile(path.join(sourceDir, "Combo.Retry.Success.Part1.mkv"), "combo retry file 1\n", "utf8");
  await fs.writeFile(path.join(sourceDir, "Combo.Retry.Fail.Part2.mkv"), "combo retry file 2\n", "utf8");
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
  await locator.waitFor({ timeout: 25000 });
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

async function searchAndSelectTmdb(page, manualDialog, keyword, type) {
  await manualDialog.locator("button").filter({ hasText: "从 TMDB 选择" }).click();
  const tmdbDialog = await findDialogByTitle(page, "TMDB 手动搜索");
  await setInputValue(tmdbDialog.locator('input[placeholder="输入电影或剧集名称搜索"]').first(), keyword);
  await tmdbDialog.locator(".n-select").click();
  await page.locator(".n-base-select-option").filter({ hasText: type === "tv" ? "剧集" : "电影" }).last().click();
  await tmdbDialog.getByRole("button", { name: "搜索" }).click();
  await page.waitForTimeout(2500);
  const resultCards = tmdbDialog.getByTestId("tmdb-result-item");
  const resultCount = await resultCards.count();
  if (resultCount === 0) {
    throw new Error(`TMDB 搜索无结果: keyword=${keyword} type=${type}`);
  }
  await resultCards.first().click();
  await tmdbDialog.waitFor({ state: "hidden", timeout: 15000 });
}

async function applyTvTmdbIdentify(page, organizeDialog, fileName, season, episode) {
  const row = organizeDialog.locator("tbody tr").filter({ hasText: fileName }).first();
  await row.locator("button").filter({ hasText: "手动识别" }).click();
  const manualDialog = await findDialogByTitle(page, "手动识别修正");
  await searchAndSelectTmdb(page, manualDialog, "Breaking Bad", "tv");
  await setInputValue(labeledField(manualDialog, "季数").locator(".n-input-number input").first(), season);
  await setInputValue(labeledField(manualDialog, "集数").locator(".n-input-number input").first(), episode);
  const title = await labeledField(manualDialog, "标题").locator("input").first().inputValue();
  const tmdbId = await labeledField(manualDialog, "TMDB ID").locator(".n-input-number input").first().inputValue();
  await manualDialog.locator("button").filter({ hasText: "应用到预览" }).click();
  await manualDialog.waitFor({ state: "hidden", timeout: 25000 });
  await waitForMessage(page, "预览完成");
  return { title, tmdbId };
}

async function pathExists(target) {
  try {
    await fs.access(target);
    return true;
  } catch {
    return false;
  }
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
    await page.getByRole("button", { name: "登录" }).click();
    await page.waitForURL(/\/dashboard(\/|$)/, { timeout: 30000, waitUntil: "commit" });
    addCase("TC-ORG-COMBO-AUTH-001", "登录成功", "PASS", "", await saveShot(page, "01_login"));

    const token = await page.evaluate(() => localStorage.getItem("token"));
    if (!token) throw new Error("登录后未获取 token");
    api = await request.newContext({
      baseURL: BACKEND_API,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` }
    });

    const mediaSourceName = `organize_combo_${stamp}`;
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
    const sourceRow = page.locator("tbody tr").filter({ hasText: mediaSourceName }).first();
    await sourceRow.waitFor({ timeout: 15000 });
    await sourceRow.locator("button").first().click();

    const browserDialog = page.getByRole("dialog").filter({ hasText: "文件浏览" });
    await browserDialog.waitFor({ timeout: 15000 });
    for (const fileName of ["Combo.Retry.Success.Part1.mkv", "Combo.Retry.Fail.Part2.mkv"]) {
      const fileRow = browserDialog.locator("tbody tr").filter({ hasText: fileName }).first();
      await fileRow.waitFor({ timeout: 15000 });
      await fileRow.locator('[role="checkbox"]').click();
    }
    await browserDialog.locator("button").filter({ hasText: "批量整理" }).click();

    const organizeDialog = await findDialogByTitle(page, "批量整理");
    await organizeDialog.locator("button").filter({ hasText: "刷新预览" }).click();
    await waitForMessage(page, "预览完成");

    const firstIdentify = await applyTvTmdbIdentify(page, organizeDialog, "Combo.Retry.Success.Part1.mkv", 1, 1);
    const secondIdentify = await applyTvTmdbIdentify(page, organizeDialog, "Combo.Retry.Fail.Part2.mkv", 1, 2);
    const identifyPass = firstIdentify.tmdbId === "1396" && secondIdentify.tmdbId === "1396";
    addCase(
      "TC-ORG-COMBO-001",
      "两条剧集手动识别均成功回填 TMDB 信息",
      identifyPass ? "PASS" : "FAIL",
      `first=${firstIdentify.title}/${firstIdentify.tmdbId} second=${secondIdentify.title}/${secondIdentify.tmdbId}`,
      await saveShot(page, "02_identify_ready")
    );

    await organizeDialog.locator('[role="checkbox"]').filter({ hasText: "整理后同时刮削 NFO" }).click();
    await fs.unlink(path.join(sourceDir, "Combo.Retry.Fail.Part2.mkv"));
    await organizeDialog.locator("button").filter({ hasText: "执行整理" }).click();
    await waitForMessage(page, "整理完成");

    const resultDialog = await findDialogByTitle(page, "整理结果");
    const firstDialogText = ((await resultDialog.textContent()) || "").replace(/\s+/g, " ");
    const tvTargetDir = path.join(targetDir, "欧美剧");
    const firstNfo = path.join(tvTargetDir, "绝命毒师 - S01E01.nfo");
    const firstMkv = path.join(tvTargetDir, "绝命毒师 - S01E01.mkv");
    const initialPass = firstDialogText.includes("重试失败项 (1)") && await pathExists(firstMkv) && await pathExists(firstNfo);
    addCase(
      "TC-ORG-COMBO-002",
      "首次整理后成功项已落盘且自动刮削生成 NFO，失败项可重试",
      initialPass ? "PASS" : "FAIL",
      `dialog=${firstDialogText.slice(0, 260)} firstMkv=${await pathExists(firstMkv)} firstNfo=${await pathExists(firstNfo)}`,
      await saveShot(page, "03_first_execute")
    );

    await fs.writeFile(path.join(sourceDir, "Combo.Retry.Fail.Part2.mkv"), "combo retry restored file 2\n", "utf8");
    await resultDialog.locator("button").filter({ hasText: "重试失败项" }).click();
    await waitForMessage(page, "重试完成");
    await page.waitForTimeout(1500);

    const secondDialogText = ((await resultDialog.textContent()) || "").replace(/\s+/g, " ");
    const secondMkv = path.join(tvTargetDir, "绝命毒师 - S01E02.mkv");
    const secondNfo = path.join(tvTargetDir, "绝命毒师 - S01E02.nfo");
    const retryPass = !secondDialogText.includes("重试失败项 (") && await pathExists(secondMkv) && await pathExists(secondNfo);
    addCase(
      "TC-ORG-COMBO-003",
      "重试失败项后补齐成功文件与对应 NFO",
      retryPass ? "PASS" : "FAIL",
      `dialog=${secondDialogText.slice(0, 260)} secondMkv=${await pathExists(secondMkv)} secondNfo=${await pathExists(secondNfo)}`,
      await saveShot(page, "04_retry_result")
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
      "# 整理刮削重试组合测试报告",
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
    addCase("TC-ORG-COMBO-RUN-000", "整理刮削重试组合执行异常", "FAIL", `${error.name}: ${error.message}`);
    const lines = [
      "# 整理刮削重试组合测试报告",
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
    if (api) {
      if (mediaSourceId) {
        await api.delete(`/media/sources/${mediaSourceId}`).catch(() => {});
      }
      await api.dispose().catch(() => {});
    }
    if (browser) await browser.close();
  }
}

run();
