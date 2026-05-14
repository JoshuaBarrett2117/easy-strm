import { spawn } from "node:child_process";
import fs from "node:fs/promises";
import http from "node:http";
import path from "node:path";
import { chromium } from "playwright";

const FRONTEND_URL = process.env.E2E_FRONTEND_URL || "http://localhost:3001";
const NEEDS_SERVER = !process.env.E2E_FRONTEND_URL;

const source = {
  id: 1,
  name: "资源整理演示源",
  source_type: "cloud115",
  path: "/影视/演示"
};

const indexRows = [
  {
    id: 101,
    source_id: 1,
    source_type: "cloud115",
    source_name: "Movie.Ready.2026.mkv",
    source_path: "/影视/Movie.Ready.2026.mkv",
    target_path: "/媒体库/电影/Movie Ready (2026)",
    strm_path: "",
    metadata_path: "/媒体库/电影/Movie Ready (2026)/movie.nfo",
    has_strm: false,
    has_metadata: true,
    health_status: "strm_missing",
    identity_status: "identified",
    sync_status: "active",
    last_change_type: "updated",
    last_task_id: "library_sync_1"
  },
  {
    id: 102,
    source_id: 1,
    source_type: "cloud115",
    source_name: "Unknown.Show.S01E01.mkv",
    source_path: "/影视/Unknown.Show.S01E01.mkv",
    target_path: "",
    strm_path: "",
    metadata_path: "",
    has_strm: false,
    has_metadata: false,
    health_status: "identify_failed",
    identity_status: "failed",
    sync_status: "active",
    last_change_type: "created",
    last_task_id: "library_pipeline_1"
  }
];

const pendingRows = [
  {
    id: 301,
    source_kind: "cloud115",
    source_id: 1,
    source_path: "/影视/Unknown.Show.S01E01.mkv",
    title: "Unknown Show",
    year: 2026,
    media_type: "tv",
    season: 1,
    episode: 1,
    tmdb_id: 0,
    status: "pending",
    reason: "TMDB 未匹配",
    related_task_id: "library_pipeline_1"
  }
];

const taskDetail = {
  task_id: "library_strm_item_101_e2e",
  task_type: "strm_generate",
  task_name: "生成 STRM-Movie.Ready.2026.mkv",
  status: "completed",
  progress: 100,
  total_files: 1,
  processed_files: 1,
  success_count: 1,
  failed_count: 0,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
  steps: [
    {
      id: "generate_strm",
      name: "生成 STRM",
      status: "completed",
      progress: 100,
      message: "STRM 已生成并写入资产台账"
    }
  ]
};

function success(data) {
  return {
    state: true,
    code: 0,
    message: "success",
    data,
    error: "",
    errno: 0
  };
}

function waitForUrl(url, timeoutMs = 15000) {
  return new Promise((resolve, reject) => {
    const deadline = Date.now() + timeoutMs;
    const tick = () => {
      http.get(url, (res) => {
        res.resume();
        resolve();
      }).on("error", () => {
        if (Date.now() > deadline) {
          reject(new Error(`前端服务未就绪: ${url}`));
          return;
        }
        setTimeout(tick, 350);
      });
    };
    tick();
  });
}

async function ensureFrontend() {
  try {
    await waitForUrl(FRONTEND_URL, 1200);
    return null;
  } catch {
    if (!NEEDS_SERVER) throw new Error(`无法访问 E2E_FRONTEND_URL: ${FRONTEND_URL}`);
  }

  const viteBin = path.join(process.cwd(), "node_modules", "vite", "bin", "vite.js");
  const child = spawn(process.execPath, [viteBin, "--host", "127.0.0.1", "--port", "3001", "--strictPort"], {
    cwd: process.cwd(),
    stdio: "pipe"
  });
  child.stdout.on("data", (chunk) => process.stdout.write(chunk));
  child.stderr.on("data", (chunk) => process.stderr.write(chunk));
  await waitForUrl(FRONTEND_URL);
  return child;
}

async function routeApi(page) {
  await page.route("**/*", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (!url.pathname.startsWith("/api/")) {
      return route.continue();
    }
    const path = url.pathname.replace(/^\/api/, "");
    const method = request.method();

    if (method === "GET" && path === "/media/sources") {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(success({ data: [source], total: 1 })) });
    }
    if (method === "GET" && path === "/media/sources/1/sync/index") {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(success({ data: indexRows, total: indexRows.length })) });
    }
    if (method === "POST" && path === "/media/sources/1/sync/incremental") {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(success({ task_id: "library_sync_1", scanned: 2, changed: 1, missing: 0 })) });
    }
    if (method === "POST" && path === "/media/sources/1/pipeline") {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(success({ task_id: "library_pipeline_1", total: 2, identified: 1, pending: 1, strm: 0 })) });
    }
    if (method === "GET" && path === "/media/library/items") {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(success({ data: indexRows, total: indexRows.length })) });
    }
    if (method === "POST" && path === "/media/library/items/101/strm") {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(success({
          task_id: "library_strm_item_101_e2e",
          item_id: 101,
          message: "STRM 已生成并写入资产台账",
          item: { ...indexRows[0], has_strm: true, strm_path: "/strm/Movie.Ready.2026.strm" }
        }))
      });
    }
    if (method === "POST" && path === "/media/library/items/101/refresh-server") {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(success({ task_id: "library_refresh_item_101_e2e", item_id: 101, message: "媒体服务器刷新请求已发送" })) });
    }
    if (method === "GET" && path === "/media/pending") {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(success({ data: pendingRows, total: pendingRows.length })) });
    }
    if (method === "POST" && path === "/media/pending/301/identify") {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(success({ ...pendingRows[0], status: "identified", tmdb_id: 12345 })) });
    }
    if (method === "POST" && path === "/media/pending/301/run") {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(success({ message: "待处理项已重新入库", data: { ...pendingRows[0], status: "completed" }, task: { task_id: "library_pipeline_pending_301", total: 1, identified: 1, pending: 0, strm: 1 } })) });
    }
    if (method === "GET" && path === "/tasks/unified") {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(success({ data: [taskDetail], total: 1 })) });
    }
    if (method === "GET" && path === "/tasks/library_strm_item_101_e2e") {
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(success(taskDetail)) });
    }

    return route.fulfill({ status: 404, contentType: "application/json", body: JSON.stringify({ error: `未模拟接口: ${method} ${path}` }) });
  });
}

async function main() {
  let server;
  let browser;
  let page;
  try {
    server = await ensureFrontend();
    browser = await chromium.launch({ headless: true, args: ["--no-proxy-server"] });
    const context = await browser.newContext({ viewport: { width: 1500, height: 960 } });
    await context.addInitScript(() => {
      localStorage.setItem("token", "e2e-token");
      localStorage.setItem("user_id", "1");
    });
    page = await context.newPage();
    page.on("console", (message) => {
      if (message.type() === "error") {
        console.error(`browser console error: ${message.text()}`);
      }
    });
    await routeApi(page);

    await page.goto(`${FRONTEND_URL}/dashboard/sync-tasks?source_id=1`, { waitUntil: "networkidle" });
    await page.getByRole("heading", { name: "同步入库工作台" }).waitFor();
    await page.getByRole("button", { name: "增量同步" }).click();
    await page.getByText("任务 library_sync_1 已完成").waitFor();
    await page.getByRole("button", { name: "查看资产台账" }).last().click();

    await page.waitForURL("**/dashboard/media-library?source_id=1");
    await page.getByRole("heading", { name: "媒体资产台账" }).waitFor();
    const ledgerRow = page.locator(".el-table__row").filter({ hasText: "Movie.Ready.2026.mkv" }).first();
    await ledgerRow.getByRole("button", { name: "STRM" }).click();
    await page.getByText("library_strm_item_101_e2e").first().waitFor();
    await page.getByRole("button", { name: "查看任务" }).click();

    await page.waitForURL("**/dashboard/tasks?task_id=library_strm_item_101_e2e");
    await page.getByText("生成 STRM-Movie.Ready.2026.mkv").waitFor();

    await page.goto(`${FRONTEND_URL}/dashboard/pending-media?source_id=1`, { waitUntil: "networkidle" });
    await page.getByRole("heading", { name: "待处理资源" }).waitFor();
    const pendingRow = page.locator(".el-table__row").filter({ hasText: "Unknown Show" }).first();
    await pendingRow.getByRole("button", { name: "修正" }).click();
    const dialog = page.locator(".el-dialog").last();
    await dialog.locator("input").nth(0).fill("Unknown Show");
    await dialog.locator("input").nth(1).fill("12345");
    await dialog.getByRole("button", { name: "保存并入库" }).click();
    await page.getByText("library_pipeline_pending_301").waitFor();

    console.log(JSON.stringify({ ok: true, checked: ["sync", "ledger", "strm task link", "pending identify and run"] }, null, 2));
  } catch (error) {
    if (page) {
      const debugDir = path.resolve(process.cwd(), "..", "debug");
      await fs.mkdir(debugDir, { recursive: true });
      const screenshot = path.join(debugDir, "resource-platform-e2e-failure.png");
      await page.screenshot({ path: screenshot, fullPage: true }).catch(() => {});
      const bodyText = await page.locator("body").innerText().catch(() => "");
      console.error(JSON.stringify({
        url: page.url(),
        screenshot,
        body: bodyText.slice(0, 1000)
      }, null, 2));
    }
    throw error;
  } finally {
    if (browser) await browser.close();
    if (server) server.kill();
  }
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
