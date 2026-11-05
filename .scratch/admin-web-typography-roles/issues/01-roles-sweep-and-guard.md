# 01 角色表、配置、全站替换、公司主体对齐、守卫

Category: enhancement
Status: resolved——2026-11-05 进 main（本笔提交）
父票：[spec](../spec.md)

## 完成判据

- 三道门退 0；守卫测试在。
- 替换前后差异只在字号类（脚本核对）。
- 浏览器实测：公司主体右栏不再有 14px 正文，值 13 / 标签 12 / 卡标题 14 / 对象名 16，与服务产品等右栏一致。

## 完成记录（2026-11-05，直接在 `main` 上做，自 `497d48b28`）

- 落点：`tailwind.config.js` 用插件生成 `type-caption / body / value / section / title / display`，取值读 `@idpxyz/ui-tokens` 的 `coreTypography.fontSize`；`src/**` 的字号写法全部换成角色类（阶梯上的视觉不变，`text-xs`→`type-body leading-4`，`text-sm`/`text-lg` 补 `leading-5`/`leading-7`，阶梯外逐处裁定）；公司资料读出四个组件值 14→13、标签与等宽 13→12，卡与小节标题留 14；守卫 `src/typography-roles.test.ts` 两条（拦任意值与 Tailwind 自带档及 `text-<角色>`；拦拼错的 `type-*`）；`src/test/node-builtins.d.ts` 补两格声明；文档 `docs/design/admin-web-typography-roles.md` 与 `docs/README.md` 一条。顺带把「操作者与授予」右栏对象名从 14 改到标题档。
- ✅ 三道门：tsc 0、node:test 1205/1205、vite build；生成的 CSS 里六个角色类取值与令牌一致，且排在核心字号工具之后。
- ✅ 替换核对：脚本比对改动行，除字号类外无任何差异。
- ✅ 浏览器实测（5175 指演示 API）：公司主体右栏值 13 / 标签 12 / 卡标题 14 / 对象名 16，不再有 14px 正文；往来单位、服务产品、操作者与授予右栏同一把尺；描边按钮带 `type-body` 时保住变体颜色。
- 评审：改到 `templates/`、`components/`，非作者评审第一轮 BLOCK——角色类用 `text-` 前缀时会被组件库 `cn()` 的 tailwind-merge 当颜色吃掉；改用 `type-` 前缀 + 插件后复核 APPROVE。另两条遗留（一处放两个角色类不去重；拼错的 `type-*`）已在文档与守卫里补上。
