/** @type {import('tailwindcss').Config} */
// @idpxyz/* 各包以 TypeScript 源码形态发布（入口指向 src/index.ts），
// 类名在包源码里，Tailwind 必须把它们纳入扫描，否则外壳样式整体缺失。
export default {
  content: [
    './index.html',
    './src/**/*.{ts,tsx}',
    './node_modules/@idpxyz/*/src/**/*.{ts,tsx}',
  ],
  theme: {
    extend: {
      colors: {
        idpxyz: {
          bg: 'var(--idpxyz-bg)',
          editor: 'var(--idpxyz-editor)',
          sidebar: 'var(--idpxyz-sidebar)',
          activityBar: 'var(--idpxyz-activityBar)',
          titleBar: 'var(--idpxyz-titleBar)',
          statusBar: 'var(--idpxyz-statusBar)',
          statusHover: 'var(--idpxyz-statusHover)',
          tab: 'var(--idpxyz-tab)',
          tabActive: 'var(--idpxyz-tabActive)',
          tabBorder: 'var(--idpxyz-tabBorder)',
          panelBg: 'var(--idpxyz-panelBg)',
          border: 'var(--idpxyz-border)',
          hover: 'var(--idpxyz-hover)',
          activeItem: 'var(--idpxyz-activeItem)',
          accent: 'var(--idpxyz-accent)',
          text: 'var(--idpxyz-text)',
          textBright: 'var(--idpxyz-textBright)',
          textMuted: 'var(--idpxyz-textMuted)',
          inputBg: 'var(--idpxyz-inputBg)',
          breadcrumb: 'var(--idpxyz-breadcrumb)',
        },
      },
      ringColor: {
        'ds-focus': 'var(--ds-focus-ring)',
      },
    },
  },
  plugins: [],
};
