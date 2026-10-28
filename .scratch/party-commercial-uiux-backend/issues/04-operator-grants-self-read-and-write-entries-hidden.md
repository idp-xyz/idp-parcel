# 04 授予自查读口，管理台按操作者授予收起登记与发布入口

Category: enhancement
Status: resolved——2026-10-27 进 main `44b3fbc20`（见文末「进 main 记录」）。此前：作者完工，分支 claude-pcub4
Blocked by: 无（ADR-0315 由本票会话代决，待访问身份 owner 复核）
地盘：`internal/accessidentity`（信封访问器、`adapters/http` 新端点）、`cmd/parcel-api` 的操作者身份端点表与装配；管理台参与方与商业区各页（前端跟进，同票）。
出处：[party-commercial-uiux spec](../../party-commercial-uiux/spec.md) 第三段「管理台按操作者授予隐藏写入口（今天查阅员也看得到全部发布与登记入口，提交才被拒）」；先例：[group-legal-entities-uiux 票 11](../../group-legal-entities-uiux/issues/11-session-tenant-read-face.md) 的操作者绑定读口，那张票把「读口能不能顺带答出授予集」留作另一个判据。

## 现象（读码，钉 main `7457d23e9`）

- 授予在服务端操作者册上（ADR-0100），`id_token` 里没有；管理台没有地方知道「我持有什么」。
- 查阅员（演示库 `syn-operator-viewer-01`）打开参与方与商业区，看到各册的发布页签、登记页签、「发新版本」「收尾…」、停用入口，点下去才得到服务端的未授予。

## 要做的

- 新读口 `GET /operator-grants`：答本会话操作者此刻持有的能力面与运营决定种类（`{outcome, grants, decisions}`，数组、空数组不缺键）；只答自己，主数据与运营查阅读守口，认证件与操作者绑定同款，端点表 `ownIntake` 一行。信封新增只读访问器 `Grants()`、`Decisions()`。
- 管理台：只在明确知道没有登记册配置写授予时收起参与方与商业区的登记与发布入口；答不出（未配置、拒绝、依赖故障、形状不对）一律不知道、照摆。一页加载问一次，后挂载的页第一帧就拿到。深链指向被收起的页签或视图时落回读面。
- 收哪些：各册发布页签与 JSON 发布页签、登记页签（服务形态、映射、客户账户、参与方）、目录行的「发新版本」「收尾…」与详情抽屉同名动作、参与方关系撤销与到期、责任法人登记、资料修订与停用入口，以及待批准发布抽屉里的批准与发布（该抽屉随票 03 进 main，本票 rebase 后接上）。
- 不在本票：别的区（计价、网络、关务、追踪、结算……）的写入口与运营决定族；`decisions` 先交出，不用。

## 完成判据

- [x] 处理器用例：答的是认证结果上的授予，面与决定种类各一个数组，查询串里自报的主体与租户不算数；空数组不是 null；拒绝各态同口径；只收 GET；没接铸造器答未配置。
- [x] 装配用例（真铸造器）：查阅员只持读；配置员读写都持、另带决定种类；已过期的写授予不算；只有写授予没有读授予答未授予；没有令牌 401；发行方未配置 403。
- [x] 端点表与隔离读用例（`cmd/parcel-api`）通过；`internal/architecture` 通过。
- [x] 管理台：判读与「摆不摆」口径用例；演示库里查阅员登录后各页只剩读面（无写页签、无操作列、无行内动作、无「登记责任法人」），配置员登录后全部入口在。
- [x] 全量 `scripts/test-full.sh`、管理台三道门。

## 三类

读面与收入口是机制；谁持有什么授予是租户取值（授予登记，ADR-0100）。

## 规模

M

## 完成记录

- 起点：分支 `claude-pcub4` 自 main `7457d23e9` 起。
- 后端：`accessidentity.OperatorEnvelope.Grants/Decisions`（新文件 `operator_envelope_grants.go`）；`identityhttp.NewQueryOperatorGrantsEndpoint`、`NewOperatorGrantsAuthenticator`、`OperatorGrantsHandler`；`cmd/parcel-api` 的 `buildOperatorGrants` 与端点表一行 `/operator-grants`。用例：处理器四条、装配（真铸造器）一条。
- 管理台：待批准发布抽屉（`PendingDraftDrawer.tsx`）按授予收起批准与发布，没有授予时说明原因；`pages/operator-grants.ts`（`heldGrantsOf`、`mayWriteRegistry`，3 条用例）、`pages/use-operator-grants.ts`（模块级一页一问）；`withNewVersionAction` 多 `enabled`，`VersionDetailDrawer` 多 `readOnly`；服务产品、客户与合同、供应商协议、商业规则与策略四页收发布 / 登记签与行内动作；渠道产品目录、业务参与方收登记签与关系撤销；集团与法人页收登记入口、资料修订与停用菜单项（登记视图的深链落回列表）；商业规则与策略与业务参与方的地址签落回读签。
- 无 AT：访问身份没有 UC 与验收表，同操作者绑定、作业设备册的先例，用例只挂 ADR。
- 代决与越权风险点：见 ADR-0315（五点），其中点 1（对任何持查阅读的操作者开放、只答自己）与点 2（读不到当不知道、宁可多摆）需访问身份 owner 复核。
- 已知：别区的写入口今天仍对查阅员摆出；被收起的入口没有解释文案（空态与页头文字仍提到「登记」入口，只有待批准发布抽屉说明了原因）；授予中途变化要整页重载才反映。后两件的去处与状态在[上线线与冻结清单](../../launch-line-freeze-2026-10-27.md)「记录」：文案一件 2026-10-27 已在 main 修（`619a757d0`）；别区写入口仍记 R3。

## 评审记录

评审 ← 隔离子代理(sonnet) · 钉 `05b9570d2` / `690c0f9b6`（rebase 前）：阻断无；总评可进 main，七条非阻断。

- **已修**：待批准发布抽屉（随票 03 进 main）按授予收起批准与发布，没有授予时说明原因；服务产品、客户与合同、供应商协议、渠道产品目录四页的页签状态在答案到达前可能停在被收起的签上，改为派生值落回读签；`heldGrantsOf` 对 JSON `null` 响应体不再抛错，补用例；「换授予要重新登录」改为「整页重载」（令牌不变，服务端每次都重读操作者册）；`LegalEntityProfileSection` 里被隔开的注释归位。
- **已知、不改**：被收起页签的文案仍在空态与说明里被提到（票面已自陈）；spec 里的分支名等进 main 后在进 main 记录里一并订正。

## 进 main 记录（2026-10-27，作者兼推送方）

- **main**：`44b3fbc20`（快进，起点 `a5544faff`）。出处：分支 `claude-pcub4` 的代码笔 `feat(accessidentity)`、票面笔、评审修订笔（抽屉按授予收起、页签落回读签、null 响应体）与票面订正笔；推送方在 tip 上另补 ADR 索引与机制清点两笔生成物（AT 盘点不变，本票无 AT）。
- **验**：全量 `scripts/test-full.sh` 退 0（158 ok / 0 FAIL）；`adr-index -check` 与 `at-coverage-counts -check` 退 0；管理台三道门：tsc 退 0、node:test 798/798、vite build 退 0；一次性库灌种子退 0、用后删库；`internal/architecture` 通过。浏览器：演示库里查阅员登录后各页无写页签、无操作列、无「登记责任法人」，待批准发布抽屉只显说明；配置员登录后入口齐全，并在抽屉里带内容摘要与壳摘要批准成功。
- **号**：`ADR-0315`；无 AT。
- **评审**：见上「评审记录」。
