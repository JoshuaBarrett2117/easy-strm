# 新功能开发需求文档

## 需求日期：2026-03-28

***

## 需求1：系统配置页面

### 背景

当前alist秒传方式需要配置`alist_url`和`alist_token`，但没有统一的配置管理页面。需要创建一个系统配置页面来管理这些配置。

### 技术方案

#### 1. 数据库设计

**表名**：`t_setting`

```sql
CREATE TABLE t_setting (
    id SERIAL PRIMARY KEY,
    setting_key VARCHAR(100) UNIQUE NOT NULL,
    setting_value TEXT,
    description VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 初始配置项
INSERT INTO t_setting (setting_key, setting_value, description) VALUES
('alist_url', '', 'Alist服务器地址'),
('alist_token', '', 'Alist访问令牌');
```

#### 2. 后端实现

**Model**：

```go
type Setting struct {
    ID           int       `json:"id"`
    SettingKey   string    `json:"setting_key"`
    SettingValue string    `json:"setting_value"`
    Description  string    `json:"description"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}
```

**API接口**：

- `GET /settings` - 获取所有配置
- `GET /settings/:key` - 获取单个配置
- `PUT /settings/:key` - 更新配置
- `GET /settings/alist` - 获取alist配置（简化接口）

#### 3. 前端实现

**页面路径**：`/dashboard/settings`

**页面布局**：

- 使用表单形式展示配置项（不是表格）
- 每个配置项包含：label、input/textarea、description
- 提交按钮

**配置项**：

1. Alist服务器地址 (alist\_url) - 输入框
2. Alist访问令牌 (alist\_token) - 密码输入框

**Alist配置帮助信息**：

当秒传方式选择`alist`时，需要在系统配置页面配置以下两项：

1. **Alist服务器地址** (alist\_url)
   - 格式：`http://your-alist-server:port`
   - 示例：`http://192.168.1.100:5244` 或 `https://alist.example.com`
2. **Alist访问令牌** (alist\_token)
   - 获取方式：登录Alist网页端 → 设置 → 左侧菜单"其他" → 查看"令牌"
   - 示例：`eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...`

**配置示例**：

```
alist_url = http://192.168.1.100:5244
alist_token = eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoiMSIsImV4cCI6MTcwOTkzMzMzNn0.xxxxxxxxxxx
```

**注意事项**：

- 确保Alist服务器可以正常访问
- 确保Alist的"直链强度"设置为"弱"（允许获取直链）

***

## 需求2：115云管理页面修复

### 问题描述

1. 秒传方式无法更新保存
2. 未配置转存账号时，秒传方式和转存目录应该为空/禁用

### 修复要求

#### 1. 秒传方式保存修复

**问题分析**：

- 前端表单可能没有正确绑定transferMethod字段
- 或者后端API没有正确处理该字段

**修复点**：

1. 检查Cloud115.vue中表单数据是否包含transferMethod
2. 检查UpdateRequest结构体是否包含transferMethod字段
3. 验证API调用是否正确传递该字段

#### 2. 联动逻辑

**需求**：

- 当"转存账号"选择为空（未配置）时：
  - "秒传方式"应该禁用或隐藏
  - "转存目录"应该禁用或隐藏
- 当"转存账号"有值时：
  - "秒传方式"启用
  - "转存目录"启用

**前端逻辑**：

```javascript
// Watch 转存账号变化
watch: {
    'form.transfer_account_id': function(newVal) {
        if (!newVal || newVal === 0) {
            this.form.transfer_method = '115driver'; // 重置默认值
            this.form.transfer_directory = ''; // 清空
            this.transferDisabled = true; // 禁用
        } else {
            this.transferDisabled = false; // 启用
        }
    }
}
```

***

## 需求3：秒传方式验证

### 验证目标

验证三种秒传方式是否能成功秒传到转存账号的转存目录下：

1. 115driver（默认）
2. go115
3. alist

### 验证方法

#### 1. 日志增强

在秒传调用时打印详细日志：

```go
Info("=== Rapid Transfer Start ===")
Info("Transfer method: %s", method)
Info("Source account ID: %d, Target account ID: %d", sourceID, targetID)
Info("Source pickcode: %s", sourcePickCode)
Info("Target directory: %s", targetDir)
Info("File SHA1: %s", sha1)
Info("File size: %d", fileSize)

// 秒传API调用...

Info("=== Rapid Transfer Result ===")
Info("Method: %s, Status: %d, PickCode: %s", method, status, newPickCode)
```

#### 2. 验证步骤

1. 配置账号3的秒传方式为115driver，调用直链API，检查日志
2. 配置账号3的秒传方式为go115，调用直链API，检查日志
3. 配置账号3的秒传方式为alist（需要先配置alist\_url和alist\_token），调用直链API，检查日志 。

#### 3. 成功标准

- 秒传API返回status=2（成功）
- 日志显示`new pickcode`不为空
- 文件存在于目标账号的转存目录下

***

## 实施优先级

1. **P0**：需求2（秒传方式保存修复）- 阻塞其他功能
2. **P1**：需求1（系统配置页面）- 支撑alist配置
3. **P2**：需求3（秒传验证）- 功能验证

***

## 数据库迁移

需要执行的SQL：

```sql
CREATE TABLE IF NOT EXISTS t_setting (
    id SERIAL PRIMARY KEY,
    setting_key VARCHAR(100) UNIQUE NOT NULL,
    setting_value TEXT,
    description VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE t_cloud_115 ADD COLUMN IF NOT EXISTS transfer_method VARCHAR(50) DEFAULT '115driver';
ALTER TABLE t_cloud_115 ADD COLUMN IF NOT EXISTS alist_url VARCHAR(500);
ALTER TABLE t_cloud_115 ADD COLUMN IF NOT EXISTS alist_token VARCHAR(255);
```

***

## QA审查区

### 需求1：系统配置页面

| 检查项         | 状态         | QA备注   |
| ----------- | ---------- | ------ |
| 数据库表设计正确    | √ 通过 ☐ 需修改 | <br /> |
| Model字段命名正确 | √ 通过 ☐ 需修改 | <br /> |
| API接口设计合理   | √ 通过 ☐ 需修改 | <br /> |
| 前端表单布局合理    | √ 通过 ☐ 需修改 | <br /> |
| Alist帮助信息完整 | √ 通过 ☐ 需修改 | <br /> |

**其他问题**：

***

### 需求2：115云管理页面修复

| 检查项          | 状态         | QA备注   |
| ------------ | ---------- | ------ |
| 秒传方式保存修复方案合理 | √ 通过 ☐ 需修改 | <br /> |
| 联动逻辑设计正确     | √ 通过 ☐ 需修改 | <br /> |

**其他问题**：

***

### 需求3：秒传方式验证

| 检查项      | 状态         | QA备注   |
| -------- | ---------- | ------ |
| 日志增强方案完整 | √ 通过 ☐ 需修改 | <br /> |
| 验证步骤清晰可行 | √ 通过 ☐ 需修改 | <br /> |

**其他问题**：

***

### 整体评估

| 项目     | 结果         |
| ------ | ---------- |
| 需求完整性  | √ 完整 ☐ 需补充 |
| 技术方案可行 | √ 可行 ☐ 需调整 |
| 优先级合理  | √ 合理 ☐ 需调整 |

**综合意见**：

暂无意见可以实施

<br />

#
