import fs from "node:fs/promises";
import path from "node:path";
import { chromium, request } from "playwright";

const FRONTEND_URL = process.env.E2E_FRONTEND_URL || "http://127.0.0.1:3001";
const BACKEND_API = process.env.E2E_BACKEND_API || "http://127.0.0.1:8082";
const ROOT_DIR = path.resolve(process.cwd(), "..");
const DEBUG_DIR = path.join(ROOT_DIR, "debug");
const SCREENSHOT_DIR = path.join(DEBUG_DIR, "e2e-screenshots", "organize-manual-identify");
const REPORT_DIR = path.join(ROOT_DIR, "docs");

const now = new Date();
const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, "0")}${String(now.getDate()).padStart(2, "0")}_${String(now.getHours()).padStart(2, "0")}${String(now.getMinutes()).padStart(2, "0")}${String(now.getSeconds()).padStart(2, "0")}`;
const runId = `organize_manual_identify_${stamp}`;
const runScreenshotDir = path.join(SCREENSHOT_DIR, runId);
const reportFile = path.join(REPORT_DIR, `整理手动识别深挖测试报告_${stamp}.md`);
const fixtureRoot = path.join(DEBUG_DIR, "e2e-fixtures", runId);
const sourceDir = path.join(fixtureRoot, "source");
const targetDir = path.join(fixtureRoot, "target");
const fixtureFileName = `Codex.Manually.Fixed.Release.${stamp}.mkv`;

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
  await fs.writeFile(path.join(sourceDir, fixtureFileName), "manual identify fixture\n", "utf8");
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
  const locator = page.locator(".n-message").filter({ hasText: text }).last();
  await locator.waitFor({ timeout: 20000 });
}

async function closeDialogByTitle(page, title) {
  const dialog = page.locator('[role="dialog"]').filter({ hasText: title }).last();
  if (await dialog.count()) {
    const closeBtn = dialog.locator('[aria-label="close"]').last();
    if (await closeBtn.count()) {
      await closeBtn.click();
      await dialog.waitFor({ state: "hidden", timeout: 15000 }).catch(() => {});
    }
  }
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

async function setInputNumber(dialog, label, value) {
  const input = labeledField(dialog, label).locator(".n-input-number input").first();
  await setInputValue(input, value);
  await dialog.page().waitForTimeout(150);
}

async function waitForPreviewReady(organizeDialog, expectedText) {
  await organizeDialog.waitFor({ state: "visible", timeout: 15000 });
  await organizeDialog.locator("table").first().waitFor({ timeout: 20000 });
  const deadline = Date.now() + 25000;
  while (Date.now() < deadline) {
    const text = (await organizeDialog.textContent()) || "";
    const ready = !text.includes("待刷新") && text.includes("预览状态已生成");
    const matched = !expectedText || text.includes(expectedText);
    if (ready && matched) {
      return;
    }
    await organizeDialog.page().waitForTimeout(300);
  }
  throw new Error(`等待整理预览完成超时: expectedText=${expectedText || "<none>"}`);
}

async function waitForRowUpdateDone(organizeDialog, expectedText) {
  const deadline = Date.now() + 25000;
  while (Date.now() < deadline) {
    const text = (await organizeDialog.textContent()) || "";
    const rowUpdated = text.includes("已修正") && text.includes("重新修正");
    const matched = !expectedText || text.includes(expectedText);
    const notUpdating = !text.includes("更新中");
    if (rowUpdated && matched && notUpdating) {
      return;
    }
    await organizeDialog.page().waitForTimeout(250);
  }
  throw new Error(`等待单行预览更新完成超时: expectedText=${expectedText || "<none>"}`);
}

async function run() {
  let browser;
  let api;
  let mediaSourceId = null;

  try {
    await ensureDirs();
    process.env.NO_PROXY = process.env.NO_PROXY
      ? `${process.env.NO_PROXY},127.0.0.1,localhost`
      : "127.0.0.1,localhost";

    browser = await chromium.launch({ headless: true });
    const context = await browser.newContext({ viewport: { width: 1560, height: 980 } });
    const page = await context.newPage();
    const requestStats = {
      asyncPreview: 0,
      syncPreview: 0
    };

    page.on("request", (request) => {
      const url = request.url();
      if (url.includes("/api/media/organize/preview/async")) {
        requestStats.asyncPreview += 1;
      }
      if (url.includes("/api/media/organize/preview") && !url.includes("/api/media/organize/preview/async")) {
        requestStats.syncPreview += 1;
      }
    });

    await page.goto(`${FRONTEND_URL}/login`, { waitUntil: "networkidle" });
    await page.locator('input[placeholder*="用户名"], input[type="text"]').first().fill("admin");
    await page.locator('input[type="password"]').first().fill("admin");
    await page.getByRole("button", { name: "登录" }).click();
    await page.waitForURL(/\/dashboard(\/|$)/, { timeout: 30000, waitUntil: "commit" });
    addCase("TC-ORG-MAN-AUTH-001", "登录成功", "PASS", "", await saveShot(page, "01_login"));

    const token = await page.evaluate(() => localStorage.getItem("token"));
    if (!token) throw new Error("登录后未获取 token");
    api = await request.newContext({
      baseURL: BACKEND_API,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` }
    });

    const tmdbConfigResp = await api.get("/media/tmdb/config");
    const tmdbConfig = unwrapData(await apiJson(tmdbConfigResp)) || {};

    const mediaSourceName = `organize_manual_${stamp}`;
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
    const fileRow = browserDialog.locator("tbody tr").filter({ hasText: fixtureFileName }).first();
    await fileRow.waitFor({ timeout: 15000 });
    await fileRow.locator('[role="checkbox"]').click();
    await browserDialog.locator("button").filter({ hasText: "批量整理" }).click();

    const organizeDialog = await findDialogByTitle(page, "批量整理");
    await organizeDialog.locator("button").filter({ hasText: "刷新预览" }).click();
    await waitForMessage(page, "预览完成");

    const previewTextBefore = await organizeDialog.textContent();
    const initialPreviewReady = (previewTextBefore || "").includes(fixtureFileName)
      && (previewTextBefore || "").includes("可处理");
    addCase(
      "TC-ORG-MAN-001",
      "批量整理预览可正常生成并展示当前样本",
      initialPreviewReady ? "PASS" : "FAIL",
      (previewTextBefore || "").replace(/\s+/g, " ").slice(0, 180),
      await saveShot(page, "02_initial_preview")
    );

    await organizeDialog.locator("tbody tr button").filter({ hasText: "手动识别" }).first().click();
    const manualDialog = await findDialogByTitle(page, "手动识别修正");

    await manualDialog.locator(".n-select").first().click();
    await page.locator(".n-base-select-option").filter({ hasText: "剧集" }).last().click();
    await setInputValue(labeledField(manualDialog, "标题").locator("input").first(), "Codex Manual Show");
    await setInputNumber(manualDialog, "年份", 2024);
    await setInputNumber(manualDialog, "季数", 1);
    await setInputNumber(manualDialog, "集数", 2);
    await manualDialog.locator("button").filter({ hasText: "应用到预览" }).click();
    await manualDialog.waitFor({ state: "hidden", timeout: 25000 });
    await waitForMessage(page, "当前行已更新到预览，无需重新刷新整批预览");
    await waitForRowUpdateDone(organizeDialog, "Codex Manual Show");

    const previewTextAfterManual = await organizeDialog.textContent();
    const manualPass = (previewTextAfterManual || "").includes("Codex Manual Show")
      && (previewTextAfterManual || "").includes("S01E02")
      && (previewTextAfterManual || "").includes("已手动修正")
      && (previewTextAfterManual || "").includes("已修正")
      && (previewTextAfterManual || "").includes("重新修正");
    addCase(
      "TC-ORG-MAN-002",
      "手动识别修正后当前行立即回流标题、季集号与修正状态",
      manualPass ? "PASS" : "FAIL",
      (previewTextAfterManual || "").replace(/\s+/g, " ").slice(0, 220),
      await saveShot(page, "03_manual_override_preview")
    );

    const asyncPreviewAfterManualPass = requestStats.asyncPreview === 1;
    addCase(
      "TC-ORG-MAN-002A",
      "应用到预览后不会再次触发整批异步预览",
      asyncPreviewAfterManualPass ? "PASS" : "FAIL",
      `asyncPreview=${requestStats.asyncPreview}`,
      await saveShot(page, "03a_no_async_preview_after_manual")
    );

    const syncPreviewAfterManualPass = requestStats.syncPreview >= 1;
    addCase(
      "TC-ORG-MAN-002B",
      "应用到预览后会触发单行同步预览刷新",
      syncPreviewAfterManualPass ? "PASS" : "FAIL",
      `syncPreview=${requestStats.syncPreview}`,
      ""
    );

    if (tmdbConfig.has_key) {
      await organizeDialog.locator("tbody tr button").filter({ hasText: /手动识别|重新修正/ }).first().click();
      const manualDialog2 = await findDialogByTitle(page, "手动识别修正");
      await manualDialog2.locator("button").filter({ hasText: "从 TMDB 选择" }).click();

      const tmdbDialog = await findDialogByTitle(page, "TMDB 手动搜索");
      await setInputValue(tmdbDialog.locator('input[placeholder="输入电影或剧集名称搜索"]').first(), "Breaking Bad");
      await tmdbDialog.locator(".n-select").click();
      await page.locator(".n-base-select-option").filter({ hasText: "剧集" }).last().click();
      await tmdbDialog.getByRole("button", { name: "搜索" }).click();
      await page.waitForTimeout(2500);
      const resultCards = tmdbDialog.getByTestId("tmdb-result-item");
      const resultCount = await resultCards.count();
      const tmdbSearchPass = resultCount > 0;
      addCase(
        "TC-ORG-MAN-003",
        "整理手动识别中的 TMDB 手动搜索能返回结果",
        tmdbSearchPass ? "PASS" : "FAIL",
        `resultCount=${resultCount}`,
        await saveShot(page, "04_tmdb_search_results")
      );

      if (resultCount > 0) {
        await resultCards.first().click();
        await tmdbDialog.waitFor({ state: "hidden", timeout: 15000 });
        await manualDialog2.waitFor({ state: "visible", timeout: 15000 });
        const titleInputValue = await labeledField(manualDialog2, "标题").locator("input").inputValue();
        const tmdbIdValue = await labeledField(manualDialog2, "TMDB ID").locator(".n-input-number input").first().inputValue();
        const selectPass = titleInputValue.trim() !== "" && tmdbIdValue.trim() !== "" && tmdbIdValue.trim() !== "0";
        addCase(
          "TC-ORG-MAN-004",
          "选择 TMDB 结果后会回写到手动识别表单",
          selectPass ? "PASS" : "FAIL",
          `title=${titleInputValue} tmdbId=${tmdbIdValue}`,
          await saveShot(page, "05_tmdb_selected")
        );

        await manualDialog2.locator("button").filter({ hasText: "应用到预览" }).click();
        await manualDialog2.waitFor({ state: "hidden", timeout: 25000 });
        await waitForMessage(page, "当前行已更新到预览，无需重新刷新整批预览");
        await waitForRowUpdateDone(organizeDialog, titleInputValue.trim());
        const previewTextAfterTmdb = await organizeDialog.textContent();
        const tmdbApplyPass = previewTextAfterTmdb.includes(titleInputValue.trim());
        addCase(
          "TC-ORG-MAN-005",
          "TMDB 选择结果应用后会刷新整理预览",
          tmdbApplyPass ? "PASS" : "FAIL",
          (previewTextAfterTmdb || "").replace(/\s+/g, " ").slice(0, 220),
          await saveShot(page, "06_tmdb_preview_applied")
        );
      } else {
        await closeDialogByTitle(page, "TMDB 手动搜索");
        addCase("TC-ORG-MAN-004", "选择 TMDB 结果后会回写到手动识别表单", "SKIP", "TMDB 搜索无结果");
        addCase("TC-ORG-MAN-005", "TMDB 选择结果应用后会刷新整理预览", "SKIP", "TMDB 搜索无结果");
      }
    } else {
      addCase("TC-ORG-MAN-003", "整理手动识别中的 TMDB 手动搜索能返回结果", "SKIP", "当前环境未配置可用 TMDB API Key");
      addCase("TC-ORG-MAN-004", "选择 TMDB 结果后会回写到手动识别表单", "SKIP", "当前环境未配置可用 TMDB API Key");
      addCase("TC-ORG-MAN-005", "TMDB 选择结果应用后会刷新整理预览", "SKIP", "当前环境未配置可用 TMDB API Key");
    }

    await organizeDialog.locator("button").filter({ hasText: "执行整理" }).click();
    await waitForMessage(page, "整理完成");
    await page.waitForTimeout(1200);
    const targetFiles = await listFilesRecursive(targetDir);
    const executePass = targetFiles.length > 0;
    addCase(
      "TC-ORG-MAN-006",
      "手动识别修正后的整理执行成功落盘",
      executePass ? "PASS" : "FAIL",
      `target=${targetFiles.join(",")}`,
      await saveShot(page, "07_execute_done")
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
    lines.push("# 整理手动识别深挖测试报告");
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
    addCase("TC-ORG-MAN-RUN-000", "整理手动识别深挖执行异常", "FAIL", String(error));
    const lines = [];
    lines.push("# 整理手动识别深挖测试报告");
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
