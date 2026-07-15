import fs from "node:fs/promises";
import path from "node:path";
import { chromium, request } from "playwright";

const FRONTEND_URL = process.env.E2E_FRONTEND_URL || "http://localhost:3001";
const BACKEND_API = process.env.E2E_BACKEND_API || "http://localhost:8082";
const ROOT_DIR = path.resolve(process.cwd(), "..");
const DEBUG_DIR = path.join(ROOT_DIR, "debug");
const SCREENSHOT_DIR = path.join(DEBUG_DIR, "e2e-screenshots", "media-source-config-roundtrip");
const REPORT_DIR = path.join(ROOT_DIR, "docs");

const now = new Date();
const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, "0")}${String(now.getDate()).padStart(2, "0")}_${String(now.getHours()).padStart(2, "0")}${String(now.getMinutes()).padStart(2, "0")}${String(now.getSeconds()).padStart(2, "0")}`;
const runId = `media_source_config_roundtrip_${stamp}`;
const runScreenshotDir = path.join(SCREENSHOT_DIR, runId);
const reportFile = path.join(REPORT_DIR, `媒体源配置回写测试报告_${stamp}.md`);
const fixtureRoot = path.join(DEBUG_DIR, "e2e-fixtures", runId);

const cases = [];

function addCase(id, name, status, detail = "", artifact = "") {
  cases.push({ id, name, status, detail, artifact });
  const prefix = status === "PASS" ? "[PASS]" : status === "FAIL" ? "[FAIL]" : "[SKIP]";
  console.log(`${prefix} ${id} ${name}${detail ? ` -> ${detail}` : ""}`);
}

async function ensureDirs() {
  await fs.mkdir(runScreenshotDir, { recursive: true });
  await fs.mkdir(REPORT_DIR, { recursive: true });
  await fs.mkdir(fixtureRoot, { recursive: true });
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

async function openDialog(page, title) {
  const dialog = page.locator('[role="dialog"]').filter({ hasText: title }).last();
  await dialog.waitFor({ timeout: 15000 });
  return dialog;
}

function getFormItem(container, label) {
  return container.locator(".n-form-item").filter({
    has: container.locator(".n-form-item__label", { hasText: label })
  }).first();
}

async function clickSelectAt(page, container, index, optionText) {
  await container.locator(".n-select").nth(index).click();
  await page.locator(".n-base-select-option").filter({ hasText: optionText }).last().click();
}

async function setInputByPlaceholder(container, placeholder, value) {
  const input = container.locator(`input[placeholder*="${placeholder}"]`).first();
  await input.waitFor({ timeout: 10000 });
  await input.fill(String(value));
}

async function setInputNumberAt(container, index, value) {
  const input = container.locator(".n-input-number input").nth(index);
  await input.waitFor({ timeout: 10000 });
  await input.fill(String(value));
  await input.press("Tab");
}

async function setSwitchAt(container, index, targetOn) {
  const control = container.locator('[role="switch"]').nth(index);
  await control.waitFor({ timeout: 10000 });
  const checked = await control.evaluate((node) => node.classList.contains("is-checked"));
  if (checked !== targetOn) {
    await control.click();
  }
}

async function openMediaSourceDialog(page) {
  await page.goto(`${FRONTEND_URL}/dashboard/media-manager`, { waitUntil: "networkidle" });
  const addButton = page.getByRole("button", { name: "新增媒体源" }).first();
  await addButton.waitFor({ timeout: 15000 });
  await addButton.click();
  return openDialog(page, "新增媒体源");
}

async function run() {
  let browser;
  let api;
  const createdSourceIds = [];

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
    addCase("TC-MS-CONFIG-AUTH-001", "登录成功", "PASS", "", await saveShot(page, "01_login"));

    const token = await page.evaluate(() => localStorage.getItem("token"));
    if (!token) throw new Error("登录后未获取 token");
    api = await request.newContext({
      baseURL: BACKEND_API,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` }
    });

    const cloudResp = await api.get("/cloud115");
    const cloudPayload = unwrapData(await apiJson(cloudResp)) || [];
    const cloudAccounts = Array.isArray(cloudPayload) ? cloudPayload : cloudPayload.data || [];

    const localPath = path.join(fixtureRoot, "local-source");
    const localTarget = path.join(fixtureRoot, "local-target");
    await fs.mkdir(localPath, { recursive: true });
    await fs.mkdir(localTarget, { recursive: true });

    let dialog = await openMediaSourceDialog(page);
    const localName = `本地配置回写_${stamp}`;
    await setInputByPlaceholder(dialog, "媒体源名称", localName);
    await setInputByPlaceholder(dialog, "请输入路径", localPath);
    await setInputByPlaceholder(dialog, "留空则使用媒体源路径", localTarget);
    await clickSelectAt(page, dialog, 0, "剧集");
    await clickSelectAt(page, dialog, 1, "追加序号");
    await clickSelectAt(page, dialog, 2, "硬链接");
    await setSwitchAt(dialog, 0, true);
    await setSwitchAt(dialog, 1, true);
    await dialog.locator("button").click();
    await waitForMessage(page, "新增成功");
    addCase("TC-MS-CONFIG-LOCAL-001", "本地媒体源表单提交成功", "PASS", localName, await saveShot(page, "02_local_created"));

    const listResp = await api.get("/media/sources");
    const listPayload = unwrapData(await apiJson(listResp)) || [];
    const listItems = Array.isArray(listPayload) ? listPayload : listPayload.data || [];
    const createdLocal = listItems.find((item) => item.name === localName);
    if (!createdLocal?.id) throw new Error("未查到新建的本地媒体源");
    createdSourceIds.push(createdLocal.id);

    const localDetailResp = await api.get(`/media/sources/${createdLocal.id}`);
    const localDetail = unwrapData(await apiJson(localDetailResp)) || {};
    const localPass =
      localDetail.source_type === "local" &&
      localDetail.path === localPath &&
      localDetail.organize_target_path === localTarget &&
      localDetail.media_type === "tv" &&
      localDetail.conflict_policy === "suffix" &&
      localDetail.operation_mode === "hardlink" &&
      localDetail.auto_organize === true &&
      localDetail.watch_enabled === true;
    addCase(
      "TC-MS-CONFIG-LOCAL-002",
      "本地媒体源字段回写正确",
      localPass ? "PASS" : "FAIL",
      `type=${localDetail.source_type} media=${localDetail.media_type} conflict=${localDetail.conflict_policy} mode=${localDetail.operation_mode} auto=${localDetail.auto_organize} watch=${localDetail.watch_enabled}`,
      await saveShot(page, "03_local_verified")
    );

    if (cloudAccounts.length > 0) {
      dialog = await openMediaSourceDialog(page);
      const cloudName = `云盘配置回写_${stamp}`;
      const cloudPath = `/codex/config/${stamp}`;
      const cloudWatchPath = `/codex/watch/${stamp}`;
      const cloudTarget = `/codex/organized/${stamp}`;
      await setInputByPlaceholder(dialog, "媒体源名称", cloudName);
      await dialog.locator('.n-radio-group .n-radio').filter({ hasText: "115云盘" }).click();
      await setInputByPlaceholder(dialog, "请输入路径", cloudPath);
      await clickSelectAt(page, dialog, 0, cloudAccounts[0].name);
      await setInputByPlaceholder(dialog, "请输入要轮询的 115 目录 CID", cloudWatchPath);
      await setInputByPlaceholder(dialog, "留空则默认整理回当前媒体源路径", cloudTarget);
      await clickSelectAt(page, dialog, 1, "电影");
      await clickSelectAt(page, dialog, 2, "覆盖");
      await clickSelectAt(page, dialog, 3, "复制文件");
      await setSwitchAt(dialog, 0, true);
      await setSwitchAt(dialog, 1, true);
      await setInputNumberAt(dialog, 0, 600);
      await dialog.locator("button").click();
      await waitForMessage(page, "新增成功");
      addCase("TC-MS-CONFIG-CLOUD-001", "115 云盘媒体源表单提交成功", "PASS", cloudName, await saveShot(page, "04_cloud_created"));

      const cloudListResp = await api.get("/media/sources");
      const cloudListPayload = unwrapData(await apiJson(cloudListResp)) || [];
      const cloudItems = Array.isArray(cloudListPayload) ? cloudListPayload : cloudListPayload.data || [];
      const createdCloud = cloudItems.find((item) => item.name === cloudName);
      if (!createdCloud?.id) throw new Error("未查到新建的 115 云盘媒体源");
      createdSourceIds.push(createdCloud.id);

      const cloudDetailResp = await api.get(`/media/sources/${createdCloud.id}`);
      const cloudDetail = unwrapData(await apiJson(cloudDetailResp)) || {};
      const cloudPass =
        cloudDetail.source_type === "cloud115" &&
        cloudDetail.path === cloudPath &&
        cloudDetail.watch_path === cloudWatchPath &&
        cloudDetail.organize_target_path === cloudTarget &&
        cloudDetail.media_type === "movie" &&
        cloudDetail.conflict_policy === "overwrite" &&
        cloudDetail.operation_mode === "copy" &&
        cloudDetail.auto_organize === true &&
        cloudDetail.watch_enabled === true &&
        cloudDetail.watch_interval === 600 &&
        cloudDetail.cloud115_id === cloudAccounts[0].id;
      addCase(
        "TC-MS-CONFIG-CLOUD-002",
        "115 云盘媒体源字段回写正确",
        cloudPass ? "PASS" : "FAIL",
        `watch_path=${cloudDetail.watch_path} conflict=${cloudDetail.conflict_policy} mode=${cloudDetail.operation_mode} auto=${cloudDetail.auto_organize} watch=${cloudDetail.watch_enabled} interval=${cloudDetail.watch_interval}`,
        await saveShot(page, "05_cloud_verified")
      );
    } else {
      addCase("TC-MS-CONFIG-CLOUD-001", "115 云盘媒体源表单提交成功", "SKIP", "当前环境无可用 115 账号");
      addCase("TC-MS-CONFIG-CLOUD-002", "115 云盘媒体源字段回写正确", "SKIP", "当前环境无可用 115 账号");
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

    const lines = [
      "# 媒体源配置回写测试报告",
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
    addCase("TC-MS-CONFIG-RUN-000", "媒体源配置回写执行异常", "FAIL", `${error.name}: ${error.message}`);
    const lines = [
      "# 媒体源配置回写测试报告",
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
      for (const id of createdSourceIds.reverse()) {
        await api.delete(`/media/sources/${id}`).catch(() => {});
      }
      await api.dispose().catch(() => {});
    }
    if (browser) await browser.close();
  }
}

run();
