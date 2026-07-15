import fs from "node:fs/promises";
import path from "node:path";
import crypto from "node:crypto";
import { chromium, request } from "playwright";

const FRONTEND_URL = process.env.E2E_FRONTEND_URL || "http://localhost:3001";
const BACKEND_API = process.env.E2E_BACKEND_API || "http://localhost:8082";
const ROOT_DIR = path.resolve(process.cwd(), "..");
const DEBUG_DIR = path.join(ROOT_DIR, "debug");
const SCREENSHOT_DIR = path.join(DEBUG_DIR, "e2e-screenshots", "organize-scrape");
const REPORT_DIR = path.join(ROOT_DIR, "docs");

const now = new Date();
const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, "0")}${String(now.getDate()).padStart(2, "0")}_${String(now.getHours()).padStart(2, "0")}${String(now.getMinutes()).padStart(2, "0")}${String(now.getSeconds()).padStart(2, "0")}`;
const runId = `organize_scrape_${stamp}`;
const fixtureRoot = path.join(DEBUG_DIR, "e2e-fixtures", runId);
const fixtureSource = path.join(fixtureRoot, "source");
const fixtureTarget = path.join(fixtureRoot, "target");
const runScreenshotDir = path.join(SCREENSHOT_DIR, runId);
const runJsonFile = path.join(REPORT_DIR, `organize_scrape_e2e_${stamp}.json`);

const movieFileName = "Inception.2010.1080p.mkv";
const sourceName = `e2e_organize_scrape_${stamp}`;
const standardMovieTemplate = "{{ title }}{% if year %} ({{ year }}){% endif %}/{{ title }}{% if year %} ({{ year }}){% endif %}{{ fileExt }}";

async function ensureFixtures() {
  await fs.mkdir(fixtureSource, { recursive: true });
  await fs.mkdir(fixtureTarget, { recursive: true });
  await fs.mkdir(runScreenshotDir, { recursive: true });
  await fs.writeFile(path.join(fixtureSource, movieFileName), "e2e movie fixture\n", "utf8");
  await fs.writeFile(path.join(fixtureSource, "Ignore.txt"), "ignore\n", "utf8");
}

async function saveShot(page, name) {
  const file = path.join(runScreenshotDir, `${name}.png`);
  await page.screenshot({ path: file, fullPage: true });
  return file;
}

function fail(message) {
  throw new Error(message);
}

function normalizeData(payload) {
  if (!payload || typeof payload !== "object") return payload;
  return "data" in payload ? payload.data : payload;
}

async function loginByApi(page) {
  const authApi = await request.newContext({ baseURL: BACKEND_API });
  try {
    const passwordMd5 = crypto.createHash("md5").update("admin").digest("hex");
    const resp = await authApi.post("/login", {
      data: { name: "admin", password: passwordMd5 }
    });
    if (!resp.ok()) {
      fail(`登录失败: HTTP ${resp.status()} ${await resp.text()}`);
    }
    const data = await resp.json();
    if (!data?.token) {
      fail("登录成功但未获取到 token");
    }

    await page.goto(`${FRONTEND_URL}/login`, { waitUntil: "networkidle" });
    await page.evaluate((payload) => {
      localStorage.setItem("token", payload.token);
      localStorage.setItem("user_id", String(payload.user_id ?? ""));
      localStorage.setItem("user_name", payload.name ?? "admin");
      document.cookie = `token=${payload.token}; path=/`;
    }, data);

    return data.token;
  } finally {
    await authApi.dispose().catch(() => {});
  }
}

async function collectProducedFiles(rootDir) {
  const producedFiles = [];

  async function walk(dir) {
    const entries = await fs.readdir(dir, { withFileTypes: true });
    for (const entry of entries) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        await walk(full);
      } else {
        producedFiles.push(path.relative(rootDir, full).replaceAll("\\", "/"));
      }
    }
  }

  await walk(rootDir);
  producedFiles.sort();
  return producedFiles;
}

async function main() {
  let browser;
  let api;
  let sourceId = null;
  let existingSettings = {};
  const artifacts = {};

  try {
    await ensureFixtures();

    browser = await chromium.launch({ headless: true });
    const context = await browser.newContext({ viewport: { width: 1600, height: 1000 } });
    const page = await context.newPage();

    const token = await loginByApi(page);
    artifacts.login = await saveShot(page, "01_login");

    api = await request.newContext({
      baseURL: BACKEND_API,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` }
    });

    const backupResp = await api.get("/settings");
    existingSettings = normalizeData(await backupResp.json()) || {};

    const settingsPayload = {
      scrape_enabled_on_organize: "true",
      scrape_write_nfo: "true",
      scrape_write_poster: "true",
      scrape_write_fanart: "true",
      scrape_write_thumb: "true",
      movie_naming_template: standardMovieTemplate
    };
    const settingsResp = await api.put("/settings", { data: settingsPayload });
    if (!settingsResp.ok()) {
      fail(`更新系统配置失败: HTTP ${settingsResp.status()} ${await settingsResp.text()}`);
    }

    const createResp = await api.post("/media/sources", {
      data: {
        name: sourceName,
        source_type: "local",
        path: fixtureSource,
        organize_target_path: fixtureTarget,
        auto_organize: false,
        watch_enabled: false,
        watch_interval: 300,
        emby_library_id: "",
        enabled: true,
        priority: 10
      }
    });
    const createData = await createResp.json();
    if (!createResp.ok()) {
      fail(`创建媒体源失败: ${JSON.stringify(createData)}`);
    }
    sourceId = normalizeData(createData)?.id || normalizeData(createData)?.data?.id || null;
    if (!sourceId) fail(`创建媒体源后缺少 sourceId: ${JSON.stringify(createData)}`);

    await page.goto(`${FRONTEND_URL}/dashboard/media-manager`, { waitUntil: "networkidle" });
    artifacts.mediaManager = await saveShot(page, "02_media_manager");

    const sourceRow = page.locator("tbody tr").filter({ hasText: sourceName }).first();
    await sourceRow.waitFor({ timeout: 15000 });
    await sourceRow.locator("button").first().click();

    const browserDialog = page.getByRole("dialog").filter({ hasText: "文件浏览" });
    await browserDialog.waitFor({ timeout: 15000 });
    artifacts.fileBrowser = await saveShot(page, "03_file_browser");

    const movieRow = browserDialog.locator("tbody tr").filter({ hasText: movieFileName }).first();
    await movieRow.waitFor({ timeout: 15000 });
    await movieRow.locator('[role="checkbox"]').first().click();
    artifacts.fileSelected = await saveShot(page, "04_file_selected");

    await browserDialog.getByRole("button", { name: /批量整理/ }).click();

    const organizeDialog = page.getByRole("dialog").filter({ hasText: "批量整理工作流" });
    await organizeDialog.waitFor({ timeout: 15000 });
    artifacts.organizeDialog = await saveShot(page, "05_organize_dialog");

    const previewButton = organizeDialog.getByRole("button", { name: "刷新预览" });
    await previewButton.click();

    await page.waitForFunction(() => {
      const dialogs = Array.from(document.querySelectorAll('[role="dialog"]'));
      const dialog = dialogs.find((item) => item.textContent?.includes("批量整理工作流"));
      if (!dialog) return false;
      const loadingMask = dialog.querySelector('[aria-busy="true"]');
      const headers = Array.from(dialog.querySelectorAll("table th")).map((item) => (item.textContent || "").trim());
      const hasPreviewHeaders = headers.length >= 5;
      const hasCandidateInfo = (dialog.textContent || "").includes("Inception");
      return !loadingMask && hasPreviewHeaders && hasCandidateInfo;
    }, { timeout: 120000 });
    artifacts.previewDone = await saveShot(page, "06_preview_done");

    const executeButton = organizeDialog.getByRole("button", { name: "执行整理" });
    await executeButton.click();

    const resultDialog = page.getByRole("dialog").filter({ hasText: "整理结果" });
    await resultDialog.waitFor({ timeout: 120000 });
    artifacts.resultDialog = await saveShot(page, "07_result_dialog");

    const producedFiles = await collectProducedFiles(fixtureTarget);
    const requiredPatterns = [
      { label: "organized_video", test: (file) => file.endsWith(".mkv") },
      { label: "nfo", test: (file) => file.endsWith(".nfo") },
      { label: "poster_sidecar", test: (file) => file.endsWith("-poster.jpg") },
      { label: "fanart_sidecar", test: (file) => file.endsWith("-fanart.jpg") },
      { label: "poster_generic", test: (file) => file.endsWith("/poster.jpg") || file === "poster.jpg" },
      { label: "fanart_generic", test: (file) => file.endsWith("/fanart.jpg") || file === "fanart.jpg" }
    ];

    const missing = requiredPatterns
      .filter((rule) => !producedFiles.some((file) => rule.test(file)))
      .map((rule) => rule.label);
    const report = {
      ok: missing.length === 0,
      runId,
      sourceId,
      sourceName,
      fixtureSource,
      fixtureTarget,
      requiredPatterns: requiredPatterns.map((rule) => rule.label),
      producedFiles,
      missingChecks: missing,
      screenshots: artifacts,
      timestamp: new Date().toISOString()
    };

    await fs.mkdir(REPORT_DIR, { recursive: true });
    await fs.writeFile(runJsonFile, JSON.stringify(report, null, 2), "utf8");

    if (missing.length > 0) {
      fail(`整理后刮削产物缺失: ${missing.join(", ")}`);
    }

    console.log(JSON.stringify(report, null, 2));
  } finally {
    if (api && sourceId) {
      await api.delete(`/media/sources/${sourceId}`).catch(() => {});
    }
    if (api) {
      const restorePayload = {
        scrape_enabled_on_organize: existingSettings.scrape_enabled_on_organize || "",
        scrape_write_nfo: existingSettings.scrape_write_nfo || "",
        scrape_write_poster: existingSettings.scrape_write_poster || "",
        scrape_write_fanart: existingSettings.scrape_write_fanart || "",
        scrape_write_thumb: existingSettings.scrape_write_thumb || "",
        movie_naming_template: existingSettings.movie_naming_template || ""
      };
      await api.put("/settings", { data: restorePayload }).catch(() => {});
      await api.dispose().catch(() => {});
    }
    if (browser) {
      await browser.close();
    }
  }
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
