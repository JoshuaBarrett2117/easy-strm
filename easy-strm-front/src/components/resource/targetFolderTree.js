/**
 * 从 Axios 响应或后端响应体中提取 115 文件数组。
 * 兼容当前 `/115/files` 直接返回 `data` 数组以及历史包装结构。
 */
export const extract115FileItems = (response) => {
  const candidates = [response, response?.data, response?.data?.data]
  for (const candidate of candidates) {
    if (Array.isArray(candidate)) return candidate
    if (Array.isArray(candidate?.files)) return candidate.files
    if (Array.isArray(candidate?.data)) return candidate.data
  }
  return []
}

/** 判断文件项是否为目录，兼容 115 原始字段与项目归一化字段。 */
const isDirectoryItem = (file) => {
  if (file?.is_directory !== undefined) return Boolean(file.is_directory)
  if (file?.is_dir !== undefined) return Boolean(file.is_dir)
  if (file?.IsDirectory !== undefined) return Boolean(file.IsDirectory)

  const directoryId = file?.cid ?? file?.category_id
  const fileId = file?.fid ?? file?.file_id ?? file?.FileID
  return file?.ico === 'folder' || (directoryId !== undefined && directoryId !== '' && !fileId)
}

/** 拼接供用户选择的目录路径，并消除重复斜杠。 */
const joinDirectoryPath = (parentPath, name) => {
  const parent = parentPath && parentPath !== '/'
    ? `/${String(parentPath).replace(/^\/+|\/+$/g, '')}`
    : ''
  return `${parent}/${String(name).replace(/^\/+|\/+$/g, '')}` || '/'
}

/** 将 115 文件项转换为 Naive UI Tree 可懒加载的目录节点。 */
export const build115DirectoryNodes = (files, parentPath = '/') => {
  return (Array.isArray(files) ? files : [])
    .filter(isDirectoryItem)
    .map((file) => {
      const name = file.name || file.n || file.file_name || file.Name || '未命名目录'
      const directoryId = file.cid ?? file.category_id ?? file.file_id ?? file.id ?? file.FileID
      const path = file.path || joinDirectoryPath(parentPath, name)
      return {
        key: path,
        label: name,
        path,
        directoryId: String(directoryId ?? ''),
        isLeaf: false
      }
    })
}
