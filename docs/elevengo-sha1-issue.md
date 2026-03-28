# 115秒传功能SHA1获取问题分析

## 问题描述

115秒传功能需要文件的真实SHA1值，但目前无法通过API获取到文件的真实SHA1。

## 已知事实

### 1. 秒传需要的参数

115秒传API需要以下参数：
- `file_name`: 文件名
- `file_size`: 文件大小
- `file_sha1`: **文件的真实SHA1**（关键）
- `sign_key`: SHA1值
- `sign_value`: 通过特定算法计算的签名
- `target_dir_id`: 目标目录ID

### 2. SHA1获取方式尝试

| 方式 | 库/方法 | 结果 | 问题 |
|------|---------|------|------|
| 从下载URL提取 | `DownloadWithUAByAndroidAPI` | ❌ | URL中的SHA1是CDN用的，不是文件真实SHA1 |
| GetFile API | `115driver.GetFile` | ❌ | 需要文件ID，不是pickcode |
| FileSearch | `elevengo.FileSearch` | ❌ | 按文件名搜索，不是pickcode |
| ImportCalculateSignValue | `elevengo.ImportCalculateSignValue` | ❌ | 只返回sign_value，不返回SHA1 |

### 3. 错误响应分析

```
{"state":false,"code":990002,"message":"参数错误。"}
```

这个错误通常表示：
1. SHA1验证失败
2. sign_value计算不正确
3. 参数格式问题

### 4. 403错误 "115 pmt user"

这个错误表示账号可能没有秒传权限，需要VIP账号才能使用秒传功能。

## 技术分析

### 115driver的GetFile方法

```go
fileInfo, err := d.GetFile(pickCode)
```

需要的是**文件ID**（如 `5123456789012345678`），不是 **pickcode**（如 `e2268q5pt3al9zss4`）。

两者的区别：
- `file_id`: 115内部的数字ID
- `pickcode`: 115分享用的提取码

### elevengo的FileSearch方法

```go
it, err := agent.FileSearch("0", pickCode)
```

这个方法是**按文件名搜索**，不是按pickcode搜索。第二个参数是搜索关键词。

### 从URL提取SHA1的问题

下载URL格式：
```
https://xxx.115cdn.net/ABC123DEF456.../filename.mkv?k=xxx
```

URL中的SHA1 (`ABC123DEF456...`) 是CDN下载用的临时签名，不是文件真实SHA1。

## 可能的解决方案

### 方案1：先获取文件ID，再获取SHA1

1. 通过某种方式获取文件的真实file_id
2. 使用115driver的GetFile获取文件信息（包括SHA1）

**问题**：目前没有找到通过pickcode获取file_id的API

### 方案2：使用115官方App API

115官方App可能使用了不同的API，可以通过抓包获取：
- 使用Fiddler或Charles抓包115客户端
- 分析秒传请求的具体参数
- 找到正确的SHA1获取方式

### 方案3：使用115网页版API

115网页版可能有不同的API端点，可以通过：
- 浏览器开发者工具分析网络请求
- 找到获取文件SHA1的正确API

### 方案4：联系115开发者支持

如果以上方案都无法解决，可能需要：
- 联系115官方技术支持
- 咨询相关开发者社区

## 结论

目前`elevengo`秒传方式无法正常工作，因为**无法获取文件的真实SHA1**。

建议：
1. 继续使用默认的`115driver`方式（如果115driver的SHA1获取正确）
2. 或者使用`alist`方式（如果配置正确）
3. 如果都需要VIP权限，考虑使用115官方客户端进行秒传

## 测试日志

```
[DEBUG] 2026-03-28 13:53:15 Attempting rapid transfer file e2268q5pt3al9zss4 to target account 2 using method elevengo
```

秒传函数被调用，但没有后续日志，可能在`GetFileDirectLink`处超时或出错。

## 相关文件

- `115client.go`: 秒传实现
- `auth.go`: 直链API处理

## 更新日志

- 2026-03-28: 创建文档
