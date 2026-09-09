// 更新日期：2026-09-08；执行者：Codex。只通过模拟接口验证清空确认、失败恢复与重新识别。
import assert from 'node:assert/strict'
import { chromium } from 'playwright'
const browser=await chromium.launch({headless:true})
try {
 const page=await browser.newPage()
 let deletes=0, identifies=0, fail=false
 let media=[{id:1,file_name:'重复单集',status:'identified',result:{success:true,title:'剧集',media_type:'tv',tmdb_id:1}}]
 await page.addInitScript(()=>localStorage.setItem('token','test'))
 await page.route('**/api/**',async route=>{
  const request=route.request(),path=new URL(request.url()).pathname
  if(!path.startsWith('/api/'))return route.continue()
  if(path==='/api/media/share-records/9/media' && request.method()==='DELETE'){
   deletes++
   if(fail)return route.fulfill({status:409,json:{error:'有分享识别任务正在运行'}})
   media=[]
   return route.fulfill({json:{data:{deleted:1}}})
  }
  if(path==='/api/media/share-records/9/identify'){
   identifies++
   return route.fulfill({json:{data:{task_id:'test-task'}}})
  }
  await route.fulfill({json:{data:path.includes('test-task')?{task_id:'test-task',status:'completed'}:{data:path==='/api/media/share-records'?[{id:9,name:'综艺包',media}]:[],total:1}}})
 })
 await page.goto('http://127.0.0.1:3001/dashboard/share-records')
 const open=()=>page.getByRole('button',{name:'清空识别内容',exact:true}).click()
 await open()
 await page.getByRole('button',{name:'取消',exact:true}).click()
 assert.equal(deletes,0)
 await open()
 fail=true
 await page.getByRole('button',{name:'确认清空',exact:true}).click()
 await page.getByText('有分享识别任务正在运行',{exact:true}).waitFor()
 assert.equal(await page.getByRole('button',{name:'确认清空',exact:true}).isVisible(),true)
 fail=false
 await page.getByRole('button',{name:'确认清空',exact:true}).click()
 await page.getByText('总数 0',{exact:true}).waitFor()
 assert.equal(deletes,2)
 assert.equal(await page.getByText('综艺包',{exact:true}).count(),1)
 await page.getByRole('button',{name:'识别',exact:true}).click()
 await page.getByText('任务 test-task',{exact:false}).first().waitFor()
 assert.equal(identifies,1)
 console.log('PASS: 取消不请求、失败保留确认、清空刷新0、保留分享、重新识别；API mocked')
}finally{await browser.close()}

