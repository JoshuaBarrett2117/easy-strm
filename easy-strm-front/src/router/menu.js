// 侧栏菜单：五个分组，每项独立图标（不得重复，见 tests/vitest/menu.spec.js）。
// 只改分组与展示，路由路径保持不变以免书签失效。
import {
  AlarmOutline,
  CheckmarkDoneOutline,
  CloudDownloadOutline,
  CloudOutline,
  DocumentTextOutline,
  FlaskOutline,
  FolderOpenOutline,
  GitBranchOutline,
  GitCompareOutline,
  HomeOutline,
  LayersOutline,
  LibraryOutline,
  ListOutline,
  PulseOutline,
  ReaderOutline,
  SettingsOutline,
  ShareSocialOutline,
  SparklesOutline,
  SwapHorizontalOutline,
  TvOutline,
  WifiOutline
} from '@vicons/ionicons5'

export const menuSections = [
  {
    title: '总览',
    items: [
      { path: '/dashboard/home', label: '仪表盘', icon: HomeOutline },
      { path: '/dashboard/tasks', label: '任务中心', icon: ListOutline },
      { path: '/dashboard/scheduled-tasks', label: '定时任务管理', icon: AlarmOutline }
    ]
  },
  {
    title: '媒体整理',
    items: [
      { path: '/dashboard/media-manager', label: '文件工作台', icon: FolderOpenOutline },
      { path: '/dashboard/file-manager', label: '文件管理', icon: SwapHorizontalOutline },
      { path: '/dashboard/category-strategy', label: '整理规则', icon: GitBranchOutline },
      { path: '/dashboard/filename-recognition', label: '识别测试', icon: FlaskOutline },
      { path: '/dashboard/ai-recognition', label: 'AI 辅助识别', icon: SparklesOutline }
    ]
  },
  {
    title: '115 与分享',
    items: [
      { path: '/dashboard/cloud115', label: '115 云管理', icon: CloudOutline },
      { path: '/dashboard/resources/transfer', label: '资源聚合', icon: CloudDownloadOutline },
      { path: '/dashboard/share-records', label: '分享管理', icon: ShareSocialOutline },
      { path: '/dashboard/share-review', label: '手动核对', icon: CheckmarkDoneOutline },
      { path: '/dashboard/share-library', label: '分享资源库', icon: LibraryOutline },
      { path: '/dashboard/share-selections', label: 'STRM 来源选择', icon: GitCompareOutline }
    ]
  },
  {
    title: 'STRM 与 Emby',
    items: [
      { path: '/dashboard/strm-config', label: 'STRM 配置', icon: DocumentTextOutline },
      { path: '/dashboard/emby-management', label: 'Emby 管理', icon: TvOutline, adminOnly: true },
      { path: '/dashboard/emby-monitor', label: 'Emby 监控', icon: PulseOutline, adminOnly: true }
    ]
  },
  {
    title: '系统',
    items: [
      { path: '/dashboard/settings', label: '系统设置', icon: SettingsOutline },
      { path: '/dashboard/system-logs', label: '系统日志', icon: ReaderOutline },
      { path: '/dashboard/network', label: '网络测试', icon: WifiOutline },
      { path: '/dashboard/cache', label: '缓存管理', icon: LayersOutline }
    ]
  }
]

/** 按权限过滤菜单（非管理员隐藏 adminOnly 项），空分组不显示。 */
export const visibleMenuSections = (isAdmin) => menuSections
  .map(section => ({ ...section, items: section.items.filter(item => !item.adminOnly || isAdmin) }))
  .filter(section => section.items.length > 0)

/** 当前路由所在分组标题。 */
export const sectionTitleFor = (path) => menuSections.find(section => section.items.some(item => item.path === path))?.title || ''

export const SIDEBAR_COLLAPSED_KEY = 'easy-strm:sidebar-collapsed'
