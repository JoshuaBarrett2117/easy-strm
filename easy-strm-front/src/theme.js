/**
 * 全局主题定义 — 深浅双主题
 * 深色:影视后台风(深蓝黑底 + 青色强调)
 * 浅色:明亮管理台(暖白底 + 深青强调)
 */

export const brand = {
  primary: '#22d3ee',
  primaryHover: '#38e0f8',
  primaryPressed: '#0eb8d4',
  primaryLight: '#0e7490',
  primaryLightHover: '#0891b2',
  primaryLightPressed: '#155e75'
}

export const themeOverridesDark = {
  common: {
    primaryColor: brand.primary,
    primaryColorHover: brand.primaryHover,
    primaryColorPressed: brand.primaryPressed,
    primaryColorSuppl: brand.primaryHover,
    bodyColor: '#0b0f1a',
    cardColor: '#111827',
    modalColor: '#141c2e',
    popoverColor: '#1a2338',
    borderRadius: '10px',
    fontFamily: '"Inter", "Avenir Next", "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif'
  },
  Card: {
    borderRadius: '14px'
  },
  Dialog: {
    borderRadius: '14px'
  },
  DataTable: {
    borderRadius: '12px'
  }
}

export const themeOverridesLight = {
  common: {
    primaryColor: brand.primaryLight,
    primaryColorHover: brand.primaryLightHover,
    primaryColorPressed: brand.primaryLightPressed,
    primaryColorSuppl: brand.primaryLightHover,
    bodyColor: '#f4f6fa',
    borderRadius: '10px',
    fontFamily: '"Inter", "Avenir Next", "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif'
  },
  Card: {
    borderRadius: '14px'
  },
  Dialog: {
    borderRadius: '14px'
  },
  DataTable: {
    borderRadius: '12px'
  }
}
