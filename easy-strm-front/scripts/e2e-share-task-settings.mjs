// 更新日期：2026-09-08；执行者：Codex。验证无限制与自定义任务时限设置。
import assert from 'node:assert/strict'
import { chromium } from 'playwright'
const browser=await chromium.launch({headless:true})
try{
 const page=await browser.newPage()
 let minutes=0
 await page.addInitScript(()=>localStorage.setItem('token','test'))
 await page.route('**/api/**',async route=>{
  const req=route.request(),path=new URL(req.url()).pathname
  if(!path.startsWith('/api/'))return route.continue()
  let data={data:[],total:0}
  if(path==='/api/media/share-task-settings'){
   if(req.method()==='PUT')minutes=req.postDataJSON().timeout_minutes
   data={timeout_minutes:minutes}
  }
  await route.fulfill({json:{data}})
 })
 await page.goto('http://localhost:3001/dashboard/share-records')
 const open=async()=>{
  await page.getByRole('button',{name:'识别任务设置',exact:true}).click()
  await page.getByText('无限制',{exact:true}).waitFor()
 }
 await open()
 await page.locator('.n-modal .n-select').click()
 await page.locator('.n-base-select-option').filter({hasText:'自定义时限'}).click()
 await page.locator('.n-modal .n-input-number input').fill('90')
 await page.getByRole('button',{name:'保存设置',exact:true}).click()
 await page.locator('.n-modal').waitFor({state:'hidden'})
 assert.equal(minutes,90)
 await page.getByRole('button',{name:'识别任务设置',exact:true}).click()
 await page.locator('.n-modal .n-input-number input').waitFor()
 assert.equal(await page.locator('.n-modal .n-input-number input').inputValue(),'90')
 await page.locator('.n-modal .n-select').click()
 await page.locator('.n-base-select-option').filter({hasText:'无限制'}).click()
 await page.getByRole('button',{name:'保存设置',exact:true}).click()
 await page.locator('.n-modal').waitFor({state:'hidden'})
 assert.equal(minutes,0)
 await page.reload();await open()
 assert.equal(await page.locator('.n-modal .n-input-number').count(),0)
 console.log('PASS: 默认无限制、自定义90分钟保存重开、切回无限制持久化、刷新回显；API mocked')
}finally{await browser.close()}

