// 更新日期：2026-09-08；执行者：Codex。使用接口夹具验证AI配置、模型发现和未保存配置测试。
import assert from 'node:assert/strict'
import { chromium } from 'playwright'
const browser=await chromium.launch({headless:true})
try{
 const page=await browser.newPage({viewport:{width:1280,height:960}})
 let config={enabled:false,base_url:'http://mock.local/v1',api_key:'',has_api_key:true,model:'chat-a',timeout_seconds:30,scenes:['no_match'],prompt:'测试提示词'}
 let saved,modelDraft,testDraft,failTest=false
 const errors=[]
 page.on('pageerror',e=>errors.push(e.message))
 await page.addInitScript(()=>localStorage.setItem('token','test'))
 await page.route('**/api/**',async route=>{
  const request=route.request(),path=new URL(request.url()).pathname
  if(!path.startsWith('/api/'))return route.continue()
  let data={data:[],total:0}
  if(path==='/api/settings/ai-recognition'){
   if(request.method()==='PUT'){saved=request.postDataJSON();config={...saved,api_key:'',has_api_key:true}}
   data=config
  }
  if(path.endsWith('/ai-recognition/models')){modelDraft=request.postDataJSON();data=['chat-a','chat-b']}
  if(path.endsWith('/ai-recognition/test')){
   testDraft=request.postDataJSON()
   if(failTest)return route.fulfill({status:502,json:{error:'AI端点返回HTTP 401'}})
   data={title:'七龙珠',original_title:'Dragon Ball',year:1986,media_type:'tv'}
  }
  await route.fulfill({json:{data}})
 })
 await page.goto('http://127.0.0.1:3001/dashboard/ai-recognition')
 await page.getByPlaceholder('已保存，留空保留（更换地址后请重新填写）').waitFor()
 const body=page.locator('.ai-settings')
 await body.getByRole('button',{name:'获取模型列表',exact:true}).click()
 await page.getByText('获取到 2 个模型',{exact:true}).waitFor()
 assert.equal(modelDraft.base_url,'http://mock.local/v1')
 await body.locator('.n-select').click()
 await page.locator('.n-base-select-option').filter({hasText:'chat-b'}).click()
 await body.getByRole('switch').click()
 await body.getByRole('checkbox',{name:'类型不确定：本地规则无法确定电影或剧集',exact:true}).click()
 await body.locator('textarea').first().fill('请解析真实标题，保留年份')
 await body.getByRole('button',{name:'测试 AI 识别',exact:true}).click()
 await body.getByText('Dragon Ball',{exact:true}).waitFor()
 assert.equal(testDraft.config.model,'chat-b')
 assert.equal(testDraft.config.prompt,'请解析真实标题，保留年份')
 assert.equal(saved,undefined)
 await body.getByRole('button',{name:'保存配置',exact:true}).click()
 await page.getByText('AI配置已保存',{exact:true}).waitFor()
 assert.equal(saved.enabled,true)
 assert.deepEqual(saved.scenes.sort(),['no_match','uncertain_type'])
 await page.reload()
 await body.locator('textarea').first().waitFor()
 assert.equal(await body.locator('textarea').first().inputValue(),'请解析真实标题，保留年份')
 failTest=true
 await body.getByRole('button',{name:'测试 AI 识别',exact:true}).click()
 await body.getByText('AI端点返回HTTP 401',{exact:true}).waitFor()
 await page.setViewportSize({width:390,height:844})
 const bounds=await body.boundingBox()
 assert.ok(bounds.width<=390)
 assert.deepEqual(errors,[])
 console.log('PASS: 配置加载、模型获取选择、场景开关、提示词、未保存测试、保存重载、失败提示、窄屏；API mocked')
}finally{await browser.close()}

