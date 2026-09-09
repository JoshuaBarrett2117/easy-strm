// 更新日期：2026-09-08；执行者：Codex。使用列表接口夹具验证同剧集折叠及原记录可操作。
import assert from 'node:assert/strict'
import { chromium } from 'playwright'
const browser=await chromium.launch({headless:true})
try {
 let savedSize=10
 const page=await browser.newPage()
 await page.addInitScript(()=>localStorage.setItem('token','test'))
 await page.route('**/api/**',async route=>{
  const url=new URL(route.request().url()), path=url.pathname
  if(!path.startsWith('/api/'))return route.continue()
  assert.notEqual(path,'/api/media/share-gallery-settings')
  if(path==='/api/media/share-records/batch-identify') {
   assert.deepEqual(route.request().postDataJSON(),{retry_failed:false,pending_only:true})
   return route.fulfill({json:{data:{task_id:'pending-test'}}})
  }

  if(path==='/api/media/share-records/1/identify') {
   assert.ok((url.searchParams.get('pending_only')==='true') !== (url.searchParams.get('failed_only')==='true'))
   return route.fulfill({json:{data:{task_id:'record-pending-test'}}})
  }
  const media=Array.from({length:23},(_,i)=>i+1).map(id=>({id,file_name:`综艺/一路繁花.S01E0${id}.mp4`,status:'identified',gallery_duplicate:id>1,result:{success:true,tmdb_id:1,media_type:'tv',title:'一路繁花'}}))
  const expanded=url.searchParams.get('show_duplicates')==='true'
  const gallery=path==='/api/media/share-records/1/media'
  if(gallery)assert.equal(Number(url.searchParams.get('page_size')),savedSize)
  await route.fulfill({json:{data:{data:path==='/api/media/share-records'?[{id:1,name:'爱影综艺包',identified_count:23,media_count:24,pending_count:1,failed_count:1}]:gallery?(expanded?media.slice((Number(url.searchParams.get('page'))-1)*savedSize,Number(url.searchParams.get('page'))*savedSize):media.slice(0,1)):[],total:gallery?(expanded?23:1):1,duplicate_count:22}}})
 })
 await page.goto('http://127.0.0.1:3001/dashboard/share-records')
 await page.getByText('爱影综艺包',{exact:true}).waitFor()
 const toolbar=page.locator('.share-toolbar').first()
 assert.deepEqual(await toolbar.getByRole('button').allTextContents(),['新增分享','批量导入分享','导入记录','批量自动识别','重新识别失败项','继续识别待识别内容','识别任务设置'])
 const started=page.waitForResponse(r=>r.url().includes('/share-records/batch-identify'))
 await page.getByRole('button',{name:'继续识别待识别内容',exact:true}).click()
 await started
 const resumed=page.waitForResponse(r=>r.url().includes('/share-records/1/identify'))
 await page.getByRole('button',{name:'继续识别待识别',exact:true}).click()
 await resumed
 const retried=page.waitForResponse(r=>r.url().includes('/share-records/1/identify') && r.url().includes('failed_only=true'))
 await page.getByRole('button',{name:'识别失败内容',exact:true}).click()
 await retried

 await page.locator('.n-data-table-expand-trigger').click()
 await page.locator('.media-card').first().waitFor()
 assert.equal(await page.locator('.media-card').count(),1)
 await page.getByRole('button',{name:'展开同剧集记录（22）',exact:true}).click()
 await page.locator('.media-card').nth(2).waitFor()
 assert.equal(await page.locator('.media-card').count(),10)
 assert.equal(await page.getByRole('button',{name:'手动识别',exact:true}).count(),10)
 await page.locator('.share-gallery .n-pagination-item').filter({hasText:/^2$/}).click()
 await page.locator('.media-card .directory').filter({hasText:'S01E011.mp4'}).waitFor()
 assert.equal(await page.locator('.media-card').count(),10)
 await page.locator('.share-gallery .n-pagination-item').filter({hasText:/^3$/}).click()
 await page.locator('.media-card .directory').filter({hasText:'S01E021.mp4'}).waitFor()
 assert.equal(await page.locator('.media-card').count(),3)
 await page.getByRole('button',{name:'合并相同剧集',exact:true}).click()
 await page.locator('.media-card').nth(2).waitFor({state:'detached'})
 await page.locator('.media-card').first().waitFor()
 assert.equal(await page.locator('.media-card').count(),1)
 await page.locator('.share-gallery .n-input-number input').fill('17')
 savedSize=17
 const resized=page.waitForResponse(r=>r.url().includes('/share-records/1/media?') && r.url().includes('page_size=17'))
 await page.getByRole('button',{name:'应用',exact:true}).click()
 await resized
 assert.equal(await page.evaluate(()=>localStorage.getItem('share:gallery:page_size')),'17')
 await page.reload()
 await page.getByText('爱影综艺包',{exact:true}).waitFor()
 await page.locator('.n-data-table-expand-trigger').click()
 await page.locator('.media-card').first().waitFor()
 await page.getByRole('button',{name:'展开同剧集记录（22）',exact:true}).click()
 await page.locator('.media-card').nth(16).waitFor()
 assert.equal(await page.locator('.media-card').count(),17)
 await page.locator('.share-gallery .n-input-number input').fill('')
 await page.getByRole('button',{name:'应用',exact:true}).click()
 await page.getByText('每页条数请输入1至100的整数',{exact:true}).waitFor()
 assert.equal(await page.evaluate(()=>localStorage.getItem('share:gallery:page_size')),'17')
 await page.evaluate(()=>localStorage.setItem('share:gallery:page_size','invalid'))
 savedSize=10
 await page.reload()
 await page.getByText('爱影综艺包',{exact:true}).waitFor()
 await page.locator('.n-data-table-expand-trigger').click()
 await page.locator('.media-card').first().waitFor()
 assert.equal(await page.locator('.share-gallery .n-input-number input').inputValue(),'10')
 console.log('PASS: 修改每页条数并刷新恢复； 同剧集一张海报、展开原始单集、逐条操作、重新合并；API mocked')
} finally {await browser.close()}
