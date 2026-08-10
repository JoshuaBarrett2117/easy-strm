import assert from 'node:assert/strict'
import test from 'node:test'

import {
  build115DirectoryNodes,
  extract115FileItems
} from './targetFolderTree.js'

test('从 Axios 包装的 115 原始响应提取目录数组', () => {
  const response = {
    data: {
      count: 2,
      data: [
        { cid: '100', fid: '', pid: '0', n: '电影', ico: 'folder' },
        { cid: '0', fid: '200', pid: '0', n: '说明.txt', ico: 'txt' }
      ]
    }
  }

  assert.equal(extract115FileItems(response).length, 2)
})

test('将 115 原始目录字段归一化为可懒加载树节点', () => {
  const nodes = build115DirectoryNodes([
    { cid: '100', fid: '', pid: '0', n: '电影', ico: 'folder' },
    { cid: '0', fid: '200', pid: '0', n: '说明.txt', ico: 'txt' },
    { id: '300', name: '剧集', is_directory: true }
  ], '/')

  assert.deepEqual(nodes.map(node => ({
    key: node.key,
    label: node.label,
    directoryId: node.directoryId,
    isLeaf: node.isLeaf
  })), [
    { key: '/电影', label: '电影', directoryId: '100', isLeaf: false },
    { key: '/剧集', label: '剧集', directoryId: '300', isLeaf: false }
  ])
  assert.equal('children' in nodes[0], false)
})

test('子目录路径不会产生重复斜杠', () => {
  const [node] = build115DirectoryNodes([
    { cid: '101', fid: '', n: '动作片', ico: 'folder' }
  ], '/电影/')

  assert.equal(node.key, '/电影/动作片')
})
