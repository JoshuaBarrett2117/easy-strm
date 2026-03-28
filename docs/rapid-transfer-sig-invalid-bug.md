# 秒传签名 sig invalid 问题

## 问题描述

秒传API返回 `sig invalid` 错误。

## 错误信息

```
Source account ID: 3, Target account ID: 2
File SHA1: 6698ff8331ae3282dc2cbcc49185114f82b28977
Raw response: {"request":"upload","status":4,"statuscode":400,"statusmsg":"sig invalid","pickcode":"","target":"U_1_3220107906756260133","version":"4.0"}
```

## 当前状态

- **秒传**: ❌ sig invalid 失败
- **直链获取**: ✅ 通过回退逻辑成功

## 问题根因分析

### 1. 秒传签名算法可能不正确

当前代码使用简单的SHA1哈希生成sig，但115可能需要RSA/ECDSA签名。

### 2. 直链获取成功的回退逻辑

代码在秒传失败后会回退到：
```go
directLink, err := client.GetFileDirectLink(0, targetPickCode, targetCloud115.ID, targetCloud115.Cookie, clientUA)
```

这说明：
- 115的pickcode是账号通用的
- 目标账号可以直接使用源文件的pickcode获取直链
- 秒传虽然失败，但不影响直链获取

## 修复方案

### 方案1: 调查115秒传的正确签名算法

需要确认115秒传API使用的签名方式：
1. 是否需要RSA/ECDSA签名
2. ECDH加密token是否正确生成
3. appid/appversion组合是否正确

### 方案2: 优化回退逻辑

由于直链获取已经成功，可以考虑：
1. 暂时跳过秒传失败的错误
2. 确保回退逻辑能正确处理所有场景

## 下一步行动

1. 抓包分析115官方客户端的秒传请求
2. 对比sig算法实现
3. 确认appid/appversion是否匹配

## 相关文件

- `115client.go` - RapidTransferFile函数 (L291-L493)
- `auth.go` - 直链API回退逻辑 (L385-395)
