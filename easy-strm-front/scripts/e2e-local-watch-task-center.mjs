import fs from "node:fs/promises";
import path from "node:path";
import { chromium, request } from "playwright";

const FRONTEND_URL = process.env.E2E_FRONTEND_URL || "http://127.0.0.1:3001";
const BACKEND_API = process.env.E2E_BACKEND_API || "http://127.0.0.1:8082";
const ROOT_DIR = path.resolve(process.cwd(), "..");
const DEBUG_DIR = path.join(ROOT_DIR, "debug");
const SCREENSHOT_DIR = path.join(DEBUG_DIR, "e2e-screenshots", "local-watch-task-center");
const REPORT_DIR = path.join(ROOT_DIR, "docs");

const now = new Date();
const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, "0")}${String(now.getDate()).padStart(2, "0")}_${String(now.getHours()).padStart(2, "0")}${String(now.getMinutes()).padStart(2, "0")}${String(now.getSeconds()).padStart(2, "0")}`;
const runId = `local_watch_${stamp}`;
const runScreenshotDir = path.join(SCREENSHOT_DIR, runId);
const reportFile = path.join(REPORT_DIR, `本地监控自动整理任务中心测试报告_${stamp}.md`);
const fixtureRoot = path.join(DEBUG_DIR, "e2e-fixtures", runId);
const watchDir = path.join(fixtureRoot, "watch");
const targetDir = path.join(fixtureRoot, "target");
const mediaSourceName = `local_watch_${stamp}`;
const injectedFileName = "Inception.2010.1080p.mkv";

const cases = [];

function addCase(id, name, status, detail = "", artifact = "") {
  cases.push({ id, name, status, detail, artifact });
  const prefix = status === "PASS" ? "[PASS]" : status === "FAIL" ? "[FAIL]" : "[SKIP]";
  console.log(`${prefix} ${id} ${name}${detail ? ` -> ${detail}` : ""}`);
}

function unwrapData(payload) {
  if (!payload || typeof payload !== "object") return payload;
  if ("data" in payload) return unwrapData(payload.data);
  return payload;
}

async function apiJson(resp) {
  try {
    return await resp.json();
  } catch {
    return null;
  }
}

async function ensureDirs() {
  await fs.mkdir(runScreenshotDir, { recursive: true });
  await fs.mkdir(REPORT_DIR, { recursive: true });
  await fs.mkdir(watchDir, { recursive: true });
  await fs.mkdir(targetDir, { recursive: true });
}

async function saveShot(page, name) {
  const file = path.join(runScreenshotDir, `${name}.png`);
  await page.screenshot({ path: file, fullPage: true });
  return file;
}

async function waitForUrl(page, urlPart, timeout = 30000) {
  await page.waitForURL((url) => url.toString().includes(urlPart), { timeout });
}

async function getUnifiedTasks(api) {
  const resp = await api.get("/tasks/unified");
  const payload = unwrapData(await apiJson(resp)) || [];
  return Array.isArray(payload) ? payload : [];
}

async function pollWatchTask(api, sourceName, timeoutMs = 120000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const tasks = await getUnifiedTasks(api);
    const task = tasks.find((item) => item.task_type === "watch_auto_organize" && item.metadata?.source_name === sourceName);
    if (task) {
      return task;
    }
    await new Promise((resolve) => setTimeout(resolve, 3000));
  }
  return null;
}

async function pollWatchTaskFinished(api, taskId, timeoutMs = 180000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const resp = await api.get(`/tasks/${encodeURIComponent(taskId)}`);
    const payload = unwrapData(await apiJson(resp)) || {};
    if (payload?.status && !["pending", "running"].includes(payload.status)) {
      return payload;
    }
    await new Promise((resolve) => setTimeout(resolve, 3000));
  }
  return null;
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
    await waitForUrl(page, "/dashboard");
    addCase("TC-LOCAL-WATCH-AUTH-001", "登录成功", "PASS", "", await saveShot(page, "01_login"));

    const token = await page.evaluate(() => localStorage.getItem("token"));
    if (!token) {
      throw new Error("登录后未获取 token");
    }
    api = await request.newContext({
      baseURL: BACKEND_API,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` }
    });

    const createSourceResp = await api.post("/media/sources", {
      data: {
        name: mediaSourceName,
        source_type: "local",
        path: watchDir,
        watch_path: watchDir,
        priority: 10,
        enabled: true,
        organize_target_path: targetDir,
        media_type: "movie",
        conflict_policy: "skip",
        operation_mode: "move",
        auto_organize: true,
        watch_enabled: true,
        watch_interval: 60,
        emby_library_id: ""
      }
    });
    const createSourceData = unwrapData(await apiJson(createSourceResp)) || {};
    mediaSourceId = createSourceData.id || null;
    if (!createSourceResp.ok() || !mediaSourceId) {
      throw new Error(`创建本地 watch 测试媒体源失败: ${JSON.stringify(createSourceData)}`);
    }
    addCase("TC-LOCAL-WATCH-SETUP-001", "创建本地 watch 自动整理媒体源成功", "PASS", `sourceId=${mediaSourceId}`);

    await page.goto(`${FRONTEND_URL}/dashboard/media-manager`, { waitUntil: "networkidle" });
    const sourceRow = page.locator(".el-table__row").filter({ hasText: mediaSourceName }).first();
    await sourceRow.waitFor({ timeout: 20000 });
    const sourceRowText = (await sourceRow.textContent()) || "";
    const watchEnabledVisible = sourceRowText.includes(mediaSourceName);
    addCase(
      "TC-LOCAL-WATCH-UI-001",
      "媒体源列表可见新建的 watch 自动整理媒体源",
      watchEnabledVisible ? "PASS" : "FAIL",
      sourceRowText.replace(/\s+/g, " ").slice(0, 220),
      await saveShot(page, "02_media_source")
    );

    const injectedPath = path.join(watchDir, injectedFileName);
    await fs.writeFile(injectedPath, "watch trigger fixture\n", "utf8");
    addCase("TC-LOCAL-WATCH-TRIGGER-001", "向监控目录注入新视频文件", "PASS", injectedPath);

    const watchTask = await pollWatchTask(api, mediaSourceName, 90000);
    addCase(
      "TC-LOCAL-WATCH-001",
      "检测到新增文件后会创建 watch_auto_organize 任务",
      watchTask ? "PASS" : "FAIL",
      watchTask ? `taskId=${watchTask.task_id} status=${watchTask.status}` : "超时未发现 watch_auto_organize 任务",
      ""
    );
    if (!watchTask?.task_id) {
      throw new Error("未检测到 watch_auto_organize 任务");
    }

    const finishedTask = await pollWatchTaskFinished(api, watchTask.task_id, 180000);
    addCase(
      "TC-LOCAL-WATCH-002",
      "watch_auto_organize 任务最终进入终态",
      finishedTask ? "PASS" : "FAIL",
      finishedTask ? `status=${finishedTask.status} success=${finishedTask.success_files || 0} failed=${finishedTask.failed_files || 0}` : "超时未进入终态",
      ""
    );
    if (!finishedTask?.status) {
      throw new Error("watch_auto_organize 任务未进入终态");
    }

    await page.goto(`${FRONTEND_URL}/dashboard/tasks`, { waitUntil: "networkidle" });
    const taskCard = page.locator(".task-card").filter({ hasText: mediaSourceName }).first();
    await taskCard.waitFor({ timeout: 20000 });
    const taskCardText = (await taskCard.textContent()) || "";
    const taskCardPass = taskCardText.includes("本地实时监控") && taskCardText.includes("处理文件") && taskCardText.includes("本地自动整理");
    addCase(
      "TC-LOCAL-WATCH-003",
      "任务中心卡片展示监控来源、触发方式与处理统计",
      taskCardPass ? "PASS" : "FAIL",
      taskCardText.replace(/\s+/g, " ").slice(0, 260),
      await saveShot(page, "03_task_center_card")
    );

    await taskCard.locator(".el-button").filter({ hasText: "详情" }).click();
    const drawer = page.locator(".el-drawer").filter({ hasText: "任务详情" }).last();
    await drawer.waitFor({ timeout: 20000 });
    const drawerText = (await drawer.textContent()) || "";
    const detailPass = drawerText.includes("来源媒体源") && drawerText.includes(mediaSourceName) && drawerText.includes("触发方式");
    addCase(
      "TC-LOCAL-WATCH-004",
      "任务详情展示 watch 元数据",
      detailPass ? "PASS" : "FAIL",
      drawerText.replace(/\s+/g, " ").slice(0, 320),
      await saveShot(page, "04_task_detail")
    );

    const targetFiles = await listFilesRecursive(targetDir);
    const movedFile = targetFiles.find((item) => item.endsWith(".mkv"));
    addCase(
      "TC-LOCAL-WATCH-005",
      "自动整理会把新文件整理到目标目录",
      Boolean(movedFile) ? "PASS" : "FAIL",
      `files=${targetFiles.join(",")}`,
      ""
    );

    const watchDirFiles = await listFilesRecursive(watchDir);
    const sourceCleaned = !watchDirFiles.includes(injectedFileName);
    addCase(
      "TC-LOCAL-WATCH-006",
      "move 模式下监控目录内原文件被移走",
      sourceCleaned ? "PASS" : "FAIL",
      `watchFiles=${watchDirFiles.join(",") || "<empty>"}`,
      await saveShot(page, "05_after_organize")
    );

    const passCount = cases.filter((item) => item.status === "PASS").length;
    const failCount = cases.filter((item) => item.status === "FAIL").length;
    const skipCount = cases.filter((item) => item.status === "SKIP").length;

    const lines = [];
    lines.push("# 本地监控自动整理任务中心测试报告");
    lines.push("");
    lines.push(`- 执行时间：${new Date().toLocaleString("zh-CN", { hour12: false })}`);
    lines.push("- 执行者：Codex");
    lines.push(`- 前端地址：${FRONTEND_URL}`);
    lines.push(`- 后端地址：${BACKEND_API}`);
    lines.push(`- 测试媒体源：${mediaSourceName}`);
    lines.push(`- 监控目录：${watchDir}`);
    lines.push(`- 目标目录：${targetDir}`);
    lines.push("");
    lines.push("## 结论");
    lines.push("");
    lines.push(`- 通过：${passCount}`);
    lines.push(`- 失败：${failCount}`);
    lines.push(`- 跳过：${skipCount}`);
    lines.push(`- 总计：${cases.length}`);
    lines.push("");
    lines.push("## 用例明细");
    lines.push("");
    for (const item of cases) {
      lines.push(`### ${item.id} ${item.name}`);
      lines.push(`- 结果：${item.status}`);
      if (item.detail) lines.push(`- 说明：${item.detail}`);
      if (item.artifact) lines.push(`- 截图：${item.artifact}`);
      lines.push("");
    }
    lines.push("## 文件结果");
    lines.push("");
    lines.push(`- 目标目录文件：${(await listFilesRecursive(targetDir)).join(",") || "<empty>"}`);
    lines.push(`- 监控目录剩余文件：${(await listFilesRecursive(watchDir)).join(",") || "<empty>"}`);
    lines.push("");
    lines.push("## 截图目录");
    lines.push("");
    lines.push(`- ${runScreenshotDir}`);
    lines.push("");
    await fs.writeFile(reportFile, `${lines.join("\n")}\n`, "utf8");
    console.log(`REPORT=${reportFile}`);
    console.log(`SCREENSHOTS=${runScreenshotDir}`);

    if (failCount > 0) {
      process.exitCode = 1;
    }
  } catch (error) {
    addCase("TC-LOCAL-WATCH-RUN-000", "本地 watch 任务中心深挖执行异常", "FAIL", String(error));
    const lines = [];
    lines.push("# 本地监控自动整理任务中心测试报告");
    lines.push("");
    lines.push(`- 执行时间：${new Date().toLocaleString("zh-CN", { hour12: false })}`);
    lines.push("- 执行者：Codex");
    lines.push("");
    lines.push("## 异常");
    lines.push("");
    for (const item of cases) {
      lines.push(`- ${item.id} ${item.name}: ${item.status}${item.detail ? ` -> ${item.detail}` : ""}`);
    }
    lines.push("");
    await fs.writeFile(reportFile, `${lines.join("\n")}\n`, "utf8");
    console.error(error);
    process.exitCode = 1;
  } finally {
    if (api && mediaSourceId) {
      try {
        await api.delete(`/media/sources/${mediaSourceId}`);
      } catch {}
    }
    if (api) {
      try {
        await api.dispose();
      } catch {}
    }
    if (browser) {
      try {
        await browser.close();
      } catch {}
    }
  }
}

await run();
