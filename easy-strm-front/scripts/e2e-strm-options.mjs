// 2026-09-12 Codex：确认清空默认关闭，用户选择随请求提交。
import assert from 'node:assert/strict'
import { chromium } from 'playwright'
const browser = await chromium.launch({headless:true})
try {
 const page=await browser.newPage()
 page.on('pageerror',e=>console.log('PAGE ERROR:',e.message))
 await page.addInitScript(()=>localStorage.setItem('token','test'))
 const requests=[]
 let savedSchedule
 await page.route('**/api/**',async route=>{
  const p=new URL(route.request().url()).pathname
  if(!p.startsWith('/api/'))return route.continue()
  if(p==='/api/cron/handlers')return route.fulfill({json:{data:[{key:'share_strm_incremental_export',name:'分享库 STRM 增量导出',parameters:[]}]}})
  if(p==='/api/cron/task' && route.request().method()==='POST'){savedSchedule=route.request().postDataJSON();return route.fulfill({json:{data:{id:10}}})}
  if(p.endsWith('/generate/full')) {requests.push(route.request().postDataJSON());return route.fulfill({json:{task_id:'test-task'}})}
  if(p.includes('/strm/task/'))return route.fulfill({json:{data:{status:'completed'}}})
  if(p.includes('/strm/config'))return route.fulfill({json:{data:[{id:1,cloud115_id:1,net_disk_path:'/视频',local_path:'/test/strm',extension:'.mkv'}]}})
  return route.fulfill({json:{data:[]}})
 })
 await page.goto((process.env.E2E_BASE_URL || 'http://127.0.0.1:3001')+'/dashboard/strm-config')
 await page.waitForLoadState('networkidle')
 for(const clear of [false,true]){
  await page.getByRole('button',{name:'全量生成',exact:true}).click()
  const box=page.getByRole('checkbox',{name:'生成前清空目标目录'})
  await box.waitFor();assert.equal(await box.isChecked(),false)
  if(clear)await box.check()
  const response=page.waitForResponse(r=>r.url().includes('/strm/task/'))
  await page.getByRole('button',{name:'生成',exact:true}).click()
  await response
 }
 assert.deepEqual(requests,[{clear_before_generate:false},{clear_before_generate:true}])
 console.log('PASS: 全量生成默认保留，勾选后提交清空参数')
 await page.goto((process.env.E2E_BASE_URL || 'http://127.0.0.1:3001')+'/dashboard/scheduled-tasks')
 await page.getByRole('button',{name:'新建任务',exact:true}).click()
 await page.locator('.n-modal input').first().fill('分享库每日增量')
 const saved=page.waitForResponse(r=>r.url().endsWith('/cron/task') && r.request().method()==='POST')
 await page.getByRole('button',{name:'保存',exact:true}).click();await saved
 assert.equal(savedSchedule.handler,'share_strm_incremental_export');assert.deepEqual(savedSchedule.params,{})
 console.log('PASS: 可创建分享库增量定时任务')
} finally {await browser.close()}
