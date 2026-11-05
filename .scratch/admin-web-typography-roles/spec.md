# 管理台字号统一：六档角色、取值接设计令牌、守卫拦裸像素

Category: enhancement
Status: resolved——2026-11-05 进 main
出处：用户 2026-11-05 发现公司主体右栏字号与系统其余部分不一致；会话查证后给出四步建议，用户答「按你的建议」。

现状（取证于 main `497d48b28`）：公司资料读出区经 `e52c4f518` 单独「提一档」（值 14 / 标签 13），没有推到别处；设计令牌包 `@idpxyz/ui-tokens` 有字号阶梯，管理台的 Tailwind 配置没有引用，页面直接写像素，全站多种写法并存（`text-[12px]` 最多，`text-xs` 在 Tailwind 里是 12px 而令牌的 xs 是 11px）。

切片：管理台横切（不属任何 PN），只改 `apps/admin-web/**` 与一份设计文档；改到 `templates/`、`components/`，要非作者评审。

## 决定

1. 角色表：说明 11 / 正文 12 / 读出值 13 / 区块标题 14 / 标题 16 / 大数 20，取值取自 `coreTypography.fontSize`，写在 `docs/design/admin-web-typography-roles.md`（唯一权威），在 `tailwind.config.js` 里由插件生成 `type-<角色>` 类（不用 `text-` 前缀，理由见文档）；只给字号不给行高。
2. 全站机械替换：落在阶梯上的写法换角色名（视觉不变），`text-xs` 换 `text-body leading-4` 保行高；阶梯外的逐处裁定。
3. 公司主体读出区收回到与其他右栏同一把尺：值 14→13、标签 / 等宽 13→12，卡与小节标题留 14。
4. 守卫测试：源码非注释行里出现裸像素或 Tailwind 自带字号档即红。

## 子票

| 票 | 内容 | 状态 |
|---|---|---|
| [01](./issues/01-roles-sweep-and-guard.md) | 角色表、配置、全站替换、公司主体对齐、守卫 | resolved（2026-11-05 进 main） |
