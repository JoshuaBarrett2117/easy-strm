// 更新日期：2026-09-26；维护者：Codex。隔离入口验证任务中心，避免依赖其他页面或真实服务。
import assert from 'node:assert/strict'
import { createServer } from 'vite'
import { chromium } from 'playwright'
const fixture = {task_id:'partial',task_type:'share_identify',task_name:'分享媒体批量识别',status:'failed',display_status:'partial_success',progress:100,total_files:115,processed_files:115,success_files:62,failed_files:53,error_message:'识别完成，但有 53 项失败'}
const server=await createServer({server:{port:0},plugins:[{name:'task-display-fixture',configureServer(server){server.middlewares.use((req,res,next)=>{if(req.url!=='/__task-test')return next();res.setHeader('Content-Type','text/html');res.end('<div id="app"></div><script type="module" src="/__task-test.js"></script>')})},resolveId(id){if(id==='/__task-test.js')return id},load(id){if(id==='/__task-test.js')return `import '/src/style.css';import {createApp,h} from 'vue';import {createRouter,createMemoryHistory,RouterView} from 'vue-router';import {NMessageProvider} from 'naive-ui';import TaskCenter from '/src/views/dashboard/TaskCenter.vue';const router=createRouter({history:createMemoryHistory(),routes:[{path:'/',component:TaskCenter}]});createApp({render:()=>h(NMessageProvider,null,{default:()=>h(RouterView)})}).use(router).mount('#app');`}}]})
await server.listen()
const browser=await chromium.launch({headless:true})
try{
 const page=await browser.newPage({viewport:{width:1440,height:1100}})
 const errors=[];page.on('pageerror',e=>{errors.push(e.message);console.error(e.message)})
 page.on('console',m=>{if(m.type()==='error')console.error(m.text())})
 await page.addInitScript(()=>localStorage.setItem('token','test'))
 await page.route('**/api/**',route=>new URL(route.request().url()).pathname.startsWith('/api/') ? route.fulfill({json:{data:new URL(route.request().url()).pathname.endsWith('/unified')?{data:[fixture],total:1}:fixture}}) : route.continue())
 await page.goto(`http://localhost:${server.httpServer.address().port}/__task-test`)
 const card=page.getByTestId('task-card');await card.waitFor()
 assert.match(await card.innerText(),/部分成功/)
 assert.match(await card.locator('.n-alert').getAttribute('style'), /--n-icon-color: #f0a020/)
 assert.doesNotMatch(await card.locator('.n-alert').getAttribute('style'), /--n-icon-color: #d03050/)
 assert.equal(await card.locator('.n-progress--warning').count(),1)
 await page.screenshot({path:'../debug/task-display-card.png',fullPage:true})
 await card.getByRole('button',{name:'详情',exact:true}).click()
 await page.locator('.n-drawer').getByText('部分成功',{exact:true}).waitFor()
 assert.match(await page.locator('.n-drawer').innerText(),/部分成功/)
 assert.equal(await page.locator('.n-drawer .n-progress--warning').count(),1)
 await page.locator('.n-drawer').evaluate(el => Promise.allSettled(el.getAnimations({subtree:true}).map(a=>a.finished)))
 await page.screenshot({path:'../debug/task-display-desktop.png',fullPage:true})
 await page.setViewportSize({width:390,height:844})
 assert.equal(await page.locator('.n-drawer').isVisible(),true)
 assert.deepEqual(errors,[])
 console.log('PASS: 部分成功标签、警示提示、列表与详情进度、窄屏显示；使用模拟接口，无真实通知')
}finally{await browser.close();await server.close()}

