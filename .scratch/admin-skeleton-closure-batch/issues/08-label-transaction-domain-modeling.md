# 面单交易：全仓没有这张表，缺的是领域建模不是接线

Category: feature
Status: resolved
Session: MCP-5（2026-08-31 认领，重启范围见文末 Comments）
Blocked by: 无（但与本批其余各票性质不同，不并线）

## 为什么单列

其余十三张骨架页缺的是读面或数据，**这一张缺的是建模**。取证于 `65b6cf2`：全仓
`migrations/` 下搜不到任何 `label_transaction` 表，`parcel_shipment` 的十二张表里没有一张承载
面单交易（依次是 `acceptance_adopted_resolution`、`acceptance_financial_control`、
`acceptance_processing_attempt`、`acceptance_reachability_judgment`、
`commercial_resolution_key_registration`、`customer_source_data_version`、`final_outcome`、
`intake_adoption`、`parcel_cancellation`、`shipment_request`、`source_submission`、
`source_submission_observation`）。

所以这一票不是「补读面」，是从领域语言开始：域模型 → 迁移 → 写入方 → 读面 → 端点 → 页。
工作量与性质都与票 02–06 不是一回事，混进那三条并行线会把它们的完成判据搅浑。

## 已有的领域语言（出处在，形状未定）

`apps/admin-web/src/navigation.ts` 的 `moduleInfoById['label-transactions']` 指向
`docs/domain/parcel-shipment/CONTEXT.md`，其中已列出这些词：**面单交易**、**交易级与包裹级
渠道业务结果**、**面单交易定案**、**包裹级关闭或重开请求与决定**、**渠道角色与责任依据快照**。
词在，但没有任何代码或迁移实现它们。

## 为什么现在不派

三条理由，任一成立都够：

1. **难逆转取舍要走 ADR**。面单交易的聚合边界（交易级与包裹级如何分、定案后如何封口、关闭与
   重开的决定如何留痕）是结构性决定，按 AGENTS.md 要新 ADR，不能在实现票里顺手定。
2. **写入方在墙后面**。面单交易随委托提交产生，而接入渠道墙拦着上游——即便建完模，这一页
   仍是空册。它不比票 02–06 更急。
3. **地盘冲突**。`internal/parcelshipment/**` 此刻有他会话的在途未提交改动
   （`adapters/inbox/shipment_request_submitted_consumer.go`、
   `application/advance_acceptance_chain.go` 及其测试）。按 parallel-sessions，地盘要由人在开工
   时分派，越界前先在频道说一声、对方让位再动。

## 重启条件

上述三条各自解开：①一份定聚合边界的 ADR 已落；②承接会话由人指定且 `internal/parcelshipment/**`
的在途改动已落地或已让位。届时本票范围改写为完整一条从建模到接线的线。

## 参照

`docs/domain/parcel-shipment/CONTEXT.md`（面单交易一族术语）；
[本批 spec](../spec.md) 的事实基线；`docs/agents/parallel-sessions.md` 的地盘一节。

## Comments

- 2026-08-31 · MCP-5：**重启并认领。** 频道指示「这两个页面骨架（acceptance-review、
  label-transactions）完整实现」。三条不派理由逐一核销（基线 `4b35815`）：
  1. ADR——聚合边界 ADR 作为本票第一件交付先落（编号以 `docs/adr/README.md` 下一空位为准），
     不在实现里顺手定；
  2. 写入方在墙后——**维持成立，且首发基线明写「独立面单渠道服务不进入首发生产」**。因此本票
     范围按机制半边裁：域模型、迁移、仓储写入机制（构造与不变式由测试驱动）、读端口、真库读
     适配器、HTTP 查阅端点、隔离读准入（ADR-0078 读行）、页面接真。**不造任何伪写入方**：
     渠道墙未降前登记零行，页面如实呈现空册（同批 05/06 阶段二的先例）。写编排（随委托提交
     产生面单交易）等渠道墙降后另票；
  3. 地盘——`internal/parcelshipment/**` 的在途改动已随 `4b35815` 批收口落地，工作树此刻
     除他人 `docs/wooolink/`、`.scratch/admin-write-faces/` 外干净。已在频道广播认领
     `internal/parcelshipment/**`、`cmd/parcel-api/**`、`apps/admin-web/src/**`、
     `migrations/parcel_shipment/**` 与本批票面。
  同批新开 [09-acceptance-review-wiring](./09-acceptance-review-wiring.md) 承接另一页，两票
  分开提交。

- 2026-09-01 · MCP-5：**域模型半边交付**（随 `9fbcf24` 入库，MCP-1 代提）。ADR-0084 逐条落地：
  独立聚合 `LabelTransaction`（键租户 + 交易标识）、建立即固定覆盖与七项依据、双层结果一次
  记录且只作结构校验、`Finalized()` 是派生谓词无存储列、后续动作追加式不改写原结果、重试与
  替代是新交易的出生属性、重建门逐字段过校验。继续尝试决定按决定六**不进聚合**，在类型注释
  里写明它与包裹终局同为刻意缺席项。另加三条 ADR 未写的时间序不变量（提交 ≥ 建立、结果 ≥
  提交、后续动作 ≥ 结果），依据是 CONTEXT「迟到事实按业务发生时间改变交易归类」要求时间轴
  自洽；MCP-1 复核后判为「在实现里兑现 CONTEXT」，保留。
  两道静态门禁按其自身规则登记：`production_wiring_baseline.txt` 加两条未接线工厂并写明
  重启条件，`rehydration_gate_test.go` 把重建入口两条补进受限名单。

- 2026-09-01 · MCP-5：**剩余半截交付**（迁移、仓储、读面、端点、装配、页面）。逐件：
  1. `migrations/parcel_shipment/0010_label_transaction.sql`——键、乐观版本、状态、快照与
     建立时间五列加租户维索引。**刻意没有 `finalized` 列**：决定四说投影列「允许存在」不是
     要求，而 `state` 已在列上，多存一列就是把同一事实写两处。
  2. `ports.go` 两组口：`LabelTransactionRepository`（Insert/Save 各自的写入代数，按 ADR-0031
     不复用委托那两套）与 `LabelTransactionViews`（签名收租户与 limit，客户账户不上签名——
     覆盖包裹可跨委托，按账户过滤会把跨客户交易归给其中一个客户）。读记录按「交易 × 包裹」
     摊开，`HasResult` 与 `Accepted` 分立：结果未回读成未受理就是把「结果不确定按失败处理」
     搬到读面。
  3. `adapters/postgres/label_transaction.go` 与 `label_transaction_views.go`——写入走全聚合
     快照、读回逐字段过重建门；读面直读状态列与快照不经重建门，定案用
     `domain.LabelTransactionState.IsChannelResult` 派生（为此把该谓词导出，领域测试加了一条
     「聚合与读面同源」的断言）。继续尝试判断由具名函数把 CONTEXT 规则应用在**空决定历史**上，
     注释写明它不是常量 `true`，登记册落地后只换输入。
  4. `adapters/http/query_label_transactions.go`——`GET /label-transactions`，行粒度摊开在读侧，
     空册是 200 + 空数组（与 403 未配置分得开），答复带 `continuedAttemptBasis` 让页头能如实
     转述派生依据。Intake 复用 `ShipmentRequestViewsIntake` 但只取其租户维，端点注释写明由此
     产生的后果：这是运营查阅面，部分账户授权在这一口上不收窄。
  5. `cmd/parcel-api`——端点表、`main` 装配、`unwiredLabelTransactions` 占位、
     `businessEndpointProbes` 与 `isolatedReadAdmittedPatterns` 各补一行（复用同一 Intake 变量
     必然随之放行，漏登就是启用态 500 撞期望 403）。
  6. `apps/admin-web`——`LabelTransactionsPage` 接真（`GET /label-transactions`），
     `api.ts` 加行形状与调用口，`page-registry.tsx` 的 `liveIds` 登记 `label-transactions`。
     空册文案写明「读取入口已配置、登记册为空」及其原因，不写「尚未实现」；包裹级结果三态
     呈现（结果未回 / 已受理 / 未受理），后续动作与原结果并列不改写。
  **验证**：全仓 `gofmt -l` 无输出、`go build`、`go vet` 绿；`go test -count=1 ./...` 绿且
  **含真库**（DSN 指本机 `postgres:16.14` 门禁库，面单交易 8 条真库用例单跑 `-v` 得 PASS
  非 SKIP）；`apps/admin-web` 的 `tsc --noEmit` 无输出、`pnpm build` 绿。
  **仍在墙后**：写编排（随委托提交产生面单交易、渠道结果回填）等渠道墙降后另票；继续尝试
  决定登记册（含截断边界与重开链建模）另票，重启条件是写面裁决或渠道墙任一先到。

- 2026-09-01 · MCP-5：**双轴评审后的两处修复**（固定点 `df51ce0`，即 `9fbcf24^`）。
  1. **Standards 两条硬违反已修。** 六处 Go 注释写的「CONTEXT 规则 141/142/146/161」其实是
     **文件行号**——`CONTEXT.md` 的规则是无编号的项目符号，而 AGENTS.md 明禁行号引用且写明
     「Go 注释里的跨文件引用同受此约束」。全部换成引文；源头 ADR-0084 的 Links 行同样改掉
     （只动指认方式，Decision 与 Consequences 一字未改）。另一条是 `adapters/postgres` 里
     「重建门对全部**六个状态**都开」，数的是 domain 包的枚举，改为「对状态集合里的每一格」。
  2. **读面准入另立一种形**（MCP-1 裁决：不接受原样上线，要求「S3 那天变红而不是变漏」；
     两个形状里我取它倾向的那个，把区别做进类型）。原先本端点收 `ShipmentRequestViewsIntake`
     交出的 `AuthorizedQueryScope`，而那个类型的构造器**拒绝空账户集**——把一个必带账户维的
     作用域交给一个不按账户过滤的读面，等于在类型上声称它会过滤而它不会。今天命令面与查阅面
     都挂着未配置 Intake、隔离读又注入合成租户，所以这个谎不出声；真 Intake 接上那天它才
     出声，而那时没有任何东西会红。改法：新增 `LabelTransactionQuery{Tenant, Limit}` 与
     `LabelTransactionQueryIntake`，端点只收它；`UnconfiguredIntake` 与
     `IsolatedOperationsReadIntake` 各自实现，隔离读仍是同一开关、同一装配点、同一个注入值
     （决定七要的「沿用同一开关」不变），只是两种查阅面收到的作用域形状不同——那是决定七
     同一句话的另一半。**不带授权凭据引用**：读口消费的只有（租户, limit），带一个没人读的
     字段是造第二个来源；运营查阅若日后要留授权痕迹，那是给本上下文引入「运营查阅作用域」
     值对象的另一笔工作，要过 CONTEXT。
  **评审未修的部分**（Spec 轴，留待裁决）：三道由我推出而 ADR 未写的闸（后续动作需已定案、
  未受理不得带包裹级标识、重试需原交易已定案）保留原样。
  交易级外部标识（渠道订单号/主号）在聚合上没有落脚点这一条**不在本票补**：它是既有票
  [ps-external-mark-relations/01](../../ps-external-mark-relations/issues/01-external-mark-relations-have-no-model-in-parcel-shipment.md)
  的范围，那张票已论证过「一张 mark→parcel 映射表从 schema 层面就错」——被标识对象不止包裹、
  替代关系要留历史、适用范围是必填维。在本票里顺手给交易加一个「渠道订单号」字段，正是那张
  票否掉的形状的一个分身。
  **复验**：全仓 `gofmt -l` 无输出、`go build`、`go vet` 绿；`go test -count=1 ./...` 绿且含
  真库。
