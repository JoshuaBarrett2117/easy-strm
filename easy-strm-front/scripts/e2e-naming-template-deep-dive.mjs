import fs from "node:fs/promises";
import path from "node:path";
import { chromium, request } from "playwright";

const FRONTEND_URL = process.env.E2E_FRONTEND_URL || "http://localhost:3001";
const BACKEND_API = process.env.E2E_BACKEND_API || "http://localhost:8082";
const ROOT_DIR = path.resolve(process.cwd(), "..");
const DEBUG_DIR = path.join(ROOT_DIR, "debug");
const SCREENSHOT_DIR = path.join(DEBUG_DIR, "e2e-screenshots", "naming-template-deep-dive");
const REPORT_DIR = path.join(ROOT_DIR, "docs");

const now = new Date();
const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, "0")}${String(now.getDate()).padStart(2, "0")}_${String(now.getHours()).padStart(2, "0")}${String(now.getMinutes()).padStart(2, "0")}${String(now.getSeconds()).padStart(2, "0")}`;
const runId = `naming_template_${stamp}`;
const runScreenshotDir = path.join(SCREENSHOT_DIR, runId);
const reportFile = path.join(REPORT_DIR, `命名模板深挖测试报告_${stamp}.md`);
const fixtureRoot = path.join(DEBUG_DIR, "e2e-fixtures", runId);
const renameSource = path.join(fixtureRoot, "rename-source");

const MOVIE_TEMPLATE = "{{ title }} - MOVIE{{ fileExt }}";
const TV_TEMPLATE = '{{ title }} - S{{ "%02d"|format(season|int) }}E{{ "%02d"|format(episode|int) }} - TV{{ fileExt }}';
const TMDB_MOVIE_TEMPLATE = "{{ title }}{% if year %} ({{ year }}){% endif %}{{ fileExt }}";
const TMDB_TV_TEMPLATE = '{{ title }} - S{{ "%02d"|format(season|int) }}E{{ "%02d"|format(episode|int) }}{{ fileExt }}';

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
  await fs.writeFile(path.join(renameSource, "Template.Movie.2024.1080p.mkv"), "movie fixture\n", "utf8");
  await fs.writeFile(path.join(renameSource, "Template.Show.S01E02.1080p.mkv"), "tv fixture\n", "utf8");
  await fs.writeFile(path.join(renameSource, "Inception.2010.1080p.mkv"), "tmdb movie fixture\n", "utf8");
  await fs.writeFile(path.join(renameSource, "Breaking.Bad.S01E02.1080p.mkv"), "tmdb tv fixture\n", "utf8");
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

async function waitForSuccessMessage(page, text) {
  const message = page.locator(".n-message").filter({ hasText: text }).last();
  await message.waitFor({ timeout: 15000 });
}

async function openSourceBrowser(page, sourceName) {
  await page.goto(`${FRONTEND_URL}/dashboard/media-manager`, { waitUntil: "networkidle" });
  const sourceRow = page.locator("tbody tr").filter({ hasText: sourceName }).first();
  await sourceRow.waitFor({ timeout: 15000 });
  await sourceRow.locator("button").first().click();
  const browserDialog = page.getByRole("dialog").filter({ hasText: "文件浏览" });
  await browserDialog.waitFor({ timeout: 15000 });
  return browserDialog;
}

async function triggerBatchRenameForFile(page, browserDialog, fileName) {
  const row = browserDialog.locator("tbody tr").filter({ hasText: fileName }).first();
  await row.waitFor({ timeout: 15000 });
  await row.locator('[role="checkbox"]').click();
  await browserDialog.locator("button").filter({ hasText: "批量重命名" }).click();
  const renameDialog = page.locator('[role="dialog"]').filter({ hasText: "重命名预览" }).last();
  await renameDialog.waitFor({ timeout: 15000 });
  return renameDialog;
}

async function executeRenameAndWait(page, renameDialog) {
  await renameDialog.locator("button").click();
  await waitForSuccessMessage(page, "批量重命名");
  await page.waitForTimeout(1200);
}

async function run() {
  let browser;
  let api;
  let mediaSourceId = null;
  let originalSettings = null;

  try {
    await ensureDirs();

    browser = await chromium.launch({ headless: true });
    const context = await browser.newContext({ viewport: { width: 1560, height: 980 } });
    const page = await context.newPage();

    await page.goto(`${FRONTEND_URL}/login`, { waitUntil: "networkidle" });
    await page.locator('input[placeholder*="用户名"], input[type="text"]').first().fill("admin");
    await page.locator('input[type="password"]').first().fill("admin");
    await page.getByRole("button", { name: "登录" }).click();
    await page.waitForURL("**/dashboard/**", { timeout: 15000 });
    addCase("TC-NAMING-AUTH-001", "登录成功", "PASS", "", await saveShot(page, "01_login"));

    const token = await page.evaluate(() => localStorage.getItem("token"));
    if (!token) {
      throw new Error("登录后未获取 token");
    }

    api = await request.newContext({
      baseURL: BACKEND_API,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` }
    });

    const settingsResp = await api.get("/settings");
    originalSettings = unwrapData(await apiJson(settingsResp)) || {};

    await page.goto(`${FRONTEND_URL}/dashboard/settings`, { waitUntil: "networkidle" });
    await page.getByRole("heading", { name: "系统配置", exact: true }).waitFor({ timeout: 15000 });

    const movieInput = page.locator(".n-form-item").filter({ hasText: "电影命名模板" }).locator("input").first();
    const tvInput = page.locator(".n-form-item").filter({ hasText: "电视剧命名模板" }).locator("input").first();
    await movieInput.fill(MOVIE_TEMPLATE);
    await tvInput.fill(TV_TEMPLATE);
    await page.getByRole("button", { name: "保存配置" }).last().click();
    await waitForSuccessMessage(page, "配置保存成功");
    const settingsShot = await saveShot(page, "02_settings_saved");

    const updatedSettingsResp = await api.get("/settings");
    const updatedSettings = unwrapData(await apiJson(updatedSettingsResp)) || {};
    const settingsPass = updatedSettings.movie_naming_template === MOVIE_TEMPLATE
      && updatedSettings.tv_naming_template === TV_TEMPLATE;
    addCase(
      "TC-NAMING-SET-001",
      "设置页保存电影与电视剧命名模板",
      settingsPass ? "PASS" : "FAIL",
      `movie=${updatedSettings.movie_naming_template || ""} tv=${updatedSettings.tv_naming_template || ""}`,
      settingsShot
    );

    const mediaSourceName = `naming_source_${stamp}`;
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
    const createSourceData = unwrapData(await apiJson(createSourceResp)) || {};
    mediaSourceId = createSourceData.id || null;
    if (!createSourceResp.ok() || !mediaSourceId) {
      throw new Error(`创建测试媒体源失败: ${JSON.stringify(createSourceData)}`);
    }
    addCase("TC-NAMING-SRC-001", "创建本地模板测试媒体源成功", "PASS", `id=${mediaSourceId}`);

    const browserDialog = await openSourceBrowser(page, mediaSourceName);
    addCase("TC-NAMING-UI-001", "文件浏览器打开成功", "PASS", "", await saveShot(page, "03_browser_open"));

    const tvRenameDialog = await triggerBatchRenameForFile(page, browserDialog, "Template.Show.S01E02.1080p.mkv");
    const tvPreviewText = await tvRenameDialog.textContent();
    const tvPreviewPass = (tvPreviewText || "").includes("Template Show - S01E02 - TV.mkv");
    addCase(
      "TC-NAMING-TV-001",
      "电视剧批量重命名预览命中电视剧模板",
      tvPreviewPass ? "PASS" : "FAIL",
      (tvPreviewText || "").replace(/\s+/g, " ").slice(0, 180),
      await saveShot(page, "04_tv_preview")
    );
    await executeRenameAndWait(page, tvRenameDialog);

    const filesAfterTvResp = await api.get(`/media/files?source_id=${mediaSourceId}&path=/&page=1&page_size=100`);
    const filesAfterTv = unwrapData(await apiJson(filesAfterTvResp)) || {};
    const tvFileNames = (filesAfterTv.files || []).map((item) => item.name || "");
    addCase(
      "TC-NAMING-TV-002",
      "电视剧批量重命名执行后文件名正确落盘",
      tvFileNames.includes("Template Show - S01E02 - TV.mkv") ? "PASS" : "FAIL",
      `files=${tvFileNames.join(",")}`,
      await saveShot(page, "05_tv_done")
    );

    await browserDialog.locator("button").filter({ hasText: "刷新" }).click();
    await page.waitForTimeout(800);
    const movieRenameDialog = await triggerBatchRenameForFile(page, browserDialog, "Template.Movie.2024.1080p.mkv");
    const moviePreviewText = await movieRenameDialog.textContent();
    const moviePreviewPass = (moviePreviewText || "").includes("Template Movie 2024 - MOVIE.mkv");
    addCase(
      "TC-NAMING-MOVIE-001",
      "电影批量重命名预览命中电影模板",
      moviePreviewPass ? "PASS" : "FAIL",
      (moviePreviewText || "").replace(/\s+/g, " ").slice(0, 180),
      await saveShot(page, "06_movie_preview")
    );
    await executeRenameAndWait(page, movieRenameDialog);

    const filesAfterMovieResp = await api.get(`/media/files?source_id=${mediaSourceId}&path=/&page=1&page_size=100`);
    const filesAfterMovie = unwrapData(await apiJson(filesAfterMovieResp)) || {};
    const movieFileNames = (filesAfterMovie.files || []).map((item) => item.name || "");
    addCase(
      "TC-NAMING-MOVIE-002",
      "电影批量重命名执行后文件名正确落盘",
      movieFileNames.includes("Template Movie 2024 - MOVIE.mkv") ? "PASS" : "FAIL",
      `files=${movieFileNames.join(",")}`,
      await saveShot(page, "07_movie_done")
    );

    const tmdbConfigResp = await api.get("/media/tmdb/config");
    const tmdbConfig = unwrapData(await apiJson(tmdbConfigResp)) || {};
    if (tmdbConfig.has_key) {
      const tmdbMovieResp = await api.post("/media/organize/rename-preview", {
        data: {
          source_id: mediaSourceId,
          file_id: "Inception.2010.1080p.mkv",
          tmdb_id: 27205,
          media_type: "movie",
          template: TMDB_MOVIE_TEMPLATE
        }
      });
      const tmdbMovieData = unwrapData(await apiJson(tmdbMovieResp)) || {};
      const movieTmdbPass = typeof tmdbMovieData.new_name === "string"
        && tmdbMovieData.new_name.endsWith(".mkv")
        && tmdbMovieData.new_name.includes("2010")
        && !tmdbMovieData.new_name.includes("..");
      addCase(
        "TC-NAMING-TMDB-001",
        "TMDB 电影预览可稳定带出年份",
        movieTmdbPass ? "PASS" : "FAIL",
        tmdbMovieData.new_name || JSON.stringify(tmdbMovieData)
      );

      const tmdbTvResp = await api.post("/media/organize/rename-preview", {
        data: {
          source_id: mediaSourceId,
          file_id: "Breaking.Bad.S01E02.1080p.mkv",
          tmdb_id: 1396,
          template: TMDB_TV_TEMPLATE
        }
      });
      const tmdbTvData = unwrapData(await apiJson(tmdbTvResp)) || {};
      const tvTmdbPass = tmdbTvData.media_type === "tv"
        && typeof tmdbTvData.new_name === "string"
        && tmdbTvData.new_name.includes("S01E02")
        && tmdbTvData.new_name.endsWith(".mkv");
      addCase(
        "TC-NAMING-TMDB-002",
        "未显式传媒体类型时 TMDB 电视剧预览仍命中剧集链路",
        tvTmdbPass ? "PASS" : "FAIL",
        tmdbTvData.new_name || JSON.stringify(tmdbTvData)
      );
    } else {
      addCase("TC-NAMING-TMDB-001", "TMDB 电影预览可稳定带出年份", "SKIP", "当前环境未配置可用 TMDB API Key");
      addCase("TC-NAMING-TMDB-002", "未显式传媒体类型时 TMDB 电视剧预览仍命中剧集链路", "SKIP", "当前环境未配置可用 TMDB API Key");
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

    const lines = [];
    lines.push("# 命名模板深挖测试报告");
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
    addCase("TC-NAMING-RUN-000", "命名模板深挖执行异常", "FAIL", String(error));
    const lines = [];
    lines.push("# 命名模板深挖测试报告");
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
      if (originalSettings) {
        await api.put("/settings", {
          data: {
            movie_naming_template: originalSettings.movie_naming_template || "",
            tv_naming_template: originalSettings.tv_naming_template || ""
          }
        }).catch(() => {});
      }
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
