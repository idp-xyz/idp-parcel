import { coreTypography } from '@idpxyz/ui-tokens';

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
  // 字号只用这几档角色（docs/design/admin-web-typography-roles.md 是唯一权威），取值来自设计令牌的字号阶梯。类名用 `type-` 前缀而不是
  // `text-`：组件库的 cn() 经 tailwind-merge 合并类名，它把认不出的 `text-*` 一律当文字颜色，`text-body` 会和 `text-idpxyz-*` 互相吃掉；
  // `type-*` 它不认识、原样放行。插件工具排在核心字号工具之后，与组件库基础件自带的 `text-sm` 同时出现时角色类胜出。只给字号、
  // 不给行高：行高由各处自己的 leading-* 定。守卫在 src/typography-roles.test.ts。
  plugins: [
    function typographyRoles({ addUtilities }) {
      const roles = {
        caption: coreTypography.fontSize.xs,
        body: coreTypography.fontSize.sm,
        value: coreTypography.fontSize.md,
        section: coreTypography.fontSize.lg,
        title: coreTypography.fontSize.xl,
        display: coreTypography.fontSize['2xl'],
      };
      addUtilities(Object.fromEntries(Object.entries(roles).map(([role, size]) => [`.type-${role}`, { 'font-size': size }])));
    },
  ],
};
