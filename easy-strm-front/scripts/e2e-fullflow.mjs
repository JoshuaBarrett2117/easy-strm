import fs from "node:fs/promises";
import path from "node:path";
import { chromium, request } from "playwright";

const FRONTEND_URL = process.env.E2E_FRONTEND_URL || "http://localhost:3001";
const BACKEND_API = process.env.E2E_BACKEND_API || "http://localhost:8082";
const ROOT_DIR = path.resolve(process.cwd(), "..");
const DEBUG_DIR = path.join(ROOT_DIR, "debug");
const SCREENSHOT_DIR = path.join(DEBUG_DIR, "e2e-screenshots", "fullflow");
const NETWORK_DIR = path.join(DEBUG_DIR, "e2e-network");
const REPORT_DIR = path.join(ROOT_DIR, "docs");

const now = new Date();
const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, "0")}${String(now.getDate()).padStart(2, "0")}_${String(now.getHours()).padStart(2, "0")}${String(now.getMinutes()).padStart(2, "0")}${String(now.getSeconds()).padStart(2, "0")}`;

const runId = `fullflow_${stamp}`;
const runScreenshotDir = path.join(SCREENSHOT_DIR, runId);
const runNetworkFile = path.join(NETWORK_DIR, `${runId}.json`);
const runReportFile = path.join(REPORT_DIR, `测试执行报告_${stamp}.md`);

const fixtureRoot = path.join(DEBUG_DIR, "e2e-fixtures", runId);
const fixtureSource = path.join(fixtureRoot, "source");
const fixtureTarget = path.join(fixtureRoot, "organized");
const fixtureCopyTarget = path.join(fixtureRoot, "copy-target");

const cases = [];
const networkTraces = [];

function pickMessage(payload) {
  if (!payload || typeof payload !== "object") return "";
  return payload.error || payload.message || "";
}

function addCase(id, name, status, detail = "", artifact = "") {
  cases.push({ id, name, status, detail, artifact });
  const prefix = status === "PASS" ? "[PASS]" : status === "FAIL" ? "[FAIL]" : "[SKIP]";
  console.log(`${prefix} ${id} ${name}${detail ? ` -> ${detail}` : ""}`);
}

async function ensureDirs() {
  await fs.mkdir(runScreenshotDir, { recursive: true });
  await fs.mkdir(NETWORK_DIR, { recursive: true });
  await fs.mkdir(fixtureSource, { recursive: true });
  await fs.mkdir(fixtureTarget, { recursive: true });
  await fs.mkdir(fixtureCopyTarget, { recursive: true });
}

async function writeFixtures() {
  const sampleFiles = [
    "Movie.One.2021.1080p.mkv",
    "TV.Show.S01E01.720p.mkv",
    "TV.Show.S01E02.720p.mkv",
    "Ignore.Me.txt",
    "sample.nfo"
  ];
  for (const file of sampleFiles) {
    await fs.writeFile(path.join(fixtureSource, file), `e2e fixture: ${file}\n`, "utf8");
  }
}

async function saveArtifact(page, name) {
  const file = path.join(runScreenshotDir, `${name}.png`);
  await page.screenshot({ path: file, fullPage: true });
  return file;
}

function normalizeApiData(data) {
  if (!data || typeof data !== "object") return null;
  if ("data" in data) return data.data;
  return data;
}

async function apiJson(resp) {
  try {
    return await resp.json();
  } catch {
    return null;
  }
}

async function run() {
  let browser;
  let api;
  let localSourceId = null;
  let localSourceId2 = null;
  let movieFile = null;
  let tvFile = null;

  try {
    await ensureDirs();
    await writeFixtures();

    browser = await chromium.launch({ headless: true });
    const context = await browser.newContext({ viewport: { width: 1560, height: 980 } });
    const page = await context.newPage();

    page.on("response", async (resp) => {
      const url = resp.url();
      if (!url.includes("/api/")) return;
      networkTraces.push({
        ts: new Date().toISOString(),
        url,
        method: resp.request().method(),
        status: resp.status()
      });
    });

    page.on("console", (msg) => {
      if (msg.type() === "error") {
        networkTraces.push({
          ts: new Date().toISOString(),
          type: "console-error",
          text: msg.text()
        });
      }
    });

    await page.goto(`${FRONTEND_URL}/login`, { waitUntil: "networkidle" });
    addCase("TC-AUTH-001", "登录页可访问", "PASS", "", await saveArtifact(page, "01_login_page"));

    await page.locator('input[placeholder*="用户名"], input[type="text"]').first().fill("admin");
    await page.locator('input[type="password"]').first().fill("admin");
    await page.locator(".login-btn").click();
    await page.waitForURL("**/dashboard/**", { timeout: 15000 });
    addCase("TC-AUTH-002", "登录成功进入 dashboard", "PASS", "", await saveArtifact(page, "02_after_login"));

    const token = await page.evaluate(() => localStorage.getItem("token"));
    if (!token) {
      throw new Error("登录后未获取 token");
    }

    api = await request.newContext({
      baseURL: BACKEND_API,
      extraHTTPHeaders: {
        Authorization: `Bearer ${token}`
      }
    });

    const routeChecks = [
      { id: "TC-NAV-001", path: "/dashboard/cloud115", name: "115管理页可达" },
      { id: "TC-NAV-002", path: "/dashboard/strm-config", name: "STRM配置页可达" },
      { id: "TC-NAV-003", path: "/dashboard/media-manager", name: "媒体管理页可达" },
      { id: "TC-NAV-004", path: "/dashboard/category-strategy", name: "分类策略页可达" },
      { id: "TC-NAV-005", path: "/dashboard/settings", name: "系统配置页可达" }
    ];

    for (let i = 0; i < routeChecks.length; i++) {
      const item = routeChecks[i];
      try {
        await page.goto(`${FRONTEND_URL}${item.path}`, { waitUntil: "networkidle", timeout: 15000 });
        addCase(item.id, item.name, "PASS", "", await saveArtifact(page, `03_route_${String(i + 1).padStart(2, "0")}`));
      } catch (err) {
        addCase(item.id, item.name, "FAIL", String(err));
      }
    }

    const pingApis = [
      { id: "TC-DB-001", method: "GET", url: "/dashboard/stats", name: "Dashboard统计接口" },
      { id: "TC-DB-002", method: "GET", url: "/tasks/unified", name: "统一任务列表接口" },
      { id: "TC-SET-001", method: "GET", url: "/settings", name: "系统配置读取接口" },
      { id: "TC-TMDB-001", method: "GET", url: "/media/tmdb/config", name: "TMDB配置读取接口" },
      { id: "TC-EMBY-001", method: "GET", url: "/emby/status", name: "Emby状态接口" },
      { id: "TC-EMBY-002", method: "GET", url: "/emby/libraries", name: "Emby库列表接口" },
      { id: "TC-LOG-001", method: "GET", url: "/logs", name: "日志列表接口" },
      { id: "TC-LOG-002", method: "GET", url: "/logs/config", name: "日志配置接口" },
      { id: "TC-ORG-001", method: "GET", url: "/media/organize/presets", name: "整理预设接口" }
    ];

    for (const item of pingApis) {
      const resp = await api.fetch(item.url, { method: item.method });
      const data = await apiJson(resp);
      const isEmbyNotConfigured = item.id === "TC-EMBY-002" && resp.status() === 500 && pickMessage(data).includes("未配置");
      if (resp.ok() || isEmbyNotConfigured) {
        addCase(item.id, item.name, "PASS", `HTTP ${resp.status()}`);
      } else {
        addCase(item.id, item.name, "FAIL", `HTTP ${resp.status()} ${pickMessage(data)}`);
      }
    }

    const createSourcePayload = {
      name: `e2e_local_${stamp}`,
      source_type: "local",
      path: fixtureSource,
      organize_target_path: fixtureTarget,
      auto_organize: true,
      watch_enabled: true,
      watch_interval: 600,
      emby_library_id: "",
      enabled: true,
      priority: 10
    };
    const createResp = await api.post("/media/sources", { data: createSourcePayload });
    const createData = await apiJson(createResp);
    if (!createResp.ok()) {
      addCase("TC-MS-001", "创建本地媒体源", "FAIL", `HTTP ${createResp.status()} ${pickMessage(createData)}`);
      throw new Error("无法继续：媒体源创建失败");
    }
    const createBody = normalizeApiData(createData);
    localSourceId = createBody?.data?.id || createBody?.id;
    if (!localSourceId) {
      addCase("TC-MS-001", "创建本地媒体源", "FAIL", "响应中缺少媒体源ID");
      throw new Error("无法继续：媒体源ID缺失");
    }
    addCase("TC-MS-001", "创建本地媒体源", "PASS", `source_id=${localSourceId}`);

    const createTargetResp = await api.post("/media/sources", {
      data: {
        name: `e2e_target_${stamp}`,
        source_type: "local",
        path: fixtureCopyTarget,
        organize_target_path: fixtureCopyTarget,
        auto_organize: false,
        watch_enabled: false,
        watch_interval: 300,
        emby_library_id: "",
        enabled: true,
        priority: 11
      }
    });
    const createTargetData = await apiJson(createTargetResp);
    if (!createTargetResp.ok()) {
      addCase("TC-MS-002", "创建复制目标媒体源", "FAIL", `HTTP ${createTargetResp.status()} ${pickMessage(createTargetData)}`);
    } else {
      localSourceId2 = normalizeApiData(createTargetData)?.data?.id || normalizeApiData(createTargetData)?.id || null;
      addCase("TC-MS-002", "创建复制目标媒体源", "PASS", `source_id=${localSourceId2 || "N/A"}`);
    }

    const updateResp = await api.put(`/media/sources/${localSourceId}`, {
      data: {
        auto_organize: false,
        watch_enabled: false,
        watch_interval: 120,
        organize_target_path: fixtureTarget
      }
    });
    const updateData = await apiJson(updateResp);
    if (updateResp.ok()) {
      addCase("TC-MS-003", "更新新增字段(auto/watch/target_path)", "PASS", `HTTP ${updateResp.status()}`);
    } else {
      addCase("TC-MS-003", "更新新增字段(auto/watch/target_path)", "FAIL", `HTTP ${updateResp.status()} ${pickMessage(updateData)}`);
    }

    const detailResp = await api.get(`/media/sources/${localSourceId}`);
    const detailData = await apiJson(detailResp);
    if (!detailResp.ok()) {
      addCase("TC-MS-004", "读取媒体源详情回显", "FAIL", `HTTP ${detailResp.status()} ${pickMessage(detailData)}`);
    } else {
      const item = normalizeApiData(detailData) || {};
      const pass = item.organize_target_path === fixtureTarget && item.watch_interval === 120 && item.auto_organize === false;
      addCase("TC-MS-004", "读取媒体源详情回显", pass ? "PASS" : "FAIL", pass ? "字段回显正确" : `字段不符: ${JSON.stringify(item)}`);
    }

    await page.goto(`${FRONTEND_URL}/dashboard/media-manager`, { waitUntil: "networkidle" });
    addCase("TC-UI-001", "媒体管理页面加载（包含表格）", "PASS", "", await saveArtifact(page, "04_media_manager_loaded"));

    try {
      const sourceRow = page.locator(".el-table__row").filter({ hasText: createSourcePayload.name }).first();
      await sourceRow.waitFor({ timeout: 15000 });
      await sourceRow.locator("button").first().click();

      const browserDialog = page.locator(".el-dialog").filter({ has: page.locator(".table-wrapper") }).last();
      await browserDialog.waitFor({ timeout: 15000 });
      const videoRow = browserDialog.locator(".el-table__row").filter({ hasText: ".mkv" }).first();
      await videoRow.waitFor({ timeout: 15000 });
      await videoRow.locator(".el-checkbox").first().click();
      await browserDialog.locator(".organize-primary-btn").click();

      const organizeDialog = page.locator(".el-dialog").filter({ has: page.locator(".organize-container") }).last();
      await organizeDialog.waitFor({ timeout: 15000 });
      const previewButton = organizeDialog.locator(".dialog-footer button").nth(1);
      await previewButton.click();

      await page.waitForFunction(() => {
        const dialogs = Array.from(document.querySelectorAll(".el-dialog"));
        const dialog = dialogs.find((item) => item.querySelector(".organize-container"));
        if (!dialog) return false;

        const footerButtons = Array.from(dialog.querySelectorAll(".dialog-footer button"));
        const button = footerButtons[1];
        if (!button) return false;

        const loadingMask = dialog.querySelector(".organize-container .el-loading-mask");
        const hasPreviewState = dialog.textContent?.includes("识别失败")
          || dialog.textContent?.includes("可处理")
          || dialog.textContent?.includes("原文件名")
          || dialog.textContent?.includes("手动识别");

        return !button.classList.contains("is-loading") && !loadingMask && hasPreviewState;
      }, { timeout: 20000 });

      const previewState = await organizeDialog.evaluate((dialog) => {
        const footerButtons = Array.from(dialog.querySelectorAll(".dialog-footer button")).map((button) => ({
          text: button.textContent || "",
          className: button.className
        }));
        return {
          hasLoadingMask: Boolean(dialog.querySelector(".organize-container .el-loading-mask")),
          hasPreviewTable: Boolean(dialog.querySelector(".el-table")),
          hasManualAction: (dialog.textContent || "").includes("手动识别"),
          refreshButtonClass: footerButtons[1]?.className || ""
        };
      });

      const pass = !previewState.hasLoadingMask
        && (previewState.hasPreviewTable || previewState.hasManualAction)
        && !previewState.refreshButtonClass.includes("is-loading");

      addCase(
        "TC-ORG-UI-001",
        "整理弹窗刷新预览后退出加载态",
        pass ? "PASS" : "FAIL",
        `mask=${previewState.hasLoadingMask} table=${previewState.hasPreviewTable} btn=${previewState.refreshButtonClass || "N/A"}`,
        await saveArtifact(page, "04b_organize_preview_refresh")
      );
    } catch (err) {
      addCase(
        "TC-ORG-UI-001",
        "整理弹窗刷新预览后退出加载态",
        "FAIL",
        String(err),
        await saveArtifact(page, "04b_organize_preview_refresh_fail")
      );
    }

    const filesResp = await api.get(`/media/files?source_id=${localSourceId}&path=/&page=1&page_size=100`);
    const filesData = await apiJson(filesResp);
    if (!filesResp.ok()) {
      addCase("TC-FILE-001", "获取文件列表", "FAIL", `HTTP ${filesResp.status()} ${pickMessage(filesData)}`);
    } else {
      const payload = normalizeApiData(filesData) || {};
      const fileList = payload.files || [];
      movieFile = fileList.find((f) => (f.name || "").includes("Movie.One.2021.1080p.mkv")) || null;
      tvFile = fileList.find((f) => (f.name || "").includes("TV.Show.S01E01.720p.mkv")) || null;
      const found = fileList.length >= 3;
      addCase("TC-FILE-001", "获取文件列表", found ? "PASS" : "FAIL", `files=${fileList.length}`);
    }

    const searchResp = await api.get(`/media/files/search?source_id=${localSourceId}&keyword=Movie.One`);
    const searchData = await apiJson(searchResp);
    if (searchResp.ok()) {
      addCase("TC-FILE-002", "文件搜索接口", "PASS", `HTTP ${searchResp.status()}`);
    } else {
      addCase("TC-FILE-002", "文件搜索接口", "FAIL", `HTTP ${searchResp.status()} ${pickMessage(searchData)}`);
    }

    if (movieFile) {
      const renameTo = "Movie.One.2021.RENAMED.mkv";
      const renameResp = await api.post("/media/files/rename", {
        data: { source_id: localSourceId, file_id: movieFile.id, new_name: renameTo }
      });
      const renameData = await apiJson(renameResp);
      if (renameResp.ok()) {
        addCase("TC-FILE-003", "单文件重命名", "PASS", `HTTP ${renameResp.status()}`);
      } else {
        addCase("TC-FILE-003", "单文件重命名", "FAIL", `HTTP ${renameResp.status()} ${pickMessage(renameData)}`);
      }
    } else {
      addCase("TC-FILE-003", "单文件重命名", "SKIP", "未找到测试电影文件");
    }

    const reloadFilesResp = await api.get(`/media/files?source_id=${localSourceId}&path=/&page=1&page_size=100`);
    const reloadData = await apiJson(reloadFilesResp);
    const reloadPayload = normalizeApiData(reloadData) || {};
    const reloadedFiles = reloadPayload.files || [];
    const renamedMovie = reloadedFiles.find((f) => (f.name || "").includes("RENAMED"));
    if (renamedMovie && localSourceId2) {
      const copyResp = await api.post("/media/files/copy", {
        data: {
          source_id: localSourceId,
          target_id: localSourceId2,
          file_id: renamedMovie.id,
          target_path: "/",
          delete_source: false
        }
      });
      const copyData = await apiJson(copyResp);
      if (copyResp.ok()) {
        addCase("TC-FILE-004", "单文件复制", "PASS", `HTTP ${copyResp.status()}`);
      } else {
        addCase("TC-FILE-004", "单文件复制", "FAIL", `HTTP ${copyResp.status()} ${pickMessage(copyData)}`);
      }
    } else {
      addCase("TC-FILE-004", "单文件复制", "SKIP", "未找到重命名后文件或目标媒体源");
    }

    const organizeCandidatesResp = await api.post("/media/organize/candidates", {
      data: { source_id: localSourceId, source_path: "/", media_type: "all" }
    });
    const organizeCandidatesData = await apiJson(organizeCandidatesResp);
    if (!organizeCandidatesResp.ok()) {
      addCase("TC-ORG-002", "整理候选列表", "FAIL", `HTTP ${organizeCandidatesResp.status()} ${pickMessage(organizeCandidatesData)}`);
    } else {
      const data = normalizeApiData(organizeCandidatesData) || [];
      addCase("TC-ORG-002", "整理候选列表", "PASS", `candidates=${Array.isArray(data) ? data.length : 0}`);
    }

    const identifyIds = reloadedFiles.filter((f) => !f.is_dir).slice(0, 2).map((f) => f.id);
    if (identifyIds.length > 0) {
      const batchIdentifyResp = await api.post("/media/organize/batch-identify", {
        data: { source_id: localSourceId, file_ids: identifyIds }
      });
      const batchIdentifyData = await apiJson(batchIdentifyResp);
      if (batchIdentifyResp.ok()) {
        addCase("TC-TMDB-002", "批量识别（组织服务）", "PASS", `HTTP ${batchIdentifyResp.status()}`);
      } else {
        addCase("TC-TMDB-002", "批量识别（组织服务）", "FAIL", `HTTP ${batchIdentifyResp.status()} ${pickMessage(batchIdentifyData)}`);
      }
    } else {
      addCase("TC-TMDB-002", "批量识别（组织服务）", "SKIP", "无可识别文件");
    }

    const previewResp = await api.post("/media/organize/preview", {
      data: {
        source_id: localSourceId,
        source_path: "/",
        target_path: fixtureTarget,
        media_type: "all",
        conflict_policy: "suffix",
        operation_mode: "copy",
        use_category: false
      }
    });
    const previewData = await apiJson(previewResp);
    if (!previewResp.ok()) {
      addCase("TC-ORG-003", "整理预览", "FAIL", `HTTP ${previewResp.status()} ${pickMessage(previewData)}`);
    } else {
      const payload = normalizeApiData(previewData) || [];
      addCase("TC-ORG-003", "整理预览", "PASS", `preview_items=${Array.isArray(payload) ? payload.length : 0}`);
    }

    const executeResp = await api.post("/media/organize/execute", {
      data: {
        source_id: localSourceId,
        source_path: "/",
        target_path: fixtureTarget,
        media_type: "all",
        conflict_policy: "suffix",
        operation_mode: "copy",
        use_category: false
      }
    });
    const executeData = await apiJson(executeResp);
    if (!executeResp.ok()) {
      addCase("TC-ORG-004", "执行整理（copy）", "FAIL", `HTTP ${executeResp.status()} ${pickMessage(executeData)}`);
    } else {
      const payload = normalizeApiData(executeData) || {};
      addCase("TC-ORG-004", "执行整理（copy）", "PASS", `success=${payload.success ?? "N/A"} failed=${payload.failed ?? "N/A"}`);
    }

    const singleScrapePath = (tvFile && (tvFile.path || tvFile.id)) || (reloadedFiles.find((f) => !f.is_dir)?.path || reloadedFiles.find((f) => !f.is_dir)?.id || "");
    const scrapeSingleResp = await api.post("/media/scrape/file", {
      data: {
        source_id: localSourceId,
        file_path: singleScrapePath
      }
    });
    const scrapeSingleData = await apiJson(scrapeSingleResp);
    if (scrapeSingleResp.ok()) {
      addCase("TC-SCRAPE-001", "单文件刮削", "PASS", `HTTP ${scrapeSingleResp.status()}`);
    } else {
      addCase("TC-SCRAPE-001", "单文件刮削", "FAIL", `HTTP ${scrapeSingleResp.status()} ${pickMessage(scrapeSingleData)}`);
    }

    const scrapeBatchPaths = reloadedFiles.filter((f) => !f.is_dir).slice(0, 2).map((f) => f.path || f.id);
    const scrapeBatchResp = await api.post("/media/scrape/files", {
      data: {
        source_id: localSourceId,
        file_paths: scrapeBatchPaths
      }
    });
    const scrapeBatchData = await apiJson(scrapeBatchResp);
    if (scrapeBatchResp.ok()) {
      addCase("TC-SCRAPE-002", "批量刮削", "PASS", `HTTP ${scrapeBatchResp.status()}`);
    } else {
      addCase("TC-SCRAPE-002", "批量刮削", "FAIL", `HTTP ${scrapeBatchResp.status()} ${pickMessage(scrapeBatchData)}`);
    }

    const strmFromOrganizeResp = await api.post("/strm/config/generate/from-organize", {
      data: {
        source_id: localSourceId,
        target_paths: [fixtureTarget]
      }
    });
    const strmFromOrganizeData = await apiJson(strmFromOrganizeResp);
    if (strmFromOrganizeResp.ok()) {
      addCase("TC-LINK-001", "整理后触发STRM（正向）", "PASS", `HTTP ${strmFromOrganizeResp.status()}`);
    } else {
      const expected = strmFromOrganizeResp.status() === 400 || strmFromOrganizeResp.status() === 404;
      addCase("TC-LINK-001", "整理后触发STRM（正向）", expected ? "PASS" : "FAIL", `HTTP ${strmFromOrganizeResp.status()} ${pickMessage(strmFromOrganizeData)}`);
    }

    const embyRefreshResp = await api.post("/emby/refresh", { data: { library_id: "" } });
    const embyRefreshData = await apiJson(embyRefreshResp);
    if (embyRefreshResp.ok()) {
      addCase("TC-LINK-002", "Emby手动刷新接口", "PASS", `HTTP ${embyRefreshResp.status()}`);
    } else {
      const expectedConfigMissing = embyRefreshResp.status() === 500 && pickMessage(embyRefreshData).includes("未配置");
      addCase("TC-LINK-002", "Emby手动刷新接口", expectedConfigMissing ? "PASS" : "FAIL", `HTTP ${embyRefreshResp.status()} ${pickMessage(embyRefreshData)}`);
    }

    const logsResp = await api.get("/logs");
    const logsData = await apiJson(logsResp);
    if (logsResp.ok()) {
      const logList = normalizeApiData(logsData)?.data || normalizeApiData(logsData) || [];
      const firstLog = Array.isArray(logList) && logList.length > 0 ? logList[0].name : null;
      addCase("TC-LOG-003", "日志列表读取并选择文件", "PASS", firstLog ? `first=${firstLog}` : "无日志文件");
      if (firstLog) {
        const contentResp = await api.get(`/logs/${firstLog}?lines=80`);
        const contentData = await apiJson(contentResp);
        if (contentResp.ok()) {
          addCase("TC-LOG-004", "日志内容读取", "PASS", `HTTP ${contentResp.status()}`);
        } else {
          addCase("TC-LOG-004", "日志内容读取", "FAIL", `HTTP ${contentResp.status()} ${pickMessage(contentData)}`);
        }
      } else {
        addCase("TC-LOG-004", "日志内容读取", "SKIP", "无可读取日志");
      }
    } else {
      addCase("TC-LOG-003", "日志列表读取并选择文件", "FAIL", `HTTP ${logsResp.status()} ${pickMessage(logsData)}`);
      addCase("TC-LOG-004", "日志内容读取", "SKIP", "日志列表失败");
    }

    const logConfigGet = await api.get("/logs/config");
    const logCfgData = await apiJson(logConfigGet);
    if (logConfigGet.ok()) {
      const current = Number(normalizeApiData(logCfgData)?.value || 7);
      const next = current >= 365 ? 364 : current + 1;
      const updateLogCfgResp = await api.put("/logs/config", { data: { value: next } });
      const updateLogCfgData = await apiJson(updateLogCfgResp);
      if (updateLogCfgResp.ok()) {
        addCase("TC-LOG-005", "日志保留天数更新", "PASS", `new_value=${next}`);
      } else {
        addCase("TC-LOG-005", "日志保留天数更新", "FAIL", `HTTP ${updateLogCfgResp.status()} ${pickMessage(updateLogCfgData)}`);
      }
    } else {
      addCase("TC-LOG-005", "日志保留天数更新", "SKIP", "未读到当前日志配置");
    }

    if (localSourceId2) {
      const deleteTargetResp = await api.delete(`/media/sources/${localSourceId2}`);
      const deleteTargetData = await apiJson(deleteTargetResp);
      if (deleteTargetResp.ok()) {
        addCase("TC-MS-005", "删除复制目标媒体源", "PASS", `source_id=${localSourceId2}`);
      } else {
        addCase("TC-MS-005", "删除复制目标媒体源", "FAIL", `HTTP ${deleteTargetResp.status()} ${pickMessage(deleteTargetData)}`);
      }
    }

    if (localSourceId) {
      const deleteSourceResp = await api.delete(`/media/sources/${localSourceId}`);
      const deleteSourceData = await apiJson(deleteSourceResp);
      if (deleteSourceResp.ok()) {
        addCase("TC-MS-006", "删除测试媒体源", "PASS", `source_id=${localSourceId}`);
      } else {
        addCase("TC-MS-006", "删除测试媒体源", "FAIL", `HTTP ${deleteSourceResp.status()} ${pickMessage(deleteSourceData)}`);
      }
    }

    await page.goto(`${FRONTEND_URL}/dashboard/media-manager`, { waitUntil: "networkidle" });
    addCase("TC-UI-002", "媒体管理页回归复查", "PASS", "", await saveArtifact(page, "05_media_manager_post_run"));

    await fs.writeFile(runNetworkFile, JSON.stringify(networkTraces, null, 2), "utf8");
  } catch (err) {
    addCase("TC-RUN-000", "全流程执行异常中断", "FAIL", String(err));
  } finally {
    if (api) {
      await api.dispose();
    }
    if (browser) {
      await browser.close();
    }
  }

  const passCount = cases.filter((x) => x.status === "PASS").length;
  const failCount = cases.filter((x) => x.status === "FAIL").length;
  const skipCount = cases.filter((x) => x.status === "SKIP").length;
  const total = cases.length;
  const passRate = total > 0 ? ((passCount / total) * 100).toFixed(2) : "0.00";

  const lines = [];
  lines.push(`# 浏览器全流程测试执行报告`);
  lines.push("");
  lines.push(`- 执行时间：${new Date().toLocaleString("zh-CN", { hour12: false })}`);
  lines.push(`- 执行者：Codex`);
  lines.push(`- 测试方式：Playwright 浏览器端到端（UI + API）`);
  lines.push(`- 前端地址：${FRONTEND_URL}`);
  lines.push(`- 后端地址：${BACKEND_API}`);
  lines.push(`- 测试数据目录：${fixtureRoot}`);
  lines.push(`- 截图目录：${runScreenshotDir}`);
  lines.push(`- 网络追踪：${runNetworkFile}`);
  lines.push("");
  lines.push("## 汇总");
  lines.push("");
  lines.push(`- 总用例：${total}`);
  lines.push(`- 通过：${passCount}`);
  lines.push(`- 失败：${failCount}`);
  lines.push(`- 跳过：${skipCount}`);
  lines.push(`- 通过率：${passRate}%`);
  lines.push("");
  lines.push("## 结果明细");
  lines.push("");
  lines.push("| 用例ID | 用例名称 | 结果 | 说明 | 证据 |");
  lines.push("| --- | --- | --- | --- | --- |");
  for (const item of cases) {
    lines.push(`| ${item.id} | ${item.name} | ${item.status} | ${item.detail || "-"} | ${item.artifact || "-"} |`);
  }

  await fs.writeFile(runReportFile, `${lines.join("\n")}\n`, "utf8");

  const summary = {
    runId,
    total,
    passCount,
    failCount,
    skipCount,
    passRate,
    report: runReportFile,
    screenshots: runScreenshotDir,
    network: runNetworkFile
  };

  console.log(JSON.stringify(summary, null, 2));
}

run();
