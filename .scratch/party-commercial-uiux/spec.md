# 参与方与商业区 UI/UX 评估与跟进

Category: enhancement
Status: resolved——第一段 01–06、第二段 07–21 resolved；第三段（要后端）仍在「后续」，未拆票

出处：2026-10-25，用户在会话里要求「评估 commercial party 的 UI/UX」，读完评估后说「按你建议的顺序开始」。建议的顺序是：先改文案和组件（成本低），再做要设计的，最后是要后端配合的。本批先落第一段，第二、三段列在「后续」，等第一段收口后再拆票。

## 评估方法

- 演示环境照 `scripts/demo-seeds/README.md`「操作者渠道」一节起：一次性库灌种子，`parcel-api` 连操作者渠道，管理台走开发代理。演示租户 `SYN-TENANT-01`。
- 用配置员账号 `syn-operator-config-01` 登录，在真浏览器（Windows Chrome 无头，经 CDP 驱动）里走了 7 个列表页、法人详情栏、参与方抽屉、关系抽屉，以及客户合同、价格政策、信用政策三张发布表单和法人登记表单。宽度 1440 与 1920 两档。
- 另有两个只读子代理分头通读 `src/pages/party/` 的浏览类与写入类代码，结论经会话在浏览器里抽验后才收进本表。子代理有一条「发布四口未接通」在演示环境里实测不成立（词表口答 200），已剔除。
- 取证时 main 在 `d19eeaafc` 到 `f8b25837b` 之间；其间 code/25 改过价格政策表单的体积系数一格，与本批发现无关。截图只存在会话临时目录，没有入库。

**这次没覆盖到的：** 没用查阅员账号看只读体验（「查阅员也看得到全部写入口」一条出自代码）；没有走完一次真发布（会写演示库）；深色主题没看。

## 做得好的（保持）

- 页面状态如实：加载、出错、空册、筛空、未配置、需要登录、没有授予各自呈现，不拿演示数据顶替。
- 集团与法人页是全区样板：KPI 可点即筛、主从分栏、生命周期条、复制、未保存提醒；法人登记表单的分组与白话说明也是。
- 发布流程改一格即作废此前的预览与各步，单人场景下杜绝「预览 A、发布 B」。
- 每页只读一两个口，演示数据下 0.6 秒内答完。

## 发现与去向

| 发现 | 去向 |
|---|---|
| 页面说明、册说明、空态、登记结果词里是开发理由与实现名：「版本壳」「上列」「两列不上」「发布口对象类别 … 喂入」「声明通道」「受理门」「受控 CLI parcel-commercial …」「一个字节没写」 | [01](issues/01-browse-pages-speak-business.md) |
| 发布流程与十张发布表单：卡片标题挂 `kind = CREDIT_POLICY`，节标题挂 `(0020)`，说明里有 ADR、AT、端点、`planId@planVersion`、载荷 / 构造门 / 载体 / 信封 / 逐字节 | [02](issues/02-publication-forms-speak-business.md) |
| 预览被拒时格下是服务端英文与 Go 原话：`party commercial: blank value: commercial object ID`、`parsing time "" as "2006-01-02T15:04:05…"` | [03](issues/03-field-problems-in-chinese.md) |
| 同一套状态，法人、参与方、关系是徽章，合同、协议、产品、政策、客户账户是纯文字；列头有「状态」「身份状态」「生命周期状态」三种叫法 | [04](issues/04-status-badges-everywhere.md) |
| 商业规则与策略页：10 个页签挤成两行，9 个册按钮逐字竖排，右端计数压住「保存视图」，说明被截成「商业规则…」；1920 宽同样。JSON 镜像口与主路径并列。参与方身份表里能用的排序下拉与模板禁用的「排序」位并排 | [05](issues/05-commercial-policies-layout.md) |
| 法人页「更多」菜单写「到业务参与方页停用 SYN-PARTY-…」，点了只到参与方列表；停用表单是普通主按钮、无确认、不说不可逆 | [06](issues/06-legal-entity-deactivation-entry.md) |

## 不立票及理由

- **列表模板常驻的四个禁用位**（排序 / 视图 / 保存视图 / 更多筛选）：是票 [admin-web-ux-alignment/03](../admin-web-ux-alignment/issues/03-list-template-monitor-golden.md) 按黄金标准 Rule 2「即使功能未完整，也要按完整模板布局」做的，36 页共用。评估认为它对配置员是噪音，但推翻它要用户裁定，本批不动；只在 05 里修参与方页「能用的排序」与「禁用的排序」并排的矛盾。
- **列表时间每格带秒与 UTC+8**：各册页共用的取舍（[group-legal-entities-uiux](../group-legal-entities-uiux/spec.md)「不立票及理由」已记），本批不单改。
- **注册号类型照原码显示**：同上一份 spec 已记的有意取舍。

## 后续（不在本批）

第二段，要设计，本批收口后再拆票：

- ~~引用一律显示「名称 + 标识」并可点到对象页~~ → [07](issues/07-references-are-links-with-names.md)。
- ~~商业版本按对象分组、标出在用版本~~ → [08](issues/08-versions-grouped-with-in-use-filter.md)；~~合同、协议、产品、映射、政策 5 页行可点开详情与版本历史~~ → [21](issues/21-row-detail-and-version-history.md)，映射的历史修订要后端补读口，仍在后续。
- ~~发布表单统一时刻控件~~ → [10](issues/10-publication-moments-in-local-time.md)。
- ~~引用他物的格一律用选择器~~ → [13](issues/13-references-picked-not-typed.md)；~~候选多时的可搜索选单~~ → [20](issues/20-searchable-pickers.md)。
- ~~发布前给人读得懂的预览卡与确认框~~ → [11](issues/11-readable-preview-and-publish-confirmation.md)；~~正文各册的可读摘要卡、批准与发布分开按钮区~~ → [19](issues/19-decision-area-and-body-summary.md)。
- ~~切页签不丢草稿~~ → [09](issues/09-drafts-survive-tab-switches.md)；~~未保存提醒推广到全部写入页~~ → [17](issues/17-unsaved-draft-warnings.md)，应用内换模块、关壳上页签的拦截在壳上，共用登记签几处未接，仍在后续。
- ~~「基于此版本发新版本」预填~~ → [14](issues/14-new-version-from-existing.md)；~~标识旁列出在册版本、提示区间重叠~~ → [15](issues/15-existing-versions-note.md)。
- ~~管理台上给旧版收尾（截止在新版生效时点，或同起点退役）的入口~~ → [16](issues/16-close-out-old-versions.md)；服务产品、客户合同、授权规则、接受前财务控制策略、客户服务规则的目录读面不回显截止点（只有供应商协议回显），截止后目录仍显原区间，要后端补读面，归第三段。
- ~~接入态与拒绝态（共享件 `catalogue-view.ts`、`components/states`、`components/registration` 的「提交打到 …」）把端点、错误码、文档出处收进「技术详情」~~ → [18](issues/18-technical-details-folded.md)；别的模块自拼文字的码句与自写的状态码文字未动。
- ~~业务参与方、参与方关系两张登记表单的开发话~~ → [12](issues/12-registration-forms-speak-business.md)。

第三段，要后端，走[并行会话](../../docs/agents/parallel-sessions.md)，交协调方排队：

- 预览与发布的逐格问题改回问题码加参数，前端按码查中文（03 是前端先行的过渡）。
- 客户合同读口带出货主客户账户与责任法人，列表才能有「客户」一列。
- 待批准发布的列表与读回口，批准人才能接手别人存的稿；批准、发布带期望内容摘要，防并发时批到别人的稿。
- 管理台按操作者授予隐藏写入口（今天查阅员也看得到全部发布与登记入口，提交才被拒）。
- ~~服务产品、客户合同、授权规则、接受前财务控制策略、客户服务规则的目录读面回显截止点~~ → [demo-chain-gaps/184](../demo-chain-gaps/issues/184-five-commercial-catalogues-do-not-echo-the-cut-of-a-closed-version.md)，已进 main。
- 产品—渠道映射的历史修订读口（目录只给最新一笔修订）：有了，渠道产品目录页行详情里的历史栏才能列出之前各笔。

## 子票

01–21 只改 `apps/admin-web` 与本票面，走 [workflow.md「前端切片」](../../docs/agents/workflow.md#前端切片一人在-main-上直接做)。与上一节的差别一处：当时有别的会话在主树上做合入，本批在独立 worktree（第一段 `idp-parcel-pcux`，第二段 `idp-parcel-pcux2`、`idp-parcel-pcux3`、`idp-parcel-pcux4`、`idp-parcel-pcux5`、`idp-parcel-pcux6`、`idp-parcel-pcux7`、`idp-parcel-pcux8`、`idp-parcel-pcux9`）上做，每笔先 rebase 到 `origin/main` 再推确切 SHA，不碰主树。

| # | 票 | 阻塞于 | 状态 |
|---|---|---|---|
| 01 | [浏览页说业务话](issues/01-browse-pages-speak-business.md) | — | resolved |
| 02 | [发布流程与发布表单说业务话](issues/02-publication-forms-speak-business.md) | — | resolved |
| 03 | [逐格问题说中文](issues/03-field-problems-in-chinese.md) | — | resolved |
| 04 | [状态一律用徽章](issues/04-status-badges-everywhere.md) | — | resolved |
| 05 | [商业规则与策略页布局](issues/05-commercial-policies-layout.md) | 01 | resolved |
| 06 | [法人停用入口与确认](issues/06-legal-entity-deactivation-entry.md) | — | resolved |
| 07 | [引用可点、显示名称](issues/07-references-are-links-with-names.md) | — | resolved |
| 08 | [版本按对象排在一起、可只看在用的](issues/08-versions-grouped-with-in-use-filter.md) | — | resolved |
| 09 | [切页签、换册不丢草稿](issues/09-drafts-survive-tab-switches.md) | — | resolved |
| 10 | [发布表单的时刻按本地时区填](issues/10-publication-moments-in-local-time.md) | — | resolved |
| 11 | [发布前看得懂要发什么，发布前确认](issues/11-readable-preview-and-publish-confirmation.md) | 10 | resolved |
| 12 | [两张登记表单说业务话](issues/12-registration-forms-speak-business.md) | — | resolved |
| 13 | [引用他物的格从册里选，不手敲标识](issues/13-references-picked-not-typed.md) | — | resolved |
| 14 | [基于此版本发新版本](issues/14-new-version-from-existing.md) | — | resolved |
| 15 | [版本信息一节里列出已有版本，提示撞号与区间重叠](issues/15-existing-versions-note.md) | — | resolved |
| 16 | [目录行上给旧版收尾：截止在后继生效时点，或同起点退役](issues/16-close-out-old-versions.md) | — | resolved |
| 17 | [写入页的未保存提醒：刷新关页有确认，「发新版本」换草稿前先问](issues/17-unsaved-draft-warnings.md) | — | resolved |
| 18 | [接入态、拒绝态与登记签里的接口路径、错误码、文档出处收进「技术详情」](issues/18-technical-details-folded.md) | — | resolved |
| 19 | [发布流程：批准与发布分开按钮区，预览卡加各册正文要点](issues/19-decision-area-and-body-summary.md) | — | resolved |
| 20 | [候选多时，引用选单可以搜索](issues/20-searchable-pickers.md) | — | resolved |
| 21 | [目录行点开详情，带这个对象的版本历史](issues/21-row-detail-and-version-history.md) | — | resolved |

## 界面用词对照（本批各票共用）

领域原词照留，配一句解释；实现层的叫法换成白话。改的是上屏文字，代码注释与标识符不在此列。

| 今天上屏的 | 改成 | 说明 |
|---|---|---|
| 版本壳、壳 | 版本信息 | 对象标识、版本号、适用范围、有效区间这一组 |
| 正文 | 正文（领域原词，保留） | 首次出现处说一句「这一版的条款内容」 |
| 载体 | 发布稿 | 领域名「待批准发布」；状态照旧是待批准 / 已批准 / 已发布 |
| 内容摘要、摘要 | 内容摘要（保留），步骤名「预览摘要」改「预览」 | 解释为「按内容算出的指纹，用来核对批准与发布的是同一份」 |
| 规范化版本 | 不上主视图 | 放进技术详情 |
| 受理门、构造门、写入门、发布口、声明通道 | 删；需要时说「登记册」「服务端」 | |
| 上列、转写、坐标、喂入 | 删，或换成「列出」「取自」 | |
| 恰一、恰一格在场 | 只填其中一项 | |
| 在场 | 已填写 | |
| 一个字节没写 | 没有写入任何内容 | |
| `kind = …`、`PRICE_POLICY` 一类类别码 | 不上主视图 | 需要时放技术详情 |
| ADR、AT、PAR 编号，端点路径，CLI 命令，docs 路径 | 删 | 留在代码注释里 |
| 时点锚声明 | 接受判断时点策略 | PC CONTEXT 全名「接受判断时点选择策略」，册按钮上取短名，说明里给全名 |
