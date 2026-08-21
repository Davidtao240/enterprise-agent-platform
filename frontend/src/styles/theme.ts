export const themeConfig = {
  token: {
    colorPrimary: '#1D4ED8',
    colorBgBody: '#F5F7FA',
    colorBgContainer: '#FFFFFF',
    borderRadius: 8,
    fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
    fontSize: 14,
    controlHeight: 40,
  },
  components: {
    Layout: {
      siderBg: '#0F172A',
      headerBg: '#FFFFFF',
      headerHeight: 56,
      bodyBg: '#F5F7FA',
    },
    Menu: {
      darkItemBg: '#0F172A',
      darkSubMenuItemBg: '#0F172A',
      darkItemSelectedBg: '#1D4ED8',
      darkItemHoverBg: '#1E293B',
      darkItemColor: '#94A3B8',
      darkItemSelectedColor: '#FFFFFF',
      darkItemHoverColor: '#FFFFFF',
      itemBorderRadius: 8,
      itemHeight: 44,
      itemMarginInline: 8,
    },
    Card: {
      borderRadiusLG: 12,
      paddingLG: 20,
      boxShadowTertiary: '0 1px 2px 0 rgba(0, 0, 0, 0.03)',
    },
    Button: {
      borderRadius: 8,
      controlHeight: 36,
      fontWeight: 500,
    },
    Input: {
      borderRadius: 8,
      controlHeight: 40,
    },
    Table: {
      headerBg: '#F9FAFB',
      headerSplitColor: '#F3F4F6',
      rowHoverBg: '#F9FAFB',
      borderColor: '#E5E7EB',
    },
    Tag: {
      borderRadiusSM: 6,
    },
    Statistic: {
      titleFontSize: 13,
      contentFontSize: 24,
    },
    Tabs: {
      itemSelectedColor: '#1D4ED8',
      itemHoverColor: '#3B82F6',
      inkBarColor: '#1D4ED8',
    },
    Modal: {
      borderRadiusLG: 12,
    },
    Drawer: {
      borderRadiusLG: 12,
    },
    Badge: {
      colorError: '#EF4444',
      colorWarning: '#F59E0B',
      colorSuccess: '#10B981',
      colorProcessing: '#3B82F6',
    },
  },
};

export const categoryGradients = {
  general: 'linear-gradient(135deg, #667EEA 0%, #764BA2 100%)',
  finance: 'linear-gradient(135deg, #11998E 0%, #38EF7D 100%)',
  document: 'linear-gradient(135deg, #F093FB 0%, #F5576C 100%)',
  audit: 'linear-gradient(135deg, #FC466B 0%, #3F5EFB 100%)',
  planning: 'linear-gradient(135deg, #FA709A 0%, #FEE140 100%)',
  connector: 'linear-gradient(135deg, #4FACFE 0%, #00F2FE 100%)',
  skill: 'linear-gradient(135deg, #F6D365 0%, #FDA085 100%)',
} as const;

export const categoryLabels: Record<string, string> = {
  general: '通用',
  departmental: '部门级',
  finance: '财务',
  document: '文档',
  audit: '审计',
  planning: '规划',
  connector: '连接器',
  skill: '技能',
};
