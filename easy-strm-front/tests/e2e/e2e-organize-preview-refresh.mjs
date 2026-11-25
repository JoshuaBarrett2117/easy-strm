import fs from "node:fs/promises";
import path from "node:path";
import { chromium, request } from "playwright";

const FRONTEND_URL = process.env.E2E_FRONTEND_URL || "http://localhost:3001";
const BACKEND_API = process.env.E2E_BACKEND_API || "http://127.0.0.1:8082";
const ROOT_DIR = path.resolve(process.cwd(), "..");
const DEBUG_DIR = path.join(ROOT_DIR, "debug");

const now = new Date();
const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, "0")}${String(now.getDate()).padStart(2, "0")}_${String(now.getHours()).padStart(2, "0")}${String(now.getMinutes()).padStart(2, "0")}${String(now.getSeconds()).padStart(2, "0")}`;

const fixtureRoot = path.join(DEBUG_DIR, "e2e-fixtures", `preview_refresh_${stamp}`);
const fixtureSource = path.join(fixtureRoot, "source");
const fixtureTarget = path.join(fixtureRoot, "target");
const screenshotFile = path.join(DEBUG_DIR, `preview_refresh_${stamp}.png`);

async function ensureFixtures() {
  await fs.mkdir(fixtureSource, { recursive: true });
  await fs.mkdir(fixtureTarget, { recursive: true });
  await fs.writeFile(path.join(fixtureSource, "Movie.Manual.2024.1080p.mkv"), "preview refresh fixture\n", "utf8");
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
    await page.getByPlaceholder("用户名").fill("admin");
    await page.getByPlaceholder("密码").fill("admin");
    await page.getByRole("button", { name: "登录" }).click();
    await page.waitForURL("**/dashboard/**", { timeout: 15000 });

    const token = await page.evaluate(() => localStorage.getItem("token"));
    if (!token) fail("登录后未获取到 token");

    api = await request.newContext({
      baseURL: BACKEND_API,
      extraHTTPHeaders: {
        Authorization: `Bearer ${token}`
      }
    });

    const sourceName = `manual_local_${Date.now()}`;
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

    await page.goto(`${FRONTEND_URL}/dashboard/media-manager`, { waitUntil: "networkidle" });
    const row = page.getByRole("row").filter({ hasText: sourceName });
    await row.waitFor({ timeout: 15000 });
    await row.getByRole("button", { name: "浏览" }).click();

    const browserDialog = page.getByRole("dialog").filter({ hasText: "文件浏览" });
    await browserDialog.waitFor({ timeout: 15000 });

    const fileRow = browserDialog.getByRole("row").filter({ hasText: "Movie.Manual.2024.1080p.mkv" });
    await fileRow.getByRole("checkbox").click();
    await browserDialog.getByRole("button", { name: /批量整理/ }).click();

    const organizeDialog = page.getByRole("dialog").filter({ hasText: "批量整理工作流" });
    await organizeDialog.waitFor({ timeout: 15000 });

    const previewButton = organizeDialog.getByRole("button", { name: "刷新预览" });
    await previewButton.click();

    await page.waitForFunction(() => {
      const dialogs = Array.from(document.querySelectorAll('[role="dialog"]'));
      const dialog = dialogs.find((item) => item.textContent?.includes("批量整理工作流"));
      if (!dialog) return false;

      const button = Array.from(dialog.querySelectorAll("button"))
        .find((item) => item.textContent?.includes("刷新预览"));
      if (!button) return false;

      const hasPreviewState = dialog.textContent?.includes("识别失败")
        || dialog.textContent?.includes("可处理")
        || dialog.textContent?.includes("原文件名");

      return !button.disabled && hasPreviewState;
    }, { timeout: 20000 });

    const previewState = await organizeDialog.evaluate((dialog) => {
      const footerButtons = Array.from(dialog.querySelectorAll("button"))
        .filter((button) => ["取消", "刷新预览", "执行整理"].some((text) => button.textContent?.includes(text)))
        .map((button) => ({
        text: button.textContent || "",
        disabled: button.disabled
      }));
      return {
        hasLoadingMask: Boolean(dialog.querySelector('[aria-busy="true"]')),
        hasPreviewTable: Boolean(dialog.querySelector("table")),
        footerButtons,
        text: dialog.textContent || ""
      };
    });

    if (previewState.hasLoadingMask) {
      fail("整理弹窗遮罩仍未退出");
    }

    const refreshButtonState = previewState.footerButtons.find((button) => button.text.includes("刷新预览"));
    if (!refreshButtonState || refreshButtonState.disabled) {
      fail("刷新预览按钮仍处于 loading 状态");
    }

    await page.screenshot({ path: screenshotFile, fullPage: true });

    console.log(JSON.stringify({
      ok: true,
      sourceId,
      sourceName,
      screenshot: screenshotFile,
      previewState
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
