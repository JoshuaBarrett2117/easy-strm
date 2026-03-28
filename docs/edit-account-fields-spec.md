# 编辑115云账号字段修复需求文档

> **文档版本**：v1.0\
> **创建日期**：2026-03-27\
> **需求来源**：编辑115云账号时，账号类型、状态、优先级无法保存生效\
> **状态**：待排查

***

## 一、问题描述

### 1.1 问题现象

在编辑 115 云账号时，以下字段无法保存生效：

- **账号类型**（account\_type）
- **状态**（status）
- **优先级**（priority）

### 1.2 复现步骤

1. 进入 115 云账号管理页面
2. 点击任意账号的「编辑」按钮
3. 修改账号类型、状态、优先级
4. 点击「确定」保存
5. 观察列表中对应字段是否已更新

***

## 二、问题分析

### 2.1 代码链路追踪

#### 前端链路

| 步骤 | 文件           | 函数/位置                    | 说明               |
| :- | :----------- | :----------------------- | :--------------- |
| 1  | Cloud115.vue | handleEdit(row)          | 点击编辑按钮           |
| 2  | Cloud115.vue | orm.value = {...}       | 表单数据绑定（134-152行） |
| 3  | Cloud115.vue | handleSubmit()           | 提交表单（543-575行）   |
| 4  | cloud115.js  | updateCloud115(id, data) | 调用 PUT API       |

#### 后端链路

| 步骤 | 文件      | 函数/位置                        | 说明                                 |
| :- | :------ | :--------------------------- | :--------------------------------- |
| 1  | auth.go | PUT /cloud115/:id (912-977行) | 接收请求                               |
| 2  | auth.go | 绑定 cloud115Data 结构体          | 包含 account\_type, priority, status |
| 3  | auth.go | 调用 UpdateCloud115()          | 传入所有参数                             |
| 4  | db.go   | UpdateCloud115() (697-714行)  | 执行 SQL UPDATE                      |

### 2.2 可疑点排查

#### 排查点 1：后端 API 接收

**文件**：uth.go 第 917-928 行

`go
var cloud115Data struct {
    Name              string json:"name" binding:"required"
    Cookie            string json:"cookie"
    RefreshToken      string json:"refresh_token"
    AccessToken       string json:"access_token"
    ExpiresIn         int    json:"expires_in"
    TransferAccountID int    json:"transfer_account_id"
    TransferDirectory string json:"transfer_directory"
    AccountType       string json:"account_type"   // ✅ 存在
    Priority          int    json:"priority"       // ✅ 存在
    Status           string json:"status"         // ✅ 存在
}
`

**结论**：后端结构体字段存在，无异常。

#### 排查点 2：默认值处理

**文件**：uth.go 第 936-944 行

`go
if cloud115Data.AccountType == "" {
    cloud115Data.AccountType = "resource"
}
if cloud115Data.Priority == 0 {
    cloud115Data.Priority = 5
}
if cloud115Data.Status == "" {
    cloud115Data.Status = "active"
}
`

**结论**：默认值逻辑正常，但如果有值传来不会被覆盖。

#### 排查点 3：UpdateCloud115 函数

**文件**：db.go 第 697-714 行

\`go
func UpdateCloud115(id int, name, cookie, refreshToken, accessToken string,
expiresIn, transferAccountID int, transferDirectory string,
accountType string, priority int, status string) (\*Cloud115, error) {

```
_, err := db.QueryRow(
    UPDATE t_cloud_115 SET 
        name = , cookie = , refresh_token = , access_token = , 
        expires_in = , transfer_account_id = , transfer_directory = , 
        account_type = , priority = , status =  
     WHERE id = 
     RETURNING ...,
    name, cookie, refreshToken, accessToken, expiresIn, 
    transferAccountID, transferDirectory, accountType, priority, status, id,
)
```

}
\`

**结论**：SQL UPDATE 语句包含所有字段，参数传递顺序正确。

#### 排查点 4：前端表单提交

**文件**：Cloud115.vue 第 556-565 行

\`javascript
const submitData = {
...form.value,
transfer\_account\_id: form.value.transfer\_account\_id || 0
}

request(apiUrl, {
method,
data: submitData
})
\`

**form.value 结构**：

`javascript
const form = ref({
    id: null,
    name: '',
    cookie: '',
    access_token: '',
    refresh_token: '',
    transfer_account_id: null,
    transfer_directory: '',
    account_type: 'resource',  // ✅ 存在
    status: 'active',           // ✅ 存在
    priority: 5                 // ✅ 存在
})
`

**结论**：表单数据结构正确，展开运算符会包含所有字段。

### 2.3 初步结论

代码层面看不出明显问题，需要进一步排查：

1. **网络请求是否正确发送？** - 需要检查前端 Network 面板
2. **后端是否正确接收？** - 需要在后端添加 Debug 日志
3. **数据库是否正确更新？** - 需要检查数据库实际值

***

## 三、进一步排查计划

### 3.1 前端 Debug

在 handleSubmit 函数中添加日志：

`javascript
const handleSubmit = async () => {
    console.log('提交数据:', JSON.stringify(submitData, null, 2))
    // ...
}
`

### 3.2 后端 Debug

在 uth.go PUT 处理器中添加日志：

`go
Debug("Update cloud115 data:%+v", cloud115Data)
`

### 3.3 数据库验证

\`sql
\-- 更新前
SELECT id, name, account\_type, status, priority FROM t\_cloud\_115 WHERE id = ?;

\-- 更新后
SELECT id, name, account\_type, status, priority FROM t\_cloud\_115 WHERE id = ?;
\`

***

## 四、修复方案（待确认根因后补充）

### 方案 A：若后端未接收到正确数据

**可能原因**：前端请求体格式问题

**修复**：

1. 检查
   equest 工具是否正确序列化数据
2. 确保 Content-Type: application/json 头正确设置

### 方案 B：若后端接收正确但未更新到数据库

**可能原因**：SQL 执行问题

**修复**：

1. 添加 SQL 执行日志
2. 检查是否有错误被吞掉

### 方案 C：若数据库更新成功但返回数据有问题

**可能原因**：RETURNING 子句读取的数据不正确

**修复**：

1. 分离 UPDATE 和 SELECT 操作
2. 先 UPDATE，再单独查询验证

***

## 五、附录：相关文件索引

| 文件        | 路径                                        | 说明                |
| :-------- | :---------------------------------------- | :---------------- |
| 前端账号管理页   | easy-strm-front/src/views/Cloud115.vue    | 账号列表和表单           |
| 前端 API 工具 | easy-strm-front/src/utils/api/cloud115.js | API 调用封装          |
| 后端 API 路由 | easy-strm/auth.go                         | PUT /cloud115/:id |
| 数据库操作     | easy-strm/db.go                           | UpdateCloud115 函数 |
| 领域模型      | easy-strm/db.go                           | Cloud115 结构体定义    |

***

## 六、需求答疑区

**\[需求答疑区]**

待确认问题：

1. **account\_type 字段的可选值有哪些？** 当前代码中有 resource/vip/both，是否完整？   回答：完整
2. **status 字段的 cooling 状态如何触发？** 是手动设置还是系统自动判断？                         回答：系统可以自动判断，也可以手动设置
3. **priority 字段的数值范围？** 1-10 还是其他？                                      回答：1-10

