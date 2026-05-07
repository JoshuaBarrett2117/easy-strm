import fs from "node:fs/promises";
import path from "node:path";
import { chromium, request } from "playwright";

const FRONTEND_URL = process.env.E2E_FRONTEND_URL || "http://localhost:3001";
const BACKEND_API = process.env.E2E_BACKEND_API || "http://127.0.0.1:8082";
const ROOT_DIR = path.resolve(process.cwd(), "..");
const DEBUG_DIR = path.join(ROOT_DIR, "debug");

const now = new Date();
const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, "0")}${String(now.getDate()).padStart(2, "0")}_${String(now.getHours()).padStart(2, "0")}${String(now.getMinutes()).padStart(2, "0")}${String(now.getSeconds()).padStart(2, "0")}`;

const fixtureRoot = path.join(DEBUG_DIR, "e2e-fixtures", `preview_cancel_${stamp}`);
const fixtureSource = path.join(fixtureRoot, "source");
const fixtureTarget = path.join(fixtureRoot, "target");
const screenshotFile = path.join(DEBUG_DIR, `preview_cancel_${stamp}.png`);

async function ensureFixtures() {
  await fs.mkdir(fixtureSource, { recursive: true });
  await fs.mkdir(fixtureTarget, { recursive: true });
  await fs.writeFile(path.join(fixtureSource, "Movie.Cancel.2024.1080p.mkv"), "preview cancel fixture\n", "utf8");
}

function fail(message) {
  throw new Error(message);
}

async function main() {
  let browser;
  let api;
  let sourceId = null;

  try {
    await ensureFixtures();

    browser = await chromium.launch({ headless: true });
    const context = await browser.newContext({ viewport: { width: 1560, height: 980 } });
    const page = await context.newPage();

    await page.goto(`${FRONTEND_URL}/login`, { waitUntil: "networkidle" });
    await page.locator('input[type="text"]').first().fill("admin");
    await page.locator('input[type="password"]').first().fill("admin");
    await page.locator(".login-btn").click();
    await page.waitForURL("**/dashboard/**", { timeout: 15000 });

    const token = await page.evaluate(() => localStorage.getItem("token"));
    if (!token) fail("登录后未获取到 token");

    api = await request.newContext({
      baseURL: BACKEND_API,
      extraHTTPHeaders: {
        Authorization: `Bearer ${token}`
      }
    });

    const sourceName = `preview_cancel_${Date.now()}`;
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
    if (!createResp.ok()) fail(`创建媒体源失败: ${JSON.stringify(createData)}`);

    sourceId = createData?.data?.id || createData?.data?.data?.id || null;
    if (!sourceId) fail(`创建媒体源响应中缺少 ID: ${JSON.stringify(createData)}`);

    let statusPollCount = 0;
    await page.route("**/api/media/organize/preview/async", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          code: 0,
          message: "success",
          data: {
            task_id: "stub-preview-task"
          }
        })
      });
    });
    await page.route("**/api/media/organize/preview/status?*", async (route) => {
      statusPollCount += 1;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          code: 0,
          message: "success",
          data: {
            task_id: "stub-preview-task",
            status: "processing"
          }
        })
      });
    });

    await page.goto(`${FRONTEND_URL}/dashboard/media-manager`, { waitUntil: "networkidle" });
    const row = page.locator(".el-table__row").filter({ hasText: sourceName }).first();
    await row.waitFor({ timeout: 15000 });
    await row.locator("button").first().click();

    const browserDialog = page.locator(".el-dialog").last();
    await browserDialog.waitFor({ timeout: 15000 });

    await browserDialog.locator(".el-table__body-wrapper tbody .el-checkbox").first().click();
    await browserDialog.locator(".organize-primary-btn").click();

    let organizeDialog = page.locator(".el-dialog").last();
    await organizeDialog.waitFor({ timeout: 15000 });

    const previewButton = organizeDialog.locator(".dialog-footer button").nth(1);
    await previewButton.click();

    await page.waitForFunction(() => {
      const dialogs = Array.from(document.querySelectorAll(".el-dialog"));
      const dialog = dialogs.find((item) => item.querySelector(".organize-container"));
      if (!dialog) return false;
      const footerButtons = Array.from(dialog.querySelectorAll(".dialog-footer button"));
      return footerButtons[1]?.classList.contains("is-loading");
    }, { timeout: 10000 });

    while (statusPollCount < 2) {
      await page.waitForTimeout(300);
    }

    await organizeDialog.locator(".dialog-footer button").first().click();
    await page.waitForTimeout(2600);
    const countAfterClose = statusPollCount;
    await page.waitForTimeout(2200);
    if (statusPollCount !== countAfterClose) {
      fail(`关闭弹窗后轮询未停止，关闭后计数 ${countAfterClose}，当前 ${statusPollCount}`);
    }

    const browserDialogAgain = page.locator(".el-dialog").filter({ has: page.locator(".table-wrapper") }).last();
    await browserDialogAgain.waitFor({ timeout: 15000 });

    await browserDialogAgain.locator(".organize-primary-btn").click();
    organizeDialog = page.locator(".el-dialog").last();
    await organizeDialog.waitFor({ timeout: 15000 });

    await page.waitForFunction(() => {
      const dialogs = Array.from(document.querySelectorAll(".el-dialog"));
      const dialog = dialogs.find((item) => item.querySelector(".organize-container"));
      if (!dialog) return false;
      const loadingMask = dialog.querySelector(".organize-container .el-loading-mask");
      const footerButtons = Array.from(dialog.querySelectorAll(".dialog-footer button"));
      return !loadingMask && !footerButtons[1]?.classList.contains("is-loading");
    }, { timeout: 15000 });

    const dialogState = await organizeDialog.evaluate((dialog) => {
      const footerButtons = Array.from(dialog.querySelectorAll(".dialog-footer button")).map((button) => ({
        text: button.textContent || "",
        className: button.className
      }));
      return {
        hasLoadingMask: Boolean(dialog.querySelector(".organize-container .el-loading-mask")),
        refreshButtonClass: footerButtons[1]?.className || ""
      };
    });

    if (dialogState.hasLoadingMask) {
      fail("重新打开整理弹窗后仍残留 loading 遮罩");
    }
    if (dialogState.refreshButtonClass.includes("is-loading")) {
      fail("重新打开整理弹窗后刷新预览按钮仍处于 loading 状态");
    }

    await page.screenshot({ path: screenshotFile, fullPage: true });

    console.log(JSON.stringify({
      ok: true,
      sourceId,
      sourceName,
      screenshot: screenshotFile,
      statusPollCount
    }, null, 2));
  } finally {
    if (api && sourceId) {
      await api.delete(`/media/sources/${sourceId}`).catch(() => {});
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
