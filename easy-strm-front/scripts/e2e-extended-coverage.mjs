import fs from "node:fs/promises";
import path from "node:path";
import { chromium, request } from "playwright";

const FRONTEND_URL = process.env.E2E_FRONTEND_URL || "http://localhost:3001";
const BACKEND_API = process.env.E2E_BACKEND_API || "http://localhost:8082";
const ROOT_DIR = path.resolve(process.cwd(), "..");
const DEBUG_DIR = path.join(ROOT_DIR, "debug");
const SCREENSHOT_DIR = path.join(DEBUG_DIR, "e2e-screenshots", "extended");
const REPORT_DIR = path.join(ROOT_DIR, "docs");

const now = new Date();
const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, "0")}${String(now.getDate()).padStart(2, "0")}_${String(now.getHours()).padStart(2, "0")}${String(now.getMinutes()).padStart(2, "0")}${String(now.getSeconds()).padStart(2, "0")}`;
const runId = `extended_${stamp}`;
const runScreenshotDir = path.join(SCREENSHOT_DIR, runId);
const reportFile = path.join(REPORT_DIR, `扩展覆盖测试报告_${stamp}.md`);

const cases = [];

function addCase(id, name, status, detail = "", artifact = "") {
  cases.push({ id, name, status, detail, artifact });
  const prefix = status === "PASS" ? "[PASS]" : status === "FAIL" ? "[FAIL]" : "[SKIP]";
  console.log(`${prefix} ${id} ${name}${detail ? ` -> ${detail}` : ""}`);
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

async function apiJson(resp) {
  try {
    return await resp.json();
  } catch {
    return null;
  }
}

function normalizeData(payload) {
  if (!payload || typeof payload !== "object") return payload;
  return "data" in payload ? payload.data : payload;
}

function settingsToPayload(raw = {}) {
  const payload = {};
  for (const [key, value] of Object.entries(raw)) {
    payload[key] = value == null ? "" : String(value);
  }
  return payload;
}

async function confirmPrimaryAction(page) {
  const primary = page.locator(".el-message-box__btns .el-button--primary").last();
  await primary.waitFor({ timeout: 10000 });
  await primary.click();
}

async function run() {
  let browser;
  let api;
  let createdCategoryId = null;
  let originalTheme = null;
  let originalSettingsPayload = {};

  try {
    await ensureDirs();

    browser = await chromium.launch({ headless: true });
    const context = await browser.newContext({ viewport: { width: 1560, height: 980 } });
    const page = await context.newPage();

    await page.goto(`${FRONTEND_URL}/login`, { waitUntil: "networkidle" });
    await page.locator('input[placeholder*="用户名"], input[type="text"]').first().fill("admin");
    await page.locator('input[type="password"]').first().fill("admin");
    await page.locator(".login-btn").click();
    await page.waitForURL("**/dashboard/**", { timeout: 15000 });
    addCase("TC-EXT-AUTH-001", "登录进入控制台", "PASS", "", await saveShot(page, "01_login_success"));

    originalTheme = await page.evaluate(() => localStorage.getItem("theme") || "light");
    const token = await page.evaluate(() => localStorage.getItem("token"));
    if (!token) {
      throw new Error("登录后未获取 token");
    }

    api = await request.newContext({
      baseURL: BACKEND_API,
      extraHTTPHeaders: { Authorization: `Bearer ${token}` }
    });

    const settingsResp = await api.get("/settings");
    const settingsData = await apiJson(settingsResp);
    originalSettingsPayload = settingsToPayload(normalizeData(settingsData) || {});

    await page.goto(`${FRONTEND_URL}/dashboard/settings`, { waitUntil: "networkidle" });
    await page.locator(".header-title").filter({ hasText: "系统配置" }).waitFor({ timeout: 15000 });
    const alistValue = `http://127.0.0.1:${Math.floor(4000 + Math.random() * 500)}`;
    const proxyDomainsValue = "github,tmdb";
    await page.locator('input[placeholder*="192.168.1.100:5244"]').fill(alistValue);
    await page.locator('.el-input-number input').first().fill("9");
    await page.locator('textarea[placeholder*="tg,github"]').fill(proxyDomainsValue);
    await page.locator(".form-actions .el-button--primary").last().click();
    await page.waitForTimeout(1000);

    const savedSettingsResp = await api.get("/settings");
    const savedSettingsData = normalizeData(await apiJson(savedSettingsResp)) || {};
    const settingsPass = savedSettingsData.alist_url === alistValue
      && String(savedSettingsData.log_save_day_limit) === "9"
      && savedSettingsData.proxy_domains === proxyDomainsValue;
    addCase(
      "TC-EXT-SET-001",
      "系统设置保存并回读成功",
      settingsPass ? "PASS" : "FAIL",
      `alist=${savedSettingsData.alist_url || ""} days=${savedSettingsData.log_save_day_limit || ""} domains=${savedSettingsData.proxy_domains || ""}`,
      await saveShot(page, "02_settings_saved")
    );

    await page.goto(`${FRONTEND_URL}/dashboard/category-strategy`, { waitUntil: "networkidle" });
    await page.locator(".category-hero").waitFor({ timeout: 15000 });
    await page.locator(".hero-actions .el-button--primary").click();
    const categoryName = `扩展覆盖分类_${stamp}`;
    const categoryPath = `/电影/扩展覆盖_${stamp}`;
    await page.locator('input[placeholder="例如：国漫"]').fill(categoryName);
    await page.locator('input[placeholder="/电视剧/国漫"]').fill(categoryPath);
    await page.locator(".editor-footer .el-button--primary").click();
    await page.waitForTimeout(1000);

    const categoryListResp = await api.get("/media/categories");
    const categoryPayload = normalizeData(await apiJson(categoryListResp)) || {};
    const createdCategory = (categoryPayload.data || []).find((item) => item.name === categoryName);
    if (createdCategory) {
      createdCategoryId = createdCategory.id;
    }
    addCase(
      "TC-EXT-CAT-001",
      "分类策略创建成功",
      createdCategory ? "PASS" : "FAIL",
      createdCategory ? `id=${createdCategory.id}` : "未在接口中查询到新分类",
      await saveShot(page, "03_category_created")
    );

    if (createdCategory) {
      await page.locator(".strategy-item").filter({ hasText: categoryName }).click();
      const editedPath = `${categoryPath}_编辑`;
      await page.locator('input[placeholder="/电视剧/国漫"]').fill(editedPath);
      await page.locator('.editor-actions .el-switch').click();
      await page.locator(".editor-footer .el-button--primary").click();
      await page.waitForTimeout(1000);

      const updatedResp = await api.get("/media/categories");
      const updatedPayload = normalizeData(await apiJson(updatedResp)) || {};
      const updatedCategory = (updatedPayload.data || []).find((item) => item.id === createdCategory.id);
      const editPass = updatedCategory?.target_path === editedPath && updatedCategory?.enabled === false;
      addCase(
        "TC-EXT-CAT-002",
        "分类策略编辑成功",
        editPass ? "PASS" : "FAIL",
        updatedCategory ? `path=${updatedCategory.target_path} enabled=${updatedCategory.enabled}` : "未查到更新后的分类",
        await saveShot(page, "04_category_updated")
      );

      await page.locator(".editor-actions .el-button--danger").click();
      await confirmPrimaryAction(page);
      await page.waitForTimeout(1000);

      const deletedResp = await api.get("/media/categories");
      const deletedPayload = normalizeData(await apiJson(deletedResp)) || {};
      const stillExists = (deletedPayload.data || []).some((item) => item.id === createdCategory.id);
      addCase(
        "TC-EXT-CAT-003",
        "分类策略删除成功",
        stillExists ? "FAIL" : "PASS",
        stillExists ? "删除后接口仍返回该分类" : "",
        await saveShot(page, "05_category_deleted")
      );
      if (!stillExists) {
        createdCategoryId = null;
      }
    } else {
      addCase("TC-EXT-CAT-002", "分类策略编辑成功", "SKIP", "创建失败后跳过");
      addCase("TC-EXT-CAT-003", "分类策略删除成功", "SKIP", "创建失败后跳过");
    }

    await page.goto(`${FRONTEND_URL}/dashboard/strm-config`, { waitUntil: "networkidle" });
    await page.locator(".header-title").filter({ hasText: "STRM 文件配置中心" }).waitFor({ timeout: 15000 });
    await page.locator(".card-header .el-button--primary").click();
    const strmDialog = page.locator(".el-dialog").filter({ hasText: "新增配置" }).last();
    await strmDialog.waitFor({ timeout: 10000 });
    addCase("TC-EXT-STRM-001", "STRM 配置页新增弹窗可打开", "PASS", "", await saveShot(page, "06_strm_dialog"));
    await strmDialog.locator(".dialog-footer .el-button").first().click();

    const summary = {
      runId,
      total: cases.length,
      passCount: cases.filter((item) => item.status === "PASS").length,
      failCount: cases.filter((item) => item.status === "FAIL").length,
      skipCount: cases.filter((item) => item.status === "SKIP").length,
      screenshots: runScreenshotDir,
      report: reportFile
    };

    const lines = [];
    lines.push("# 扩展覆盖测试报告");
    lines.push("");
    lines.push(`- 执行时间：${new Date().toLocaleString("zh-CN", { hour12: false })}`);
    lines.push(`- 执行者：Codex`);
    lines.push(`- 前端地址：${FRONTEND_URL}`);
    lines.push(`- 后端地址：${BACKEND_API}`);
    lines.push(`- 截图目录：${runScreenshotDir}`);
    lines.push("");
    lines.push("## 汇总");
    lines.push("");
    lines.push(`- 总用例：${summary.total}`);
    lines.push(`- 通过：${summary.passCount}`);
    lines.push(`- 失败：${summary.failCount}`);
    lines.push(`- 跳过：${summary.skipCount}`);
    lines.push("");
    lines.push("## 明细");
    lines.push("");
    lines.push("| 用例ID | 名称 | 结果 | 说明 | 证据 |");
    lines.push("| --- | --- | --- | --- | --- |");
    for (const item of cases) {
      lines.push(`| ${item.id} | ${item.name} | ${item.status} | ${item.detail || "-"} | ${item.artifact || "-"} |`);
    }
    await fs.writeFile(reportFile, `${lines.join("\n")}\n`, "utf8");

    console.log(JSON.stringify(summary, null, 2));
  } catch (error) {
    addCase("TC-EXT-RUN-000", "扩展覆盖执行异常", "FAIL", String(error));
    const lines = [];
    lines.push("# 扩展覆盖测试报告");
    lines.push("");
    lines.push(`- 执行异常：${String(error)}`);
    lines.push("");
    lines.push("| 用例ID | 名称 | 结果 | 说明 | 证据 |");
    lines.push("| --- | --- | --- | --- | --- |");
    for (const item of cases) {
      lines.push(`| ${item.id} | ${item.name} | ${item.status} | ${item.detail || "-"} | ${item.artifact || "-"} |`);
    }
    await fs.writeFile(reportFile, `${lines.join("\n")}\n`, "utf8");
    console.log(JSON.stringify({ report: reportFile, screenshots: runScreenshotDir }, null, 2));
    process.exitCode = 1;
  } finally {
    if (api) {
      if (createdCategoryId) {
        await api.delete(`/media/categories/${createdCategoryId}`).catch(() => {});
      }
      if (Object.keys(originalSettingsPayload).length > 0) {
        await api.put("/settings", { data: originalSettingsPayload }).catch(() => {});
      }
      await api.dispose().catch(() => {});
    }

    if (browser) {
      if (originalTheme) {
        try {
          const restorePage = await browser.newPage();
          await restorePage.goto(`${FRONTEND_URL}/login`, { waitUntil: "domcontentloaded" });
          await restorePage.evaluate((theme) => {
            localStorage.setItem("theme", theme);
            document.documentElement.classList.toggle("dark", theme === "dark");
          }, originalTheme);
          await restorePage.close();
        } catch {
          // ignore theme restore issues
        }
      }
      await browser.close();
    }
  }
}

run();
