import fs from "node:fs/promises";
import path from "node:path";
import { chromium } from "playwright";

const FRONTEND_URL = "http://localhost:3001";
const BACKEND_API = "http://127.0.0.1:8082";
const DEBUG_DIR = path.resolve(process.cwd(), "..", "debug");
const stamp = new Date().toISOString().replace(/[:.]/g, "-");

const results = [];
const record = (id, name, pass, detail) => {
  results.push({ id, name, pass, detail });
  console.log(`${pass ? "PASS" : "FAIL"} [${id}] ${name} - ${detail}`);
};

function extractToken(obj) {
  const s = JSON.stringify(obj);
  const m = s.match(/eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/);
  return m ? m[0] : null;
}

async function main() {
  await fs.mkdir(DEBUG_DIR, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1560, height: 980 } });
  const page = await context.newPage();
  const api = await (await import("playwright")).request.newContext({
    baseURL: BACKEND_API,
    extraHTTPHeaders: { "Content-Type": "application/json" },
  });

  let token = null;
  let testSourceId = null;
  const tempDir = path.resolve(process.cwd(), "..", "debug", `browse-test-${Date.now()}`);

  try {
    // ---------- Login ----------
    await page.goto(`${FRONTEND_URL}/login`, { waitUntil: "networkidle" });
    await page.getByPlaceholder("用户名").fill("admin");
    await page.getByPlaceholder("密码").fill("admin");
    await page.getByRole("button", { name: "进入工作台" }).click();
    await page.waitForURL("**/dashboard/**", { timeout: 15000 });
    token = await page.evaluate(() => localStorage.getItem("token"));
    if (token) api._token = token; // store for later header injection
    record("auth", "登录成功并获取 token", !!token, token ? "token 已获取" : "未获取 token");

    // Helper for authed API
    const authHeader = { Authorization: `Bearer ${token}`, "Content-Type": "application/json" };
    const authedFetch = async (url, opts = {}) =>
      fetch(BACKEND_API + url, { ...opts, headers: { ...authHeader, ...(opts.headers || {}) } });

    // ========== ISSUE 2: TMDB 配置回显 ==========
    try {
      const testKey = "QA_VERIFY_KEY_12345678";
      const postResp = await authedFetch("/media/tmdb/config", {
        method: "POST",
        body: JSON.stringify({ api_key: testKey, language: "zh-CN" }),
      });
      const postJson = await postResp.json();
      const getResp = await authedFetch("/media/tmdb/config");
      const getJson = await getResp.json();
      const cfg = getJson?.data || getJson;
      const masked = cfg?.api_key || "";
      const hasKey = cfg?.has_key === true;
      const lang = cfg?.language;
      const echoOk = hasKey && masked.includes("****") && masked.startsWith("QA_V") && lang === "zh-CN";
      record(
        "issue2-api",
        "TMDB 配置持久化并回显 (API)",
        echoOk,
        `has_key=${hasKey}, masked=${masked}, language=${lang}`
      );

      // UI 回显验证
      await page.goto(`${FRONTEND_URL}/dashboard/settings`, { waitUntil: "networkidle" });
      await page.locator(".n-tabs-tab", { hasText: "TMDB" }).first().click().catch(async () => {
        await page.getByRole("tab", { name: "TMDB" }).click();
      });
      await page.waitForTimeout(500);
      const tmdbInput = page.locator(".n-form-item").filter({ hasText: "TMDB API Key" }).locator("input").first();
      const placeholder = await tmdbInput.getAttribute("placeholder");
      const uiEchoOk = placeholder && placeholder.includes("当前已配置") && placeholder.includes("QA_V");
      record("issue2-ui", "TMDB 配置回显到设置页 (UI)", !!uiEchoOk, `placeholder=${placeholder}`);
      await page.screenshot({ path: path.join(DEBUG_DIR, `qa-tmdb-${stamp}.png`), fullPage: true });
    } catch (e) {
      record("issue2", "TMDB 回显测试异常", false, e.message);
    }

    // ========== ISSUE 3: 媒体源被禁用 -> 仍可浏览 ==========
    try {
      await fs.mkdir(tempDir, { recursive: true });
      await fs.writeFile(path.join(tempDir, "sample.mkv"), "x", "utf8");
      const createResp = await authedFetch("/media/sources", {
        method: "POST",
        body: JSON.stringify({
          name: `qa_disabled_${Date.now()}`,
          source_type: "local",
          path: tempDir,
          organize_target_path: tempDir,
          auto_organize: false,
          watch_enabled: false,
          watch_interval: 300,
          emby_library_id: "",
          enabled: false,
          priority: 10,
        }),
      });
      const createJson = await createResp.json();
      testSourceId = createJson?.data?.id || createJson?.data?.data?.id || null;

      // API 层：禁用源浏览不应返回“媒体源已禁用”
      const filesResp = await authedFetch(`/media/files?source_id=${testSourceId}`);
      const filesBody = await filesResp.text();
      const apiNoDisabled = filesResp.ok && !filesBody.includes("媒体源已禁用");
      record(
        "issue3-api",
        "禁用媒体源可浏览 (API 不报已禁用)",
        apiNoDisabled,
        `status=${filesResp.status}, bodyHasDisabledErr=${filesBody.includes("媒体源已禁用")}`
      );

      // UI 层：进入文件工作台，点击该源“浏览”
      await page.goto(`${FRONTEND_URL}/dashboard/media-manager`, { waitUntil: "networkidle" });
      const row = page.getByRole("row").filter({ hasText: `qa_disabled_` });
      await row.first().waitFor({ timeout: 15000 });
      await row.first().getByRole("button", { name: "浏览" }).click();
      const dialog = page.getByRole("dialog").filter({ hasText: "文件浏览" });
      await dialog.first().waitFor({ timeout: 15000 });
      const dialogText = await dialog.first().innerText();
      const uiNoDisabled = !dialogText.includes("媒体源已禁用");
      const uiShowsFiles = dialogText.includes("sample.mkv") || dialogText.includes("暂无");
      record(
        "issue3-ui",
        "禁用媒体源在 UI 可打开文件浏览",
        uiNoDisabled && uiShowsFiles,
        `noDisabledErr=${uiNoDisabled}, showsFilesOrEmpty=${uiShowsFiles}`
      );
      await page.screenshot({ path: path.join(DEBUG_DIR, `qa-browse-disabled-${stamp}.png`), fullPage: true });
    } catch (e) {
      record("issue3", "媒体源禁用测试异常", false, e.message);
    }

    // ========== ISSUE 1: 115 转存预设配置持久化 ==========
    try {
      await page.goto(`${FRONTEND_URL}/dashboard/resources/transfer`, { waitUntil: "networkidle" });
      const presetVisible = await page.getByText("转存默认配置").isVisible();
      record("issue1-ui-present", "115 转存预设配置区存在", presetVisible, presetVisible ? "已渲染预设区" : "未找到");

      // 打开“自动整理”开关（preset 区第一个 switch）
      const firstSwitch = page.locator(".preset-section .n-switch").first();
      await firstSwitch.click();
      await page.waitForTimeout(300);
      // 保存配置
      await page.getByRole("button", { name: /保存配置/ }).click();
      await page.waitForTimeout(600);

      // 重新加载，验证 localStorage 持久化
      await page.reload({ waitUntil: "networkidle" });
      const preset = await page.evaluate(() => localStorage.getItem("easy-strm-115-transfer-preset"));
      let persisted = false;
      let parsed = null;
      try {
        parsed = preset ? JSON.parse(preset) : null;
        persisted = !!parsed && parsed.autoOrganize === true;
      } catch {}
      record(
        "issue1-persist",
        "115 转存预设保存到 localStorage 并恢复",
        persisted,
        `localStorage=${preset}`
      );
      await page.screenshot({ path: path.join(DEBUG_DIR, `qa-transfer-${stamp}.png`), fullPage: true });
    } catch (e) {
      record("issue1", "115 转存预设测试异常", false, e.message);
    }

    // ========== ISSUE 4: 仪表盘空白 ==========
    try {
      await page.goto(`${FRONTEND_URL}/dashboard/home`, { waitUntil: "load" });
      await page.waitForTimeout(2500);
      const bodyText = await page.locator("body").innerText();
      // 趋势区：无数据时显示空状态文案；有数据时显示柱状图（均非大片空白）
      const hasTrendEmpty = bodyText.includes("暂无近 7 天整理趋势数据");
      const trendBars = await page.locator("span.bg-gradient-to-b").count();
      const hasTrendContent = hasTrendEmpty || trendBars > 0;
      const hasHero = bodyText.includes("让整理流程简单清晰") || bodyText.includes("从媒体源到 STRM");
      const hasStats = bodyText.includes("启用媒体源") || bodyText.includes("媒体源");
      record(
        "issue4",
        "仪表盘渲染正常且无大片空白图表",
        hasHero && hasStats && hasTrendContent,
        `hero=${hasHero}, stats=${hasStats}, trendEmpty=${hasTrendEmpty}, trendBars=${trendBars}`
      );
      await page.screenshot({ path: path.join(DEBUG_DIR, `qa-dashboard-${stamp}.png`), fullPage: true });
    } catch (e) {
      record("issue4", "仪表盘测试异常", false, e.message);
    }
  } finally {
    // 清理禁用测试源
    if (testSourceId && token) {
      try {
        await fetch(`${BACKEND_API}/media/sources/${testSourceId}`, {
          method: "DELETE",
          headers: { Authorization: `Bearer ${token}` },
        });
      } catch {}
    }
    await api.dispose().catch(() => {});
    await browser.close().catch(() => {});
  }

  const passed = results.filter((r) => r.pass).length;
  const failed = results.filter((r) => !r.pass).length;
  console.log("\n==== SUMMARY ====");
  console.log(JSON.stringify({ total: results.length, passed, failed, results }, null, 2));
  process.exit(failed > 0 ? 1 : 0);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
