# 管理台字号角色表

管理台的字号只用下表各档，页面按角色取名，不写像素。取值来自设计令牌包 `@idpxyz/ui-tokens` 的 `coreTypography.fontSize`，在 `apps/admin-web/tailwind.config.js` 里由一个插件生成 `type-<角色>` 类；守卫是 `apps/admin-web/src/typography-roles.test.ts`，任意值（`text-[12px]`）与 Tailwind 自带档（`text-xs`、`text-sm`……）一律不许出现。

出处：2026-11-05 用户发现公司主体右栏的字号比系统其他地方大一档。查下来是两件事叠在一起：公司资料读出区单独「提一档」，没有推到别处；系统有令牌阶梯却没接上，各页直接写像素、写法五花八门（取证于 `497d48b28`）。

| 角色 | 类名 | 值（令牌档） | 用在 |
|---|---|---|---|
| 说明 | `type-caption` | 11px（xs） | 计数徽章、角标、表单格下的提示、技术详情里的次要串 |
| 正文 | `type-body` | 12px（sm） | 表格单元、列表与右栏正文、字段标签、按钮、表单输入、元信息行 |
| 读出值 | `type-value` | 13px（md） | 详情读出里的字段值（`DetailRow` 的值、公司资料读出的值）、需要比正文醒目一点的说明行 |
| 区块标题 | `type-section` | 14px（lg） | 卡片 / 区块 / 小节标题、状态区标题 |
| 标题 | `type-title` | 16px（xl） | 页标题、右栏与对象页头的对象名、对话框标题 |
| 大数 | `type-display` | 20px（2xl） | 工作台指标卡上的数字；别处不用 |

几条约定：

- **为什么是 `type-` 不是 `text-`。** 组件库的 `cn()` 用 tailwind-merge 合并类名，它把认不出的 `text-*` 当文字颜色：`text-body` 会吃掉 `text-idpxyz-textMuted`，反过来也一样。`type-*` 它不认识、原样放行；插件生成的类排在核心字号工具之后，和组件库基础件自带的 `text-sm` 同时出现时角色类胜出。
- **一处只放一个角色类。** `type-*` 不经 tailwind-merge 去重，同一串里放两个时生效的是插件里排在后面的那一档，不是写在后面的那个；要覆盖组件库基础件的字号，放一个角色类即可。守卫也拦拼错的 `type-*` 名。
- **只给字号，不给行高。** 行高由各处自己的 `leading-*` 定；换角色名不改排版。原来用 `text-xs` 的地方换成 `type-body leading-4`，原来用 `text-sm` / `text-lg` 的换成 `type-… leading-5` / `leading-7`，保住 Tailwind 那一档自带的行高。
- **标签与正文同一档。** 字段标签与正文都是 12px，靠颜色（`text-idpxyz-breadcrumb` / `text-idpxyz-textMuted`）与字重分主次，不靠再小一档。
- **右栏与对象页统一。** 读出类的值用 `type-value`、标签用 `type-body`、等宽标识用 `type-body`；公司主体的读出区与各商业册的右栏同一把尺。
- **新加一档要改这里。** 只在现有各档都不合适时才加，且必须取令牌阶梯里的值；先改本表与 `tailwind.config.js`，再用。
