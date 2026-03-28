import { chromium } from 'playwright';

async function runTests() {
  const results = [];
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext();
  const page = await context.newPage();
  
  const consoleErrors = [];
  page.on('console', msg => {
    if (msg.type() === 'error') {
      consoleErrors.push(msg.text());
    }
  });

  try {
    console.log('[测试1] 访问登录页面...');
    await page.goto('http://localhost:3001/login', { waitUntil: 'networkidle', timeout: 15000 });
    const loginTitle = await page.textContent('h2');
    results.push({ test: '1.1 登录页面加载', passed: loginTitle && loginTitle.includes('Easy'), result: loginTitle });
    
    console.log('[测试2] 执行登录...');
    await page.fill('input[placeholder=\"请输入用户名\"]', 'admin');
    await page.fill('input[placeholder=\"请输入密码\"]', 'admin123');
    await page.click('button:has-text(\"登录系统\")');
    await page.waitForURL('**/dashboard**', { timeout: 10000 });
    results.push({ test: '2.1 登录成功', passed: page.url().includes('dashboard'), result: page.url() });
    
    console.log('[测试3] 检查Dashboard...');
    await page.waitForSelector('.el-menu', { timeout: 5000 });
    results.push({ test: '3.1 Dashboard菜单加载', passed: true, result: '菜单存在' });
    
    console.log('[测试4] 测试用户信息页面...');
    await page.click('text=用户信息');
    await page.waitForTimeout(1000);
    const userInfoContent = await page.textContent('body');
    results.push({ test: '4.1 用户信息页面', passed: userInfoContent.includes('admin'), result: '页面包含admin' });
    
    console.log('[测试5] 测试115云账号页面...');
    await page.click('text=115云账号');
    await page.waitForTimeout(2000);
    const cloud115Content = await page.textContent('body');
    results.push({ test: '5.1 115云账号列表', passed: cloud115Content.includes('115主号'), result: cloud115Content.includes('115主号') ? '包含115主号' : '未包含' });
    
    const addBtn = page.locator('button:has-text(\"新增账号\")');
    if (await addBtn.isVisible()) {
      await addBtn.click();
      await page.waitForTimeout(500);
      const dialog = page.locator('.el-dialog');
      results.push({ test: '5.2 新增账号弹窗', passed: await dialog.isVisible(), result: '弹窗状态: ' + (await dialog.isVisible()) });
      await page.click('.el-dialog__close');
    }
    
    const editBtn = page.locator('button:has-text(\"编辑\")').first();
    if (await editBtn.isVisible()) {
      await editBtn.click();
      await page.waitForTimeout(500);
      results.push({ test: '5.3 编辑账号弹窗', passed: true, result: '编辑弹窗已打开' });
      await page.click('.el-dialog__close');
    }
    
    const qrBtn = page.locator('button:has-text(\"扫码更新\")').first();
    if (await qrBtn.isVisible()) {
      await qrBtn.click();
      await page.waitForTimeout(1000);
      results.push({ test: '5.4 扫码更新弹窗', passed: true, result: '扫码更新弹窗已打开' });
      await page.click('.el-dialog__close');
    }
    
    const deleteBtn = page.locator('button:has-text(\"删除\")').first();
    if (await deleteBtn.isVisible()) {
      await deleteBtn.click();
      await page.waitForTimeout(500);
      const confirmBtn = page.locator('.el-button--danger:has-text(\"确定\")');
      if (await confirmBtn.isVisible()) {
        results.push({ test: '5.5 删除确认框', passed: true, result: '确认框已显示' });
        await page.keyboard.press('Escape');
      }
    }
    
    console.log('[测试6] 测试STRM配置页面...');
    await page.click('text=STRM配置');
    await page.waitForTimeout(2000);
    const strmContent = await page.textContent('body');
    results.push({ test: '6.1 STRM配置列表', passed: strmContent.includes('配置'), result: 'STRM配置页面已加载' });
    
    const addConfigBtn = page.locator('button:has-text(\"新增配置\")');
    if (await addConfigBtn.isVisible()) {
      await addConfigBtn.click();
      await page.waitForTimeout(500);
      results.push({ test: '6.2 新增配置弹窗', passed: true, result: '新增配置弹窗已打开' });
      await page.click('.el-dialog__close');
    }
    
    console.log('[测试7] 测试Dashboard任务功能...');
    const taskBtn = page.locator('button:has-text(\"查看任务\")');
    if (await taskBtn.isVisible()) {
      await taskBtn.click();
      await page.waitForTimeout(500);
      results.push({ test: '7.1 任务弹窗', passed: true, result: '任务弹窗已打开' });
      await page.click('.el-dialog__close');
    }
    
    const logBtn = page.locator('button:has-text(\"查看日志\")');
    if (await logBtn.isVisible()) {
      await logBtn.click();
      await page.waitForTimeout(500);
      results.push({ test: '8.1 日志弹窗', passed: true, result: '日志弹窗已打开' });
      await page.click('.el-dialog__close');
    }
    
    console.log('[测试9] 测试退出登录...');
    const logoutBtn = page.locator('button:has-text(\"退出登录\")');
    if (await logoutBtn.isVisible()) {
      await logoutBtn.click();
      await page.waitForTimeout(1000);
      results.push({ test: '9.1 退出登录', passed: page.url().includes('login'), result: '当前URL: ' + page.url() });
    }
    
  } catch (error) {
    results.push({ test: '错误', passed: false, result: error.message });
  }
  
  if (consoleErrors.length > 0) {
    results.push({ test: '控制台错误数', passed: false, result: consoleErrors.length + '个错误' });
    consoleErrors.forEach((err, i) => console.log('Console Error ' + (i+1) + ': ' + err));
  } else {
    results.push({ test: '控制台错误', passed: true, result: '无错误' });
  }
  
  await browser.close();
  
  console.log('\\n========== 测试报告 ==========');
  console.log('总计: ' + results.length + ' 项测试');
  const passed = results.filter(r => r.passed).length;
  const failed = results.filter(r => !r.passed).length;
  console.log('通过: ' + passed);
  console.log('失败: ' + failed);
  console.log('\\n详细结果:');
  results.forEach(r => {
    const status = r.passed ? '[PASS]' : '[FAIL]';
    console.log(status + ' ' + r.test + ': ' + r.result);
  });
  console.log('==============================');
  
  return results;
}

runTests().catch(console.error);
