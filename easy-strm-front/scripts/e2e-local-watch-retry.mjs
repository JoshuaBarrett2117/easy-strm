import fs from "node:fs/promises";
import path from "node:path";
import { chromium, request } from "playwright";

const FRONTEND_URL = process.env.E2E_FRONTEND_URL || "http://127.0.0.1:3001";
const BACKEND_API = process.env.E2E_BACKEND_API || "http://127.0.0.1:8082";
const ROOT_DIR = path.resolve(process.cwd(), "..");
const DEBUG_DIR = path.join(ROOT_DIR, "debug");
const SCREENSHOT_DIR = path.join(DEBUG_DIR, "e2e-screenshots", "local-watch-retry");
const REPORT_DIR = path.join(ROOT_DIR, "docs");

const now = new Date();
const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, "0")}${String(now.getDate()).padStart(2, "0")}_${String(now.getHours()).padStart(2, "0")}${String(now.getMinutes()).padStart(2, "0")}${String(now.getSeconds()).padStart(2, "0")}`;
const runId = `local_watch_retry_${stamp}`;
const runScreenshotDir = path.join(SCREENSHOT_DIR, runId);
const reportFile = path.join(REPORT_DIR, `本地监控失败任务重试测试报告_${stamp}.md`);
const fixtureRoot = path.join(DEBUG_DIR, "e2e-fixtures", runId);
const watchDir = path.join(fixtureRoot, "watch");
const fixedTargetDir = path.join(fixtureRoot, "target-fixed");
const invalidTargetDir = path.join(fixtureRoot, "target<invalid");
const mediaSourceName = `local_watch_retry_${stamp}`;
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
  await fs.mkdir(fixedTargetDir, { recursive: true });
}

async function saveShot(page, name) {
  const file = path.join(runScreenshotDir, `${name}.png`);
  await page.screenshot({ path: file, fullPage: true });
  return file;
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
    if (task?.status && !["pending", "running"].includes(task.status)) {
      return task;
    }
    await new Promise((resolve) => setTimeout(resolve, 3000));
  }
  return null;
}

async function pollTaskById(api, taskId, timeoutMs = 180000) {
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
    await page.getByRole("button", { name: "登录" }).click();
    await page.waitForURL(/\/dashboard(\/|$)/, { timeout: 30000, waitUntil: "commit" });
    addCase("TC-LOCAL-WATCH-RETRY-AUTH-001", "登录成功", "PASS", "", await saveShot(page, "01_login"));

    const token = await page.evaluate(() => localStorage.getItem("token"));
    if (!token) throw new Error("登录后未获取 token");
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
        organize_target_path: invalidTargetDir,
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
      throw new Error(`创建本地 watch 重试测试媒体源失败: ${JSON.stringify(createSourceData)}`);
    }
    addCase("TC-LOCAL-WATCH-RETRY-SETUP-001", "创建带无效目标路径的监控源成功", "PASS", `sourceId=${mediaSourceId}`);

    const injectedPath = path.join(watchDir, injectedFileName);
    await fs.writeFile(injectedPath, "watch retry fixture\n", "utf8");
    addCase("TC-LOCAL-WATCH-RETRY-TRIGGER-001", "注入可识别的监控文件", "PASS", injectedPath);

    const failedTask = await pollWatchTask(api, mediaSourceName, 120000);
    const failedReason = failedTask?.error_message || failedTask?.metadata?.failure_reason || "";
    addCase(
      "TC-LOCAL-WATCH-RETRY-001",
      "无效目标路径会先产生失败任务",
      failedTask?.status === "failed" ? "PASS" : "FAIL",
      failedTask ? `taskId=${failedTask.task_id} reason=${failedReason}` : "超时未发现失败任务",
      ""
    );
    if (!failedTask?.task_id) {
      throw new Error("未获取到失败任务");
    }

    await page.goto(`${FRONTEND_URL}/dashboard/tasks`, { waitUntil: "networkidle" });
    const failedCard = page.getByTestId("task-card").filter({ hasText: mediaSourceName }).first();
    await failedCard.waitFor({ timeout: 20000 });
    addCase(
      "TC-LOCAL-WATCH-RETRY-002",
      "失败任务卡片可见恢复任务按钮",
      await failedCard.locator("button").filter({ hasText: "恢复任务" }).count() > 0 ? "PASS" : "FAIL",
      ((await failedCard.textContent()) || "").replace(/\s+/g, " ").slice(0, 260),
      await saveShot(page, "02_failed_card")
    );

    const updateResp = await api.put(`/media/sources/${mediaSourceId}`, {
      data: {
        organize_target_path: fixedTargetDir
      }
    });
    if (!updateResp.ok()) {
      throw new Error(`修复目标路径失败: ${JSON.stringify(await apiJson(updateResp))}`);
    }
    addCase("TC-LOCAL-WATCH-RETRY-003", "通过接口修复媒体源目标路径", "PASS", fixedTargetDir);

    await failedCard.locator("button").filter({ hasText: "恢复任务" }).click();
    await page.waitForTimeout(1500);
    addCase("TC-LOCAL-WATCH-RETRY-004", "在任务中心触发失败任务重试", "PASS", failedTask.task_id, await saveShot(page, "03_retry_clicked"));

    const retriedTask = await pollTaskById(api, failedTask.task_id, 180000);
    addCase(
      "TC-LOCAL-WATCH-RETRY-005",
      "重试后的同一任务可以恢复为成功",
      retriedTask?.status === "completed" ? "PASS" : "FAIL",
      retriedTask ? `status=${retriedTask.status} success=${retriedTask.success_files || 0} failed=${retriedTask.failed_files || 0}` : "超时未完成",
      ""
    );
    if (!retriedTask) {
      throw new Error("重试任务未进入终态");
    }

    await page.reload({ waitUntil: "networkidle" });
    const successCard = page.getByTestId("task-card").filter({ hasText: mediaSourceName }).first();
    await successCard.waitFor({ timeout: 20000 });
    const successCardText = (await successCard.textContent()) || "";
    addCase(
      "TC-LOCAL-WATCH-RETRY-006",
      "任务中心会展示重试后的成功结果",
      successCardText.includes("已完成") && successCardText.includes("1/1 成功") ? "PASS" : "FAIL",
      successCardText.replace(/\s+/g, " ").slice(0, 300),
      await saveShot(page, "04_success_card")
    );

    const targetFiles = await listFilesRecursive(fixedTargetDir);
    addCase(
      "TC-LOCAL-WATCH-RETRY-007",
      "重试成功后文件会落到修复后的目标目录",
      targetFiles.some((item) => item.endsWith(".mkv")) ? "PASS" : "FAIL",
      `files=${targetFiles.join(",")}`,
      ""
    );

    const passCount = cases.filter((item) => item.status === "PASS").length;
    const failCount = cases.filter((item) => item.status === "FAIL").length;
    const skipCount = cases.filter((item) => item.status === "SKIP").length;

    const lines = [];
    lines.push("# 本地监控失败任务重试测试报告");
    lines.push("");
    lines.push(`- 执行时间：${new Date().toLocaleString("zh-CN", { hour12: false })}`);
    lines.push("- 执行者：Codex");
    lines.push(`- 前端地址：${FRONTEND_URL}`);
    lines.push(`- 后端地址：${BACKEND_API}`);
    lines.push(`- 测试媒体源：${mediaSourceName}`);
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
    lines.push("## 目标目录文件");
    lines.push("");
    lines.push(`- ${targetFiles.join(",") || "<empty>"}`);
    lines.push("");
    lines.push("## 截图目录");
    lines.push("");
    lines.push(`- ${runScreenshotDir}`);
    lines.push("");
    await fs.writeFile(reportFile, `${lines.join("\n")}\n`, "utf8");
    console.log(`REPORT=${reportFile}`);
    console.log(`SCREENSHOTS=${runScreenshotDir}`);

    if (failCount > 0) process.exitCode = 1;
  } catch (error) {
    addCase("TC-LOCAL-WATCH-RETRY-RUN-000", "本地监控失败任务重试执行异常", "FAIL", String(error));
    const lines = [];
    lines.push("# 本地监控失败任务重试测试报告");
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
