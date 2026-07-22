/**
 * 全局主题定义：以靛蓝为主操作色、青色为媒体状态强调色。
 * 深浅主题共享尺寸和交互语义，只调整表面、边界与文字对比度。
 */

export const brand = {
  primary: '#8b86ff',
  primaryHover: '#a7a3ff',
  primaryPressed: '#716cf0',
  primaryLight: '#5b5ce2',
  primaryLightHover: '#6d6ee8',
  primaryLightPressed: '#4848c8'
}

const shared = {
  common: {
    borderRadius: '12px',
    borderRadiusSmall: '9px',
    fontFamily: 'Inter, "Avenir Next", "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif',
    fontWeightStrong: '650',
    heightMedium: '36px',
    heightLarge: '42px'
  },
  Button: {
    borderRadiusMedium: '11px',
    borderRadiusLarge: '12px',
    fontWeight: '650',
    textColorPrimary: '#ffffff',
    textColorHoverPrimary: '#ffffff',
    textColorPressedPrimary: '#ffffff',
    textColorFocusPrimary: '#ffffff'
  },
  Card: { borderRadius: '18px' },
  Dialog: { borderRadius: '18px' },
  Modal: { borderRadius: '18px' },
  DataTable: { borderRadius: '14px', thColor: 'rgba(100, 116, 139, 0.055)' },
  Input: { borderRadius: '11px' },
  Select: { peers: { InternalSelection: { borderRadius: '11px' } } },
  Tag: { borderRadius: '999px' },
  Alert: { borderRadius: '14px' }
}

export const themeOverridesDark = {
  ...shared,
  common: {
    ...shared.common,
    primaryColor: brand.primary,
    primaryColorHover: brand.primaryHover,
    primaryColorPressed: brand.primaryPressed,
    primaryColorSuppl: brand.primaryHover,
    bodyColor: '#080b14',
    cardColor: '#101522',
    modalColor: '#131a29',
    popoverColor: '#171e2e',
    inputColor: 'rgba(23, 30, 46, 0.78)',
    tableColor: 'rgba(16, 21, 34, 0.7)',
    borderColor: 'rgba(255, 255, 255, 0.09)',
    dividerColor: 'rgba(255, 255, 255, 0.075)',
    textColorBase: '#eef2f8',
    textColor2: '#c5cede',
    textColor3: '#8d99ae'
  },
  DataTable: {
    ...shared.DataTable,
    thColor: 'rgba(255, 255, 255, 0.035)',
    tdColorHover: 'rgba(139, 134, 255, 0.07)'
  }
}

export const themeOverridesLight = {
  ...shared,
  common: {
    ...shared.common,
    primaryColor: brand.primaryLight,
    primaryColorHover: brand.primaryLightHover,
    primaryColorPressed: brand.primaryLightPressed,
    primaryColorSuppl: brand.primaryLightHover,
    bodyColor: '#f2f5fb',
    cardColor: '#ffffff',
    modalColor: '#ffffff',
    popoverColor: '#ffffff',
    inputColor: 'rgba(248, 250, 252, 0.9)',
    tableColor: 'rgba(255, 255, 255, 0.82)',
    borderColor: 'rgba(71, 85, 105, 0.14)',
    dividerColor: 'rgba(71, 85, 105, 0.12)',
    textColorBase: '#172033',
    textColor2: '#475467',
    textColor3: '#7b879b'
  }
}
