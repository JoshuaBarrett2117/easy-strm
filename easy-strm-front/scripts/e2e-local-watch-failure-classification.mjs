import fs from "node:fs/promises";
import path from "node:path";
import { chromium, request } from "playwright";

const FRONTEND_URL = process.env.E2E_FRONTEND_URL || "http://127.0.0.1:3001";
const BACKEND_API = process.env.E2E_BACKEND_API || "http://127.0.0.1:8082";
const ROOT_DIR = path.resolve(process.cwd(), "..");
const DEBUG_DIR = path.join(ROOT_DIR, "debug");
const SCREENSHOT_DIR = path.join(DEBUG_DIR, "e2e-screenshots", "local-watch-failure-classification");
const REPORT_DIR = path.join(ROOT_DIR, "docs");

const now = new Date();
const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, "0")}${String(now.getDate()).padStart(2, "0")}_${String(now.getHours()).padStart(2, "0")}${String(now.getMinutes()).padStart(2, "0")}${String(now.getSeconds()).padStart(2, "0")}`;
const runId = `local_watch_failure_${stamp}`;
const runScreenshotDir = path.join(SCREENSHOT_DIR, runId);
const reportFile = path.join(REPORT_DIR, `本地监控失败分类测试报告_${stamp}.md`);
const fixtureRoot = path.join(DEBUG_DIR, "e2e-fixtures", runId);
const watchDir = path.join(fixtureRoot, "watch");
const targetDir = path.join(fixtureRoot, "target");
const mediaSourceName = `local_watch_failed_${stamp}`;
const injectedFileName = "Codex.Watch.Auto.Trigger.2024.1080p.mkv";

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

async function getUnifiedTasks(api) {
  const resp = await api.get("/tasks/unified");
  const payload = unwrapData(await apiJson(resp)) || [];
  return Array.isArray(payload) ? payload : [];
}

async function pollWatchTask(api, sourceName, timeoutMs = 90000) {
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
    addCase("TC-LOCAL-WATCH-FAIL-AUTH-001", "登录成功", "PASS", "", await saveShot(page, "01_login"));

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
      throw new Error(`创建本地 watch 失败分类测试媒体源失败: ${JSON.stringify(createSourceData)}`);
    }
    addCase("TC-LOCAL-WATCH-FAIL-SETUP-001", "创建失败分类测试媒体源成功", "PASS", `sourceId=${mediaSourceId}`);

    const injectedPath = path.join(watchDir, injectedFileName);
    await fs.writeFile(injectedPath, "watch trigger fixture\n", "utf8");
    addCase("TC-LOCAL-WATCH-FAIL-TRIGGER-001", "注入无法识别的监控文件", "PASS", injectedPath);

    const finishedTask = await pollWatchTask(api, mediaSourceName, 120000);
    addCase(
      "TC-LOCAL-WATCH-FAIL-001",
      "无法识别的监控文件会生成失败任务",
      finishedTask?.status === "failed" ? "PASS" : "FAIL",
      finishedTask ? `taskId=${finishedTask.task_id} category=${finishedTask.metadata?.failure_category || "-"} reason=${finishedTask.metadata?.failure_reason || "-"}` : "超时未获取失败任务",
      ""
    );
    if (!finishedTask) {
      throw new Error("未获取到失败的 watch_auto_organize 任务");
    }

    await page.goto(`${FRONTEND_URL}/dashboard/tasks`, { waitUntil: "networkidle" });
    const taskCard = page.getByTestId("task-card").filter({ hasText: mediaSourceName }).first();
    await taskCard.waitFor({ timeout: 20000 });
    const taskCardText = (await taskCard.textContent()) || "";
    const cardPass = taskCardText.includes("识别失败") || taskCardText.includes("失败文件");
    addCase(
      "TC-LOCAL-WATCH-FAIL-002",
      "任务卡片会展示识别失败相关信息",
      cardPass ? "PASS" : "FAIL",
      taskCardText.replace(/\s+/g, " ").slice(0, 280),
      await saveShot(page, "02_task_card")
    );

    await taskCard.locator("button").filter({ hasText: "详情" }).click();
    const drawer = page.locator('[role="dialog"]').filter({ hasText: "任务详情" }).last();
    await drawer.waitFor({ timeout: 20000 });
    const drawerText = (await drawer.textContent()) || "";
    const detailPass = drawerText.includes("识别失败") && drawerText.includes(injectedFileName);
    addCase(
      "TC-LOCAL-WATCH-FAIL-003",
      "任务详情里的失败文件分组显示为识别失败",
      detailPass ? "PASS" : "FAIL",
      drawerText.replace(/\s+/g, " ").slice(0, 360),
      await saveShot(page, "03_task_detail")
    );

    const passCount = cases.filter((item) => item.status === "PASS").length;
    const failCount = cases.filter((item) => item.status === "FAIL").length;
    const skipCount = cases.filter((item) => item.status === "SKIP").length;

    const lines = [];
    lines.push("# 本地监控失败分类测试报告");
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
    lines.push("## 截图目录");
    lines.push("");
    lines.push(`- ${runScreenshotDir}`);
    lines.push("");
    await fs.writeFile(reportFile, `${lines.join("\n")}\n`, "utf8");
    console.log(`REPORT=${reportFile}`);
    console.log(`SCREENSHOTS=${runScreenshotDir}`);

    if (failCount > 0) process.exitCode = 1;
  } catch (error) {
    addCase("TC-LOCAL-WATCH-FAIL-RUN-000", "本地监控失败分类深挖执行异常", "FAIL", String(error));
    const lines = [];
    lines.push("# 本地监控失败分类测试报告");
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
