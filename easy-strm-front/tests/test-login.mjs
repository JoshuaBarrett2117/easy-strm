import { chromium } from 'playwright';

async function test() {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  
  console.log('访问登录页面...');
  await page.goto('http://localhost:3001/login', { waitUntil: 'networkidle', timeout: 15000 });
  
  console.log('输入用户名密码...');
  await page.fill('input[placeholder="请输入用户名"]', 'admin');
  await page.fill('input[placeholder="请输入密码"]', 'admin123');
  
  console.log('点击登录...');
  await page.click('button:has-text("登录系统")');
  
  await page.waitForTimeout(3000);
  console.log('当前URL:', page.url());
  console.log('页面标题:', await page.title());
  
  const content = await page.textContent('body');
  console.log('页面包含dashboard:', content.includes('dashboard') || content.includes('Dashboard') || content.includes('用户信息'));
  
  await browser.close();
}

test().catch(console.error);
