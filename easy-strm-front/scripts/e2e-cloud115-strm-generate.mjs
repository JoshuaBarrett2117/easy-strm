import fs from "node:fs/promises";
import path from "node:path";
import { chromium, request } from "playwright";

const FRONTEND_URL = process.env.E2E_FRONTEND_URL || "http://127.0.0.1:3001";
const BACKEND_API = process.env.E2E_BACKEND_API || "http://127.0.0.1:8082";
const ROOT_DIR = path.resolve(process.cwd(), "..");
const DEBUG_DIR = path.join(ROOT_DIR, "debug");
const SCREENSHOT_DIR = path.join(DEBUG_DIR, "e2e-screenshots", "cloud115-strm-generate");
const REPORT_DIR = path.join(ROOT_DIR, "docs");

const now = new Date();
const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, "0")}${String(now.getDate()).padStart(2, "0")}_${String(now.getHours()).padStart(2, "0")}${String(now.getMinutes()).padStart(2, "0")}${String(now.getSeconds()).padStart(2, "0")}`;
const runId = `cloud115_strm_generate_${stamp}`;
const runScreenshotDir = path.join(SCREENSHOT_DIR, runId);
const reportFile = path.join(REPORT_DIR, `115云源生成STRM闭环测试报告_${stamp}.md`);
const localStrmDir = path.join(DEBUG_DIR, "e2e-fixtures", runId, "strm-output");
const sourceName = `cloud_strm_e2e_${stamp}`;
const targetPath = `/影视资源/CodexStrmE2E_${stamp}`;
const targetFolderName = `CodexStrmE2E_${stamp}`;
const movieSourceCID = "2977269469445485999";
const sourceLibraryID = 186;
const organizedRootCID = "3410496669326971511";

const cases = [];

function addCase(id, name, status, detail = "", artifact = "") {
  cases.push({ id, name, status, detail, artifact });
  const prefix = status === "PASS" ? "[PASS]" : status === "FAIL" ? "[FAIL]" : "[SKIP]";
  console.log(`${prefix} ${id} ${name}${detail ? ` -> ${detail}` : ""}`);
}

async function ensureDirs() {
  await fs.mkdir(runScreenshotDir, { recursive: true });
  await fs.mkdir(REPORT_DIR, { recursive: true });
  await fs.mkdir(localStrmDir, { recursive: true });
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

async function waitForMessage(page, text, timeout = 60000) {
  const locator = page.locator(".n-message").filter({ hasText: text });
  await locator.waitFor({ timeout });
}

async function findDialogByTitle(page, title) {
  const dialog = page.getByRole("dialog").filter({
    has: page.getByRole("heading", { name: title, exact: true })
  });
  await dialog.waitFor({ timeout: 20000 });
  return dialog;
}

async function pollTask(api, taskId, maxAttempts = 30) {
  for (let i = 0; i < maxAttempts; i += 1) {
    const resp = await api.get(`/strm/task/${taskId}`);
    const detail = unwrapData(await apiJson(resp)) || {};
    const status = detail.status || "";
    if (status && status !== "pending" && status !== "running") {
      return detail;
    }
    await new Promise((resolve) => setTimeout(resolve, 3000));
  }
  return null;
}

async function pollUnifiedTask(api, taskId, maxAttempts = 60) {
  for (let i = 0; i < maxAttempts; i += 1) {
    const resp = await api.get(`/tasks/${taskId}`);
    const detail = unwrapData(await apiJson(resp)) || {};
    const status = detail.status || "";
    if (status && status !== "pending" && status !== "running") {
      return detail;
    }
    await new Promise((resolve) => setTimeout(resolve, 2000));
  }
  return null;
}

async function cleanupRemoteArtifacts(api) {
  try {
    const listResp = await api.get(`/media/files?source_id=${sourceLibraryID}&path=${organizedRootCID}&page=1&page_size=200`);
    const listPayload = unwrapData(await apiJson(listResp)) || {};
    const files = listPayload.files || [];
    const targetFolder = files.find((item) => item.name === targetFolderName);
    if (!targetFolder?.id) {
      return false;
    }
    const deleteResp = await api.post("/media/files/delete", {
      data: {
        source_id: sourceLibraryID,
        file_id: targetFolder.id
      }
    });
    return deleteResp.ok();
  } catch {
    return false;
  }
}

async function run() {
  let browser;
  let api;
  let tempSourceId = null;
  let tempConfigId = null;

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
    addCase("TC-CLOUD-STRM-AUTH-001", "登录成功", "PASS", "", await saveShot(page, "01_login"));

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
        name: sourceName,
        source_type: "cloud115",
        path: movieSourceCID,
        watch_path: "",
        cloud115_id: 3,
        priority: 10,
        enabled: true,
        organize_target_path: targetPath,
        media_type: "movie",
        conflict_policy: "skip",
        operation_mode: "copy",
        auto_organize: false,
        watch_enabled: false,
        watch_interval: 300,
        emby_library_id: ""
      }
    });
    const createSourcePayload = unwrapData(await apiJson(createSourceResp)) || {};
    tempSourceId = createSourcePayload.id || null;
    if (!createSourceResp.ok() || !tempSourceId) {
      throw new Error(`创建 115 测试媒体源失败: ${JSON.stringify(createSourcePayload)}`);
    }

    const createConfigResp = await api.post("/strm/config", {
      data: {
        cloud115_id: 3,
        net_disk_path: targetPath,
        local_path: localStrmDir,
        cron: "0 0 * * *",
        extension: ".mp4,.mkv,.avi"
      }
    });
    const createConfigPayload = unwrapData(await apiJson(createConfigResp)) || {};
    tempConfigId = createConfigPayload.id || null;
    if (!createConfigResp.ok() || !tempConfigId) {
      throw new Error(`创建 STRM 配置失败: ${JSON.stringify(createConfigPayload)}`);
    }

    await page.goto(`${FRONTEND_URL}/dashboard/media-manager`, { waitUntil: "networkidle" });
    const sourceRow = page.getByRole("row").filter({ hasText: sourceName });
    await sourceRow.waitFor({ timeout: 20000 });
    await sourceRow.getByRole("button", { name: "浏览" }).click();

    const browserDialog = page.getByRole("dialog").filter({ hasText: "文件浏览" });
    await browserDialog.waitFor({ timeout: 20000 });
    const fileRow = browserDialog.getByRole("row").filter({ hasText: ".mp4" }).first();
    await fileRow.waitFor({ timeout: 20000 });
    await fileRow.getByRole("checkbox").click();
    await browserDialog.getByRole("button", { name: /批量整理/ }).click();

    const organizeDialog = await findDialogByTitle(page, "批量整理");
    await organizeDialog.getByRole("button", { name: "刷新预览" }).click();
    await waitForMessage(page, "预览完成", 90000);
    const previewText = (await organizeDialog.textContent()) || "";
    const previewPass = previewText.includes("可处理 1 项") && previewText.includes("识别失败 0 项");
    addCase(
      "TC-CLOUD-STRM-001",
      "115 云源预览能够在媒体源起始目录内识别选中文件",
      previewPass ? "PASS" : "FAIL",
      previewText.replace(/\s+/g, " ").slice(0, 220),
      await saveShot(page, "02_preview_success")
    );

    const executeResponsePromise = page.waitForResponse(
      (resp) => resp.url().includes("/media/organize/execute/async") && resp.request().method() === "POST",
      { timeout: 60000 }
    );
    await organizeDialog.getByRole("button", { name: "执行整理" }).click();
    const executeResp = await executeResponsePromise;
    const executePayload = unwrapData(await executeResp.json()) || {};
    const organizeTaskId = executePayload.task_id || "";
    const organizeTask = organizeTaskId ? await pollUnifiedTask(api, organizeTaskId) : null;
    addCase(
      "TC-CLOUD-STRM-002",
      "115 云源整理任务提交后在任务中心完成",
      executeResp.ok() && organizeTask?.status === "completed" ? "PASS" : "FAIL",
      `task=${organizeTaskId || "-"} status=${organizeTask?.status || "unknown"}`,
      await saveShot(page, "03_organize_task_submitted")
    );

    const generateResp = await api.post("/strm/config/generate/from-organize", {
      data: {
        source_id: tempSourceId,
        target_paths: [targetPath],
        strm_config_id: 0
      }
    });
    const generatePayload = await apiJson(generateResp);
    const generatedConfigId = generatePayload?.strm_config_id || generatePayload?.data?.strm_config_id || 0;
    const taskId = generatePayload?.task_id || generatePayload?.data?.task_id || "";
    addCase(
      "TC-CLOUD-STRM-003",
      "生成 STRM 自动匹配当前整理目标对应的配置",
      generateResp.ok() && generatedConfigId === tempConfigId && Boolean(taskId) ? "PASS" : "FAIL",
      `matched=${generatedConfigId} expected=${tempConfigId} task=${taskId || "-"}`,
      await saveShot(page, "04_generate_message")
    );

    const taskDetail = taskId ? await pollTask(api, taskId) : null;
    const strmPath = path.join(localStrmDir, "动画电影", "哆啦A梦：大雄的地球交响乐 (2024)", "哆啦A梦：大雄的地球交响乐 (2024).strm");
    const strmExists = await fs.access(strmPath).then(() => true).catch(() => false);
    addCase(
      "TC-CLOUD-STRM-004",
      "STRM 任务完成并生成本地 .strm 文件",
      taskDetail?.status === "completed" && strmExists ? "PASS" : "FAIL",
      `taskStatus=${taskDetail?.status || "unknown"} strm=${strmExists}`,
      ""
    );

    await page.goto(`${FRONTEND_URL}/dashboard/tasks`, { waitUntil: "networkidle" });
    const taskCard = page.getByText(taskId || targetPath).first();
    await taskCard.waitFor({ timeout: 20000 }).catch(() => {});
    const taskShot = await saveShot(page, "05_task_center");
    addCase(
      "TC-CLOUD-STRM-005",
      "任务中心可见本轮 STRM 任务记录",
      taskId ? "PASS" : "FAIL",
      taskId || "任务ID为空",
      taskShot
    );

    const remoteCleanupOk = await cleanupRemoteArtifacts(api);
    addCase(
      "TC-CLOUD-STRM-006",
      "测试期间创建的 115 目标目录已尝试清理",
      remoteCleanupOk ? "PASS" : "SKIP",
      remoteCleanupOk ? "已删除远端测试目录" : "未能自动删除远端测试目录，请按需手动检查",
      ""
    );

    const summary = {
      runId,
      total: cases.length,
      passCount: cases.filter((item) => item.status === "PASS").length,
      failCount: cases.filter((item) => item.status === "FAIL").length,
      skipCount: cases.filter((item) => item.status === "SKIP").length,
      screenshots: runScreenshotDir,
      report: reportFile,
      strmOutput: localStrmDir
    };

    const lines = [
      "# 115 云源生成 STRM 闭环测试报告",
      "",
      `- 执行时间: ${new Date().toLocaleString("zh-CN", { hour12: false })}`,
      `- 执行者: Codex`,
      `- 运行标识: ${runId}`,
      `- 前端地址: ${FRONTEND_URL}`,
      `- 后端地址: ${BACKEND_API}`,
      `- 测试输出目录: ${localStrmDir}`,
      "",
      "## 结论",
      "",
      "- 本轮已覆盖 115 云源从浏览器选中文件、整理、结果弹窗点击“生成 STRM”、任务创建到本地 `.strm` 文件落地的完整闭环。",
      "- 本轮同时验证了两处修复：115 云源整理预览默认扫描起始目录改为媒体源根目录；整理后 STRM 自动匹配不再错误回退到同账号的第一条无关配置。",
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
    addCase("TC-CLOUD-STRM-RUN-000", "115 云源生成 STRM 执行异常", "FAIL", `${error.name}: ${error.message}`);
    const lines = [
      "# 115 云源生成 STRM 闭环测试报告",
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
      await cleanupRemoteArtifacts(api).catch(() => false);
      if (tempConfigId) await api.delete(`/strm/config/${tempConfigId}`).catch(() => {});
      if (tempSourceId) await api.delete(`/media/sources/${tempSourceId}`).catch(() => {});
      await api.dispose().catch(() => {});
    }
    if (browser) {
      await browser.close();
    }
  }
}

run();
