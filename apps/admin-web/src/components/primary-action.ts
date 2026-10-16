// 主动作的一份类名（票 group-legal-entities-uiux/05）。它只是一串 class，不是组件，用法同 registration/chip.ts：页面写
// `<Button variant="default" className={primaryActionClass}>`，由 Button 里的 cn（twMerge）拿这串类顶掉 default 变体的
// 同组类。
//
// 为什么不直接用上游的 default 变体：它的底色与悬停是 `bg-idpxyz-accent/20`、`/30`，本仓主题色是裸 CSS 变量，Tailwind
// 不为它生成透明度变体（票 admin-web-group-legal-entities/15 对构建产物实测），实际渲染出来只剩强调色字、没有底，与
// 不传 variant 时的 secondary（灰底灰字）一样读成禁用态，而筛选胶囊的激活态反倒是实心蓝。这里一律用实色与
// color-mix，不写 `/NN`。
//
// 白字配强调色实底：演示部署两套主题的强调色都是 #007acc，白字对比 4.51:1，过 AA 的 4.5:1；悬停压暗到 85%，约 5.9:1。
// 强调色换掉（ui-tokens 给 parcel 登记 productAccent 时）要重算这一格——更浅的强调色配白字可能掉到 4.5 以下。
// 聚焦环同理：上游基类的 `focus-visible:ring-idpxyz-accent/35` 也生不出来，这里另给一圈实色环，外隔一圈页面底色。
export const primaryActionClass = [
  'bg-idpxyz-accent',
  'text-white',
  'font-medium',
  'hover:bg-[color-mix(in_srgb,var(--idpxyz-accent)_85%,black)]',
  'hover:text-white',
  'focus-visible:ring-2',
  'focus-visible:ring-idpxyz-accent',
  'focus-visible:ring-offset-1',
  'focus-visible:ring-offset-idpxyz-bg',
].join(' ');
