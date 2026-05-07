import fs from "node:fs/promises";
import path from "node:path";
import { chromium, request } from "playwright";

const FRONTEND_URL = process.env.E2E_FRONTEND_URL || "http://127.0.0.1:3001";
const BACKEND_API = process.env.E2E_BACKEND_API || "http://127.0.0.1:8082";
const ROOT_DIR = path.resolve(process.cwd(), "..");
const DEBUG_DIR = path.join(ROOT_DIR, "debug");
const SCREENSHOT_DIR = path.join(DEBUG_DIR, "e2e-screenshots", "cloud115-copy-preserves-source");
const REPORT_DIR = path.join(ROOT_DIR, "docs");

const now = new Date();
const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, "0")}${String(now.getDate()).padStart(2, "0")}_${String(now.getHours()).padStart(2, "0")}${String(now.getMinutes()).padStart(2, "0")}${String(now.getSeconds()).padStart(2, "0")}`;
const runId = `cloud115_copy_${stamp}`;
const runScreenshotDir = path.join(SCREENSHOT_DIR, runId);
const reportFile = path.join(REPORT_DIR, `115云源复制不污染源文件测试报告_${stamp}.md`);

const movieSourceCID = "2977269469445485999";
const organizedRootCID = "3410496669326971511";
const cloud115Id = 3;
const organizeRootSourceId = 20;
const sourceName = `cloud_copy_e2e_${stamp}`;
const targetPath = `/影视资源/CodexCopyProbe_${stamp}`;
const targetFolderName = `CodexCopyProbe_${stamp}`;

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
}

async function saveShot(page, name) {
  const file = path.join(runScreenshotDir, `${name}.png`);
  await page.screenshot({ path: file, fullPage: true });
  return file;
}

async function waitForMessage(page, text, timeout = 60000) {
  const locator = page.locator(".el-message").filter({ hasText: text }).last();
  await locator.waitFor({ timeout });
}

async function findDialogByTitle(page, title) {
  const dialog = page.locator(".el-dialog").filter({ hasText: title }).last();
  await dialog.waitFor({ timeout: 20000 });
  return dialog;
}

async function listFiles(api, sourceId, folderPath) {
  const resp = await api.get(`/media/files?source_id=${sourceId}&path=${encodeURIComponent(folderPath)}&page=1&page_size=200`);
  const payload = unwrapData(await apiJson(resp)) || {};
  return payload.files || [];
}

function isVideoFile(name = "") {
  return /\.(mp4|mkv|avi|mov|wmv|flv|m4v)$/i.test(name);
}

async function findTargetFolder(api) {
  const files = await listFiles(api, organizeRootSourceId, organizedRootCID);
  return files.find((item) => item.name === targetFolderName) || null;
}

async function findFileRecursive(api, sourceId, folderPath, expectedName, depth = 6) {
  if (depth <= 0) return null;
  const files = await listFiles(api, sourceId, folderPath);
  for (const item of files) {
    if ((item.name || "") === expectedName) {
      return item;
    }
  }
  for (const item of files) {
    if (!item.is_directory || !item.id) {
      continue;
    }
    const matched = await findFileRecursive(api, sourceId, item.id, expectedName, depth - 1);
    if (matched) {
      return matched;
    }
  }
  return null;
}

async function cleanupRemoteTarget(api) {
  const folder = await findTargetFolder(api);
  if (!folder?.id) {
    return false;
  }
  const resp = await api.post("/media/files/delete", {
    data: {
      source_id: organizeRootSourceId,
      file_id: folder.id
    }
  });
  return resp.ok();
}

async function run() {
  let browser;
  let api;
  let tempSourceId = null;

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
    addCase("TC-CLOUD-COPY-AUTH-001", "登录成功", "PASS", "", await saveShot(page, "01_login"));

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
        cloud115_id: cloud115Id,
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
      throw new Error(`创建 115 copy 测试媒体源失败: ${JSON.stringify(createSourcePayload)}`);
    }
    addCase("TC-CLOUD-COPY-SETUP-001", "创建临时 115 copy 媒体源成功", "PASS", `sourceId=${tempSourceId}`);

    let sourceFiles = await listFiles(api, tempSourceId, movieSourceCID);
    if (sourceFiles.length === 0) {
      sourceFiles = await listFiles(api, tempSourceId, "/");
    }
    const candidateFile = sourceFiles.find((item) => isVideoFile(item.name || ""));
    if (!candidateFile?.name) {
      throw new Error(`API 未找到可用于 copy 测试的视频文件: ${JSON.stringify(sourceFiles.slice(0, 10))}`);
    }

    await page.goto(`${FRONTEND_URL}/dashboard/media-manager`, { waitUntil: "networkidle" });
    const sourceRow = page.locator(".el-table__row").filter({ hasText: sourceName }).first();
    await sourceRow.waitFor({ timeout: 20000 });
    await sourceRow.locator(".el-button--primary").first().click();

    const browserDialog = page.locator(".el-dialog").filter({ has: page.locator(".table-wrapper") }).last();
    await browserDialog.waitFor({ timeout: 20000 });
    const fileRow = browserDialog.locator(".el-table__row").filter({ hasText: candidateFile.name }).first();
    await fileRow.waitFor({ timeout: 20000 });
    const selectedFileName = candidateFile.name;
    if (!selectedFileName) {
      throw new Error("未在 115 浏览器中找到可用于 copy 测试的视频文件");
    }
    await fileRow.locator(".el-checkbox").click();
    addCase("TC-CLOUD-COPY-UI-001", "文件浏览器可选中待整理的 115 文件", "PASS", selectedFileName, await saveShot(page, "02_source_browser"));

    await browserDialog.locator(".el-button").filter({ hasText: "批量整理" }).click();
    const organizeDialog = await findDialogByTitle(page, "批量整理");
    await organizeDialog.locator(".dialog-footer .el-button").filter({ hasText: "刷新预览" }).click();
    await waitForMessage(page, "预览完成", 90000);

    const previewRow = organizeDialog.locator(".el-table__body .el-table__row").first();
    await previewRow.waitFor({ timeout: 20000 });
    const previewText = (await organizeDialog.textContent()) || "";
    const originalPreviewName = ((await previewRow.locator("td").nth(2).textContent()) || "").replace(/\s+/g, " ").trim();
    const extension = path.extname(selectedFileName) || ".mp4";
    const manualNewName = `COPY-PROBE-${stamp}${extension}`;
    await previewRow.locator(".edit-name-btn").click();
    const editInput = previewRow.locator("input").first();
    await editInput.fill(manualNewName);
    await editInput.press("Enter");
    await page.waitForTimeout(500);
    const previewNewName = ((await previewRow.locator("td").nth(2).textContent()) || "").replace(/\s+/g, " ").trim();
    const previewChanged = previewNewName === manualNewName;
    addCase(
      "TC-CLOUD-COPY-001",
      "复制模式预览可手动指定不同于源文件的新文件名",
      previewChanged ? "PASS" : "FAIL",
      `source=${selectedFileName} autoPreview=${originalPreviewName || "-"} manualPreview=${previewNewName || "-"}`,
      await saveShot(page, "03_preview")
    );
    if (!previewChanged) {
      throw new Error(`预览手动改名未生效: source=${selectedFileName} preview=${previewNewName}`);
    }

    await organizeDialog.locator(".dialog-footer .el-button--primary").filter({ hasText: "执行整理" }).click();
    await waitForMessage(page, "整理完成", 180000);
    const resultDialog = await findDialogByTitle(page, "整理结果");
    const resultText = (await resultDialog.textContent()) || "";
    addCase(
      "TC-CLOUD-COPY-002",
      "复制模式整理成功完成",
      resultText.includes("成功") ? "PASS" : "FAIL",
      resultText.replace(/\s+/g, " ").slice(0, 220),
      await saveShot(page, "04_result")
    );

    const sourceFilesAfterCopy = await listFiles(api, tempSourceId, movieSourceCID);
    const sourceNames = sourceFilesAfterCopy.map((item) => item.name || "");
    const sourcePreserved = sourceNames.includes(selectedFileName);
    addCase(
      "TC-CLOUD-COPY-003",
      "源目录仍保留原文件名，未被 copy 整理污染",
      sourcePreserved ? "PASS" : "FAIL",
      `sourceHasOriginal=${sourcePreserved} original=${selectedFileName}`,
      ""
    );

    const targetFolder = await findTargetFolder(api);
    const matchedTargetFile = targetFolder?.id
      ? await findFileRecursive(api, organizeRootSourceId, targetFolder.id, previewNewName)
      : null;
    const targetHasRenamedCopy = Boolean(matchedTargetFile);
    addCase(
      "TC-CLOUD-COPY-004",
      "目标目录收到已重命名的副本文件",
      targetHasRenamedCopy ? "PASS" : "FAIL",
      `targetFolder=${targetFolder?.id || "-"} targetHas=${targetHasRenamedCopy} preview=${previewNewName}`,
      ""
    );

    await page.goto(`${FRONTEND_URL}/dashboard/tasks`, { waitUntil: "networkidle" });
    addCase("TC-CLOUD-COPY-UI-002", "任务中心可正常打开用于后续观察", "PASS", "", await saveShot(page, "05_tasks"));

    const cleanupOk = await cleanupRemoteTarget(api);
    addCase(
      "TC-CLOUD-COPY-005",
      "测试创建的 115 copy 目标目录已清理",
      cleanupOk ? "PASS" : "SKIP",
      cleanupOk ? "已删除远端测试目录" : "未能自动删除远端测试目录，请按需复查",
      ""
    );

    const passCount = cases.filter((item) => item.status === "PASS").length;
    const failCount = cases.filter((item) => item.status === "FAIL").length;
    const skipCount = cases.filter((item) => item.status === "SKIP").length;

    const lines = [];
    lines.push("# 115云源复制不污染源文件测试报告");
    lines.push("");
    lines.push(`- 执行时间：${new Date().toLocaleString("zh-CN", { hour12: false })}`);
    lines.push("- 执行者：Codex");
    lines.push(`- 前端地址：${FRONTEND_URL}`);
    lines.push(`- 后端地址：${BACKEND_API}`);
    lines.push(`- 测试媒体源：${sourceName}`);
    lines.push(`- 目标目录：${targetPath}`);
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
    lines.push("## 关键观察");
    lines.push("");
    lines.push(`- 本轮浏览器实际选中的源文件名：${selectedFileName}`);
    lines.push(`- 预览生成的新文件名：${previewNewName}`);
    lines.push(`- 源目录仍包含原文件名：${sourcePreserved ? "是" : "否"}`);
    lines.push(`- 目标目录包含重命名副本：${targetHasRenamedCopy ? "是" : "否"}`);
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
    addCase("TC-CLOUD-COPY-RUN-000", "115 copy 深挖执行异常", "FAIL", String(error));
    const lines = [];
    lines.push("# 115云源复制不污染源文件测试报告");
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
    if (api && tempSourceId) {
      try {
        await api.delete(`/media/sources/${tempSourceId}`);
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
