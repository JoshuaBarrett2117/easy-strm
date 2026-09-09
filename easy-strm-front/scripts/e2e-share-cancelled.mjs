// 更新日期：2026-09-08；执行者：Codex。验证持久化字段回显及恢复状态，无真实API写入。
import assert from 'node:assert/strict'
import { chromium } from 'playwright'
const browser = await chromium.launch({ headless: true })
try {
 const page = await browser.newPage()
 let cancelled = true
 await page.addInitScript(() => localStorage.setItem('token', 'e2e-token'))
 await page.route('**/api/**', async route => {
  const path = new URL(route.request().url()).pathname
  if (!path.startsWith('/api/')) return route.continue()
  const data = path === '/api/media/share-records' ? {data:[
   {id:1,name:'取消分享',url:'https://115.com/s/a',share_cancelled:cancelled,media:[]},
   {id:2,name:'超时分享',url:'https://115.com/s/b',share_cancelled:false,media:[]}
  ],total:2} : {data:[],total:0}
  await route.fulfill({json:{code:0,data}})
 })
 await page.goto('http://127.0.0.1:3001/dashboard/share-records')
 await page.getByText('分享已取消',{exact:true}).waitFor()
 assert.equal(await page.getByText('分享已取消',{exact:true}).count(),1)
 await page.reload()
 await page.getByText('分享已取消',{exact:true}).waitFor()
 cancelled=false
 await page.reload()
 await page.getByText('超时分享',{exact:true}).waitFor()
 assert.equal(await page.getByText('分享已取消',{exact:true}).count(),0)
 console.log('PASS: 取消标签、刷新保留、超时不标记、恢复移除；API mocked')
} finally { await browser.close() }

