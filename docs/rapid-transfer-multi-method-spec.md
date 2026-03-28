# 秒传方式可配置化需求文档

> **文档版本**：v1.0  
> **创建日期**：2026-03-27  
> **需求来源**：115driver 秒传签名算法存在 sig invalid 问题  
> **状态**：待实现

---

## 一、需求背景

### 1.1 问题描述

当前 115 云账号的转存秒传功能使用的是 115driver，但秒传签名算法存在问题（sig invalid），导致秒传 API 调用失败。系统目前通过回退逻辑（直接使用 pickcode 获取直链）绕过了这个问题，但这不是最优解。

### 1.2 现状分析

| 项目 | 现状 |
| :--- | :--- |
| **秒传实现** | 115driver（SHA1 签名） |
| **秒传状态** | sig invalid 失败 |
| **直链获取** | 通过回退逻辑成功 |
| **备用方案** | pickcode 直链（不依赖秒传） |

### 1.3 需求目标

支持多种秒传方式，提供灵活的后备方案：
- 115driver（当前实现，签名有问题）
- go115（Go 语言实现的 115 SDK）
- alist（使用 alist 的秒传 API）

---

## 二、技术方案

### 2.1 数据库变更

**新增字段**：	ransfer_method

`sql
ALTER TABLE t_cloud_115 ADD COLUMN IF NOT EXISTS transfer_method VARCHAR(20) DEFAULT '115driver';
`

**字段说明**：

| 字段名 | 类型 | 默认值 | 可选值 | 说明 |
| :--- | :--- | :--- | :--- | :--- |
| transfer_method | VARCHAR(20) | 115driver | 115driver, go115, alist | 秒传方式 |

### 2.2 后端实现

#### 2.2.1 Domain 模型变更

**文件**：easy-strm/db.go 或 easy-strm/internal/domain/cloud115.go

`go
// Cloud115 115云账号信息
type Cloud115 struct {
    // ... 现有字段 ...
    TransferMethod string json:"transfer_method"  // 新增：秒传方式
}
`

#### 2.2.2 DAO 层变更

**文件**：easy-strm/internal/dao/cloud115_dao.go 或 easy-strm/db.go

**Create 函数**：
- 新增 	ransferMethod 参数
- SQL INSERT 语句添加 	ransfer_method 字段

**Update 函数**：
- 新增 	ransferMethod 参数
- SQL UPDATE 语句添加 	ransfer_method =  条件

**GetByID / GetAll 函数**：
- SELECT 语句添加 COALESCE(transfer_method, '115driver') 字段

#### 2.2.3 秒传服务接口抽象

**文件**：easy-strm/internal/service/transfer/（新建）

`go
// Transferrier 秒传接口
type Transferrier interface {
    // Transfer 执行秒传
    Transfer(ctx context.Context, req *TransferRequest) (*TransferResult, error)
}

// TransferRequest 秒传请求
type TransferRequest struct {
    SourcePickCode    string
    SourceAccountID   int
    SourceCookie      string
    TargetDirID       string
    TargetAccountID   int
    TargetCookie      string
    FileName          string
    SHA1              string
    FileSize          int64
}

// TransferResult 秒传结果
type TransferResult struct {
    Success    bool
    NewPickCode string  // 秒传成功后目标账号的文件 pickcode
    Message     string
    Err        error
}
`

#### 2.2.4 三种秒传实现

**1. 115driver 秒传（当前实现）**

**文件**：easy-strm/internal/service/transfer/driver.go

- 复用现有 115client.go 中的 RapidTransferFile 函数
- 封装为 Transferrier 接口实现

**2. go115 秒传**

**文件**：easy-strm/internal/service/transfer/go115.go

- 调研 go115 库的秒传 API 实现方式
- 参考地址：https://github.com/Starryahri/ga115 或类似开源库

**3. alist 秒传**

**文件**：easy-strm/internal/service/transfer/alist.go

- 调用 alist 的秒传 API
- API 格式：POST /api/fs/transfer
- 需要在账号配置中额外存储 alist 地址和 token

### 2.3 直链 API 变更

**文件**：easy-strm/auth.go - 直链获取逻辑

**变更点**：
1. 获取账号的 	ransfer_method 配置
2. 根据配置调用对应的秒传实现
3. 保留回退逻辑：秒传失败时使用 pickcode 直接获取直链

`go
// 伪代码示例
transferMethod := cloud115.TransferMethod
if transferMethod == "" {
    transferMethod = "115driver"
}

var transferrer service.Transferrier
switch transferMethod {
case "go115":
    transferrer = go115.New()
case "alist":
    transferrer = alist.New(alistConfig)
default:
    transferrer = driver115.New()
}

newPickCode, err := transferrer.Transfer(ctx, req)
if err != nil {
    // 回退：使用源 pickcode 获取直链
    targetPickCode = pickcode
}
`

### 2.4 前端实现

**文件**：easy-strm-front/src/views/Cloud115.vue

#### 2.4.1 表单新增字段

`ue
<el-form-item label="秒传方式" prop="transfer_method">
  <el-select v-model="form.transfer_method" placeholder="请选择秒传方式" style="width: 100%">
    <el-option label="115driver（默认）" value="115driver" />
    <el-option label="go115" value="go115" />
    <el-option label="alist" value="alist" />
  </el-select>
  <div class="form-tip">选择失败时自动回退到直链获取</div>
</el-form-item>
`

#### 2.4.2 表单数据结构

`javascript
const form = ref({
  // ... 现有字段 ...
  transfer_method: '115driver'  // 新增
})
`

#### 2.4.3 编辑回填

handleEdit 函数已通过展开运算符自动处理新字段，无需额外修改。

---

## 三、API 接口变更

### 3.1 创建账号 POST /auth/cloud115

**请求体新增字段**：

`json
{
  "name": "我的115账号",
  "cookie": "...",
  "transfer_method": "115driver",
  // ... 现有字段 ...
}
`

### 3.2 更新账号 PUT /auth/cloud115/:id

**请求体新增字段**：同创建接口

### 3.3 获取账号 GET /auth/cloud115/:id

**响应新增字段**：

`json
{
  "data": {
    "id": 1,
    "name": "我的115账号",
    "transfer_method": "115driver",
    // ... 现有字段 ...
  }
}
`

---

## 四、任务拆解与排期

### 阶段一：基础架构（约 0.5 天）

| 任务 | 负责 | 优先级 |
| :--- | :--- | :--- |
| 数据库字段添加 | 后端 | P0 |
| Domain 模型变更 | 后端 | P0 |
| DAO 层 Create/Update/GetByID 变更 | 后端 | P0 |
| Transferrier 接口定义 | 后端 | P0 |

### 阶段二：115driver 实现（约 0.5 天）

| 任务 | 负责 | 优先级 |
| :--- | :--- | :--- |
| 封装现有 RapidTransferFile 为 Transferrier | 后端 | P0 |
| 115driver 秒传适配器 | 后端 | P0 |

### 阶段三：go115/alist 实现（约 1 天）

| 任务 | 负责 | 优先级 |
| :--- | :--- |
| 调研 go115 秒传实现 | 后端 |
| go115 秒传适配器 | 后端 |
| 调研 alist 秒传 API | 后端 |
| alist 秒传适配器 | 后端 |
| alist 配置（地址/token）存储 | 后端+前端 |

### 阶段四：直链 API 集成（约 0.5 天）

| 任务 | 负责 | 优先级 |
| :--- | :--- | :--- |
| 直链 API 集成新秒传逻辑 | 后端 | P0 |
| 回退逻辑确保 | 后端 | P0 |

### 阶段五：前端实现（约 0.5 天）

| 任务 | 负责 | 优先级 |
| :--- | :--- | :--- |
| 表单新增秒传方式选择 | 前端 | P1 |
| 列表展示秒传方式 | 前端 | P2 |

---

## 五、风险与备选方案

### 5.1 主要风险

| 风险 | 影响 | 缓解措施 |
| :--- | :--- | :--- |
| go115/alist 秒传调研后发现不可用 | 高 | 保留回退逻辑，确保直链获取始终可用 |
| 秒传签名算法持续失败 | 中 | 先上线回退逻辑，后续继续研究签名算法 |
| 多个秒传方式维护成本 | 低 | 接口抽象清晰，扩展成本可控 |

### 5.2 备选方案

若所有秒传方式均失败，系统回退到：
1. 直接使用源文件的 pickcode 获取直链（当前已实现）
2. 不依赖秒传，保证直链获取功能可用

---

## 六、验收标准

1. **功能验收**
   - [ ] 可在账号配置中选择秒传方式（115driver/go115/alist）
   - [ ] 创建/编辑账号时秒传方式可保存
   - [ ] 直链 API 根据配置的秒传方式进行转存
   - [ ] 秒传失败时回退到 pickcode 直链获取

2. **兼容性验收**
   - [ ] 已有账号的 	ransfer_method 字段默认为 115driver
   - [ ] 不选择秒传方式时使用默认 115driver

3. **回退验收**
   - [ ] 任意秒传方式失败，不阻断直链获取功能