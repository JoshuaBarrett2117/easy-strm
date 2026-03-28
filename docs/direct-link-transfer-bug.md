# 直链API转存逻辑问题

## 问题描述

当115账号配置了转存账号后，直链API获取直链失败。

## 错误信息

```
115 pmt user 3-2
```

## 问题根因分析

### 当前流程（有问题）

1. 用户请求 `/direct-link?path=...&cloud115_id=3`
2. 获取文件 `pickcode`（源账号3的pickcode）
3. 检测到 `transfer_account_id=2`，转存到账号2
4. 调用 `RapidTransferFile` 秒传文件到账号2的 `/自动转存` 目录
5. **使用原来的 `pickcode` 调用 `GetFileDirectLink` 获取直链** ❌

### 问题

`RapidTransferFile` 秒传后，文件在目标账号中会有**新的pickcode**，但当前代码仍然使用源文件的pickcode去获取直链。

## 修复方案

1. `RapidTransferFile` 函数需要返回秒传后文件在目标账号中的新 pickcode
2. 直链API需要使用这个新的 pickcode 去调用 `GetFileDirectLink`

## 修复记录

### 2026-03-27 修复完成

**修改文件：**

1. **115client.go** - `RapidTransferFile` 函数
   - 函数签名修改：返回类型从 `error` 改为 `(newPickCode string, err error)`
   - 秒传成功时返回目标账号中新文件的 `PickCode`（从 `driver.UploadInitResp` 获取）
   - 所有错误返回语句同步修改为返回空字符串+错误

2. **auth.go** - 直链API代码
   - 接收 `RapidTransferFile` 返回的 `newPickCode`
   - 使用新的 `targetPickCode` 变量优先使用秒传后的pickcode
   - 添加fallback逻辑：当 `newPickCode` 为空时使用源文件pickcode

**关键代码变更：**

```go
// 115client.go - 秒传成功时
if result.Status == 2 {
    Info("Rapid transfer successful with AppID=%s: file %s, new pickcode: %s", cfg.AppID, fileName, result.PickCode)
    return result.PickCode, nil
}

// auth.go - 直链获取时
targetPickCode := newPickCode
if targetPickCode == "" {
    targetPickCode = pickcode
    Warn("Using source pickcode %s as fallback (rapid transfer may have skipped)", targetPickCode)
}
directLink, err := client.GetFileDirectLink(0, targetPickCode, targetCloud115.ID, targetCloud115.Cookie, clientUA)
```

**编译状态：** ✅ 通过

## 相关文件

- `auth.go` - 直链API实现 (L369-395)
- `115client.go` - RapidTransferFile函数实现 (L290-484)