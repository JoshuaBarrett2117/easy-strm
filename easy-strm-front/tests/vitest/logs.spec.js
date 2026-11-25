import { describe, expect, it } from 'vitest'
import { formatFileSize, parseLogLine, shortLogId, sortLogFiles } from '../../src/utils/logs'

describe('parseLogLine', () => {
  it('解析统一单行格式', () => {
    const line = parseLogLine('[Wed 2026-11-25 11:54 PM CST][WARN][req=abc][task=42][module=healthz][trace=-] 依赖不可用 dependency="redis"')
    expect(line).toMatchObject({ parsed: true, level: 'WARN', req: 'abc', task: '42', module: 'healthz', message: '依赖不可用 dependency="redis"' })
  })

  it('解析 JSON 格式并带上字段与错误链', () => {
    const line = parseLogLine(JSON.stringify({ timestamp: 't', level: 'ERROR', module: 'db', message: '失败', fields: { version: 2 }, error_chain: ['boom'] }))
    expect(line).toMatchObject({ parsed: true, level: 'ERROR', req: '-', task: '-', module: 'db' })
    expect(line.message).toBe('失败 {"version":2} error_chain=["boom"]')
  })

  it('无法解析的行原样返回', () => {
    expect(parseLogLine('{not json')).toEqual({ parsed: false, raw: '{not json' })
    expect(parseLogLine('plain text')).toEqual({ parsed: false, raw: 'plain text' })
  })
})

describe('sortLogFiles', () => {
  it('按日期倒序，同日 debug 在 info 前，不修改入参', () => {
    const files = [
      { name: 'info_2026-11-24.log', type: 'info' },
      { name: 'info_2026-11-25.log', type: 'info' },
      { name: 'debug_2026-11-25.log', type: 'debug' },
      { name: 'debug_2026-11-24.log', type: 'debug' }
    ]
    const sorted = sortLogFiles(files)
    expect(sorted.map(file => file.name)).toEqual(['debug_2026-11-25.log', 'info_2026-11-25.log', 'debug_2026-11-24.log', 'info_2026-11-24.log'])
    expect(files[0].name).toBe('info_2026-11-24.log')
  })
})

describe('formatFileSize / shortLogId', () => {
  it('格式化文件大小', () => {
    expect(formatFileSize(0)).toBe('0 B')
    expect(formatFileSize(512)).toBe('512 B')
    expect(formatFileSize(1536)).toBe('1.50 KB')
    expect(formatFileSize(5 * 1024 * 1024)).toBe('5.00 MB')
  })

  it('长 ID 只保留首尾', () => {
    expect(shortLogId('short-id')).toBe('short-id')
    expect(shortLogId('0123456789abcdef')).toBe('01234567…cdef')
  })
})
