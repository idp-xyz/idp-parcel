# 18 关务立案与提交申报：生产入口与触发面

Category: enhancement
Status: in-progress——2026-10-20 分诊并认领，分支 claude-psb18。此前 ready-for-agent——2026-10-20 Claude Code 会话代 CC owner 分诊（用户原话「你作为业务和系统专家自决……解决所有的问题」），「要裁的」三问裁定见文末「分诊裁定」。更早的 needs-triage——2026-09-24 通道 2 立票（用户令通道 2 独立承接）；分诊时先定「要裁的」再转 ready-for-agent
Blocked by: 无；触发面与 [routing-first-cut/12](../../routing-first-cut/issues/12-cc-customs-applicability-judgment-for-route-candidates.md)（候选的关务适用性）、[票 10](./10-cc-declaration-channel-and-public-regulatory-reference-configuration.md) 第 1 项（申报发送连接器）相邻，分诊时划清
地盘：`internal/customscompliance`（入口与触发）、`cmd/parcel-api` 或 `cmd/parcel-dispatch` 的装配。
出处：[票 05](./05-demo-journey-criterion-evidence.md) 格 9——开发主线重定级表 PN-05 第一项记「未见缺口」，动线取证补出这一格。

## 现象

`customscompliance/application` 的 `NewEstablishCaseHandler`、`NewSubmitDeclarationHandler` 在 `cmd/` 零引用；接受决定的扇出只到 VE 与初始路由。生产进程里没有任何端点或消费门会建案或提交，外部结果口因此答「归属不上」（票 05 格 11「08 重走」）。

## 做什么

- **机制半边**：两份编排进生产装配，各有入口。
- **产品策略半边**：触发面的内置策略——谁、在什么业务时点、为哪些包裹建案；提交在什么条件下发起。

## 要裁的

1. 立案：只给运营命令面，还是另有内置触发（例如消费接受决定并按关务适用性判断派生）？若是后者，与 routing-first-cut/12 的判断口怎么分工。
2. 提交：[首发范围](../../../docs/product/PILOT-SCOPE.md)要求授权确认后提交、不启用生产自动申报——提交是否只给命令面，授权确认从哪一本册读。
3. 入口形态：隔离形态下照 operator-channel/08 放行，生产形态等操作者渠道（ADR-0149 前线操作者族）。

## 完成判据

- 生产进程至少有一条路能建案并形成提交版本（隔离形态实测只记 `S`），票 05 格 9 越过；触发面有内置策略或显式的命令面，没有默认的自动申报。

## 分诊裁定

2026-10-20，Claude Code 会话代 CC owner 裁（用户原话「你作为业务和系统专家自决……解决所有的问题」）。读过：PILOT-SCOPE 关务各条、CC `CONTEXT.md`、UC-CC-001 / 004 / 005、ADR-0091 / 0146 / 0149 / 0150 / 0151 / 0204 / 0212 / 0222，operator-channel/08，routing-first-cut/12，本目录 05、07、10。决定与逐条越权风险点落在 ADR-0227，这里只记三问的答案。

1. **立案**：本票只给显式的运营命令面 `POST /customs-cases`，挂操作者渠道「运营决定」能力面的新一种决定（建案）。包裹由请求指名，客户归属与来源资料引用由服务端按信封租户在 parcel-shipment 查当前已接受委托取出，不从请求体收；查不到已接受委托答未受理（UC-CC-001「只有提交版本而尚未接受的范围不得建立正式关务案件」）。要不要建案仍只由建案要求规则册答，未登记答未决、指名未配置。**内置触发不在本票**，另立 [35](./35-built-in-customs-case-trigger-from-acceptance.md)：接受决定不带监管程序与法定义务范围，routing-first-cut/12 的关务适用性判断按路由候选作答、只答可用 / 不可用 / 状态未知，也不产出程序与义务；派生立案要一套「已采用路由的关务段 → 辖区、方向、程序、义务」的判断方法，那是新的产品策略。分工：rfc/12 的判断口是路由判断的输入，不触发建案，也不被建案当依据复用；35 若做，读接受决定与已采用路由，仍经建案要求规则册判。立案不等于申报，内置立案与「不启用生产自动申报」不冲突——不在本票是因为它不便宜、边界没定，不是因为它被禁止。
2. **提交**：只给命令面，生产进程没有任何自动提交路径。批准与实际提交分两口、两种运营决定（CONTEXT「请求、批准、拒绝、撤销、申报渠道账号使用和实际提交权限必须分别表达」）：授权确认 `POST /customs-submission-authorizations`（UC-CC-004 首发人工确认：确认人取信封里的操作者，单元当前就绪须有效，就绪没登答未决、指名未配置）；提交执行 `POST /customs-declaration-submissions`（UC-CC-005：就绪与授权须同时有效，单元上没有授权答未决 `AUTHORITY_UNCONFIGURED`）。授权读 access_identity 操作者册的运营决定授予（决定种类加准入范围），三种决定种类追加进 `accessidentity` 与迁移 `access_identity/0018`；谁被授予哪一种是租户取值，没有授予答 `OPERATOR_NOT_GRANTED`。申报就绪仍由既有的就绪判断册承担，与授权分开。提交形成不可覆盖的提交版本与首次提交尝试；没有发送连接器，尝试记「已登记待发送」（UC-CC-005 结果契约「提交已登记待发送」），不带发送时间，不收调用方给的尝试结果与发送时间——迁移 `customs_compliance/0027` 放开这一格。发送与「尝试已发起」那一步归 [票 10](./10-cc-declaration-channel-and-public-regulatory-reference-configuration.md) 第 1 项。
3. **入口形态**：照 ADR-0150 决定三与最近先例（ADR-0204、ADR-0212），三口直接挂操作者渠道的「运营决定」能力面，合成租户 `SYN-TENANT-01` 与真实租户走同一条命令面，不加隔离放行（写开关换不了运营决定口）。ADR-0149 的前线操作者族已由 operator-channel/15 落地，票面「生产形态等操作者渠道」那一半不再等。

UC-CC-004 的拒绝、撤销、再次申请，授权目标绑定到组成 / 资料 / 渠道，受理口组成核对案件关联，以及既有授权登记 CLI 与业务角色确认并存的处理，另立 [36](./36-submission-authorization-business-role-face-remainder.md)。
