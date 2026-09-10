# SA 的外部资金事实采用不发信封：`map_external_funds.go` 采用一条事实后 Outbox 里什么都没有，CC 的税费付款核对拿不到它

Category: enhancement
Status: resolved——2026-09-10 20:0x 通道 5（接管单 task-eaecfdd6，分支 `mcp5-sacc02` 基 `76932b38`）：前会话 18:0x–18:1x 写下的七件（task-c947d9a1，会话随后重置）先原样封存 `43f7c3e4`，再按票面续做 `1af6a226` + 清点 `0384f10d`；完成判据 1–3 全部落地，验证与判断题见「完成记录」。进 main 的 SHA 由推送方重放后另记。此前 in-progress——2026-09-10 18:0x 通道 5 认领（task-c947d9a1；分支 `mcp5-sacc02` 基 `76932b38`，隔离树 D:/tops/idp-parcel-mcp5-sacc02）。此前 ready-for-agent——2026-09-10 17:3x 通道 5 按通道 1 派单 task-9a2ff746（用户授权代裁）写入裁决：分区主体取租户 / 资金事实引用、更正 / 撤销复用同一事件类型带回指（见「要裁的」下「裁决」），本票再无待裁问题。此前 draft——2026-09-10 通道 4 立票（task-9880bbc9），只写票面未动代码；取证锚 `3f485e97`
Blocked by: 无

## 缺口（取证于 `3f485e97`）

- `internal/settlementaccounting/application/map_external_funds.go` 的采用编排交 `FundsFactAdopted / FundsFactExisting / FundsFactConflict`，写 `ports.ExternalFundsFactStore`；**不调任何 `HandOff*`**（`git grep -n -i handoff -- internal/settlementaccounting/application/map_external_funds.go` 零）。
- SA 的事件类型常量表（`git grep -n 'eventing.EventType' -- internal/settlementaccounting/adapters/postgres`）：代垫回收 / 回收调整 / 费用确认 / 索赔金额 / 应收 / 确认 / 索赔调整 / 成本分摊 / 经营结果 / 核销两型 / 对账单三型 / 供应商账单三型——**没有资金事实采用**。
- CC 那一半已按「应用层入口为缝」落（mech/07 CC-c：`ReceiveFundsFact` + `external_funds_fact` 登记册，`616646d`），等的正是这封信。

## 语言从哪里来

- SA `CONTEXT.md`：「银行、支付或财务系统拥有实际付款、付款失败、追加付款、资金退回、付款撤销和外部资金事实更正」；「外部资金事实撤销、更正或退回时，`UC-SA-005` 追加映射更正或核销撤销。两类历史都不得删除。」
- CC `CONTEXT.md` 生命周期：「外部资金事实接入 → 税费付款核对：按真实程序逐范围分别形成覆盖状态、差额状态和有效性状态；资金退回、付款撤销或外部资金事实更正只作为重新核对的来源事实，不能成为关务核对状态。」
- mech/07 CC-c 原句：「SA 的 `AdoptFundsFact` 不发信封，SA 适配器无 `external-funds-fact.*` 事件类型……SA 事实采用发信封 + CC inbox 消费者接进本口，另立票归 SA（MCP-3 建议：那封的分区主体取租户/资金事实引用，因为事实的更正/撤销在 SA 是新事实回指原事实）。」

## 做法

1. `ports.ExternalFundsFactHandoff`（`HandOffExternalFundsFact(ctx, intent)`）+ `adapters/postgres/external_funds_fact_handoff.go`，事件类型 `settlement-accounting.external-funds-fact.adopted`，载荷只带引用（租户、资金事实引用、采用版本），内容由消费方按引用回查——与 SA 既有 handoff 同形（`supplier_bill_handoff.go` 那一族）。
2. 采用编排在 `FundsFactAdopted` 那一格同事务交意图；`FundsFactExisting` 不重发（重放靠 `EnqueueOnce` 认领）；`FundsFactConflict` 不发。
3. 更正 / 撤销在 SA 是新事实回指原事实（ADR-0029 / CONTEXT），同一事件类型再发一封、载荷带回指——不另开「更正」事件类型（见「要裁的」）。
4. 分区主体：租户 / 资金事实引用（MCP-3 建议）——见「要裁的」。

## 红线

- 信封只带引用不带金额；金额与币种由消费方按引用读 SA 读口（单一权威）。
- 不改采用编排的四格负向代数；`UnfundableFactOutcome` 不发。
- 不在本票接 CC 消费者（那是 [03](03-cc-inbox-consumer-receives-external-funds-fact.md)）。

## 完成判据

1. 应用层：采用成功交意图一封；重放不交；冲突不交；交接失败按仓内既有形（版本已落、意图未入队 → 技术未形成 + 续办引用）。
2. 真库：`external_funds_fact_handoff_test.go` 入队一封且分区键如裁；`EnqueueOnce` 同键不翻倍。
3. `docs/agents` / 机制清点：直投路由表不改（消费者在 03）；清点 tip 重生成。

## 地盘

`internal/settlementaccounting/ports/`、`internal/settlementaccounting/application/map_external_funds.go`、`internal/settlementaccounting/adapters/postgres/`（新文件）、`cmd/parcel-api` 装配处（`map_external_funds` 的装配点，先核在哪个 assemble 文件）。

## 要裁的

1. **分区主体**：租户 / 资金事实引用（MCP-3 建议）还是租户 / 申报范围——前者让同一事实的更正链有序，后者让同范围多笔事实有序；CC 核对按范围逐笔判，本票倾向前者。归 SA owner，一句。
2. **更正 / 撤销复用同一事件类型**还是另开 `*.superseded`：仓内先例是核销「applied / reversed」两型分开；本票倾向同型带回指（事实本身就是新版本）。归 SA owner。

### 裁决

（通道 1 推送方裁、通道 5 写入，2026-09-10 17:0x；task-b941ce87 分类两条均 A、task-9a2ff746 落笔。A 类不落 ADR。）

- **1 → 租户 / 资金事实引用。** 口径是仓内既有的「ID 管幂等、分区键管顺序：同一主体的版本链排一条队」（[ADR-0069](../../../docs/adr/0069-customs-case-chain-ordering-absorbed-by-reread-and-retry.md) 决定二「分区键收窄到业务主体」；SA `supplier_bill_handoff.go` / `operating_handoff.go` 头注同句）。SA CONTEXT「外部资金事实撤销、更正或退回时……追加映射更正」——更正链是同一事实的版本链，主体就是资金事实；同范围多笔事实之间没有先后可言（CC 按范围逐笔核对，每笔各自成核对来源）。主体名「租户 / 资金事实」按 [ADR-0074](../../../docs/adr/0074-tf-object-partitions-carry-a-port-segment-apart-from-ve-parcel-partitions.md) 决定五进中心登记表，随本票落地同笔。
- **2 → 同一事件类型带回指。** 两侧 CONTEXT 已定语义：SA 更正 / 撤销是新事实回指原事实；CC「资金退回、付款撤销或外部资金事实更正只作为重新核对的来源事实」——消费方对三者一视同仁地重新核对，不需要按类型分路。核销 applied / reversed 两型是两个生命周期态而非同一事实的新版本，不构成反例。载荷带回指（原事实引用）由消费方按引用读 SA 只读口取事实内容与更正关系。

## 参照

[mech/06](../../mechanism-executor-triage/issues/06-sa-four-executors-behind-existing-uc-steps.md)「不在本票」末段；[mech/07](../../mechanism-executor-triage/issues/07-cc-four-executors-behind-existing-uc-steps.md) CC-c；[remaining-work-dd5ed934.md](../../unresolved-review-20260904/remaining-work-dd5ed934.md) 五-7；ADR-0043（EventID 认领）、ADR-0049（分区）；`internal/partycommercial/adapters/postgres/operator_registration_completed_handoff.go`（跨上下文「已登记」信封先例，`542ebc3c`）。

## 完成记录

分支 `mcp5-sacc02`，merge-base `76932b38`（main 到 `dede3c2e` 只多 `.md`，与本分支触及的代码文件零重叠；`partition_subject_registry_test.go` 与 lc/26、lc/31 各加一行会在重放时相遇，本票那一行对基线只 +1、邻行零改动）。下表 SHA 为分支上的，作封存出处；进 main 的 SHA 由推送方重放后并列补记。

| 分支 SHA | 内容 |
|---|---|
| `0ba22316` | docs(scratch)：Status → in-progress（前会话认领笔） |
| `43f7c3e4` | chore(salvage)：前会话未提交七件一字不改入库（mtime 18:04:01–18:09:15；`ports.ExternalFundsFactIntent` / `ExternalFundsFactHandoff`、`MapExternalFundsDeps.FactHandoff` 与 `existingFact` / `handOffFact`、`OutboxExternalFundsFactHandoff`（事件类型 `settlement-accounting.external-funds-fact.adopted`，ID `<租户>/funds-fact/<事实>/<版本>`，分区键 `<租户>/funds-fact/<事实>`，载荷 `tenantId / fact / version / corrects`）、应用层两例、真库四例、登记行）。非集成候选的原样封存，接管方判「用」后由下一笔调整 |
| `1af6a226` | test(sa)：先按票面写自己的判据再对封存的三件测试——判据逐条对得上；封存件没钉的一格「重放不交」的并发形（FindByKey 未见、Save 答 `FundsFactAlreadyAdopted`）补 `beforeSave` 一次性钩子与 `TestALostAdoptionRaceHandsOffTheAdoptedVersionOnceNotTheLosers`，对封存实现一次即绿、变异核（`AlreadyAdopted` 分支改交输家那份）红在版本断言上后还原；登记行收成一行（前会话插的两行注释把上方三行邻行的 gofmt 对齐一并改了，理由已在 `externalFundsFactPartitionKey` 头注） |
| `0384f10d` | docs(inventory)：机制清点在 `1af6a226` 干净检出重生成（SA 生产 81→82、测试 61→63、端口 38→39、交接口 7→8；端口合计 376→377）——只对该检出成立，推送方在 tip 兑底 |
| （本笔） | docs(scratch)：Status → resolved + 本完成记录 |

**逐条对完成判据**：1 应用层——`FundsFactAdopted` 那一格 `handOffFact` 交一封（`TestAdoptingAFundsFactHandsOffOneEnvelopeAndReplayDoesNotDouble`）；重放答 `EXISTING` 并再交同一份、替身照 `EnqueueOnce` 口径按（租户+事实+版本）认领吞掉（同例）；`FundsFactConflict` 不交（同例）；交接失败结果仍 `FUNDS_FACT_ADOPTED`、`FundsHandoffReference()` 非空、事实已落、重放补交（`TestAFailedFundsFactHandoffLeavesAContinuationAndReplayResends`）；并发形见上表。2 真库——一封、类型、分区键 `tenant-a/funds-fact/bank-fact-1`、载荷只带引用不带金额币种（`TestAnAdoptedFundsFactEnqueuesOneReferenceOnlyEnvelope`）；更正版同型再发一封、同区、载荷回指前一版本（`TestACorrectionVersionEnqueuesItsOwnEnvelopeInTheSamePartition`）；首发一行 / 回滚无痕 / 重发同一份不翻倍 / 无事务拒（`TestExternalFundsFactFollowsTheTransactionalTemplate`）；空键拒（`TestExternalFundsFactRefusesABlankKey`）。3 直投路由表零改动（`cmd/` 无 diff）；清点 `0384f10d`。

**地盘**：`cmd/parcel-api` 装配处经核**不存在**——`NewMapExternalFundsHandler` 全仓只有测试构造它，`cmd/` 下没有采用编排的生产装配点（mech/06「不在本票」写明 SA 在线登记面 / 入向消费者落地时才带装配）。本票因此不新增装配；信封在生产上的发出路径随那一笔一起来，不在这里补一个没有入向的壳。

**验证**（隔离 detached 检出 `%TEMP%\idp-verify-sacc02` @ `1af6a226`，19:2x–19:3x）：`gofmt -l` 空（干净检出，非工作副本）；`go build ./...` / `go vet ./...` 退 0；带 DSN `go test -p 1 -count=1` `./internal/settlementaccounting/... ./cmd/... ./internal/architecture/... ./internal/parcelshipment/adapters/settlementaccounting/...` 20 包全 ok（33 s）；`internal/settlementaccounting/adapters/postgres` + `cmd/parcel-api` + `cmd/parcel-dispatch` + `internal/settlementaccounting/application` 四包 `-v` **PASS 503 / SKIP 0 / FAIL 0**，票内四条真库用例逐条 PASS。反向依赖由 `go list -f '{{.ImportPath}} {{.Deps}}'` 反查 SA ports 得出（cmd 两只、PS→SA 适配器、SA 各适配器），`cmd/*` 全带 DSN。清点生成器同检出重跑零差。不跑全量（推送方那一跑是 main 真值）；`-race` 本机无 cgo 未跑。

**/code-review 自评**（基线 `0ba22316`；两个隔离子代理都在第一步 `Authentication error` 死掉——与 tasks.md 记的 harness 症状同——改由作者按 skill 正文串行两轴；**非作者评审仍由推送方另派**，本段不顶替）：Standards 0 阻断 / 2 非阻断——① `handOffFact` 与既有 `handOff`、`existingFact` 与 `existingApplication` 两两同形（Duplicated Code，判断题 ③）；② `ports.go` 的 `ExternalFundsFactIntent` 头注把 CONTEXT 与两条裁决的理由复述得偏长，单一权威上宁可只留引用（可随下次触及收短，不单独成笔）。Spec 0 阻断 / 1 非阻断——「`FundsFactExisting` 不重发」的读法（判断题 ①）。

**判断题**（供非作者评审 / SA owner 拍）：
① 做法 2「`FundsFactExisting` 不重发（重放靠 `EnqueueOnce` 认领）」读作**观察面上 Outbox 不多一封**，不读作「编排不再调交接口」：编排在 `EXISTING` 那格再交同一份（同租户、同事实、同版本 → 同 ID），由认领键吞掉。这样读的理由是完成判据 1 自己要求的「交接失败 → 续办引用」——若 `EXISTING` 跳过交接，上一次失败留下的那封永远补不上；与本文件 `existingApplication` 同形。
② 信封 `OccurredAt` 取采用时刻 `RecordedAt`，不取事实的外部发生时刻 `Fact.OccurredAt()`：事件是「已采用」不是「已付款」，与供应商账单接收封同一取法；外部发生时刻由消费方按引用读事实本身。
③ `handOffFact` / `existingFact` 与既有 `handOff` / `existingApplication` 同形不抽公共件：两对的端口类型与意图类型都不同，抽出来要么泛型要么反射，四个几行的方法读起来比那更直。
④ 并发形测试与替身 `beforeSave` 一次性钩子算在完成判据 1「重放不交」之内（真库唯一键把两步之间的另一位写入方判成 `AlreadyAdopted`，替身不开这个口到不了那一格）；钩子只在这一例用，不是预留。
⑤ 登记行不带注释、只一行：共享接线文件里加注释会拆开 gofmt 对齐块、连改三行邻行，与 lc/26、lc/31 同时进 main 时徒增相遇面；分区主体的理由留在 `externalFundsFactPartitionKey` 头注一处。
⑥ 更正版本今天只有适配器直接入队的路径（`AdoptFact` 对同引用不同内容答 `FUNDS_FACT_CONFLICT`，不在采用处顶替，与票面「外部更正走版本链」一致）——SA 侧「更正 / 撤销追加映射更正」（UC-SA-005）的编排入口不在本票，届时复用同一交接口、载荷自带回指，无需再动适配器。

## Comments

- 2026-09-10 · 通道 4：立票。未动代码。
- 2026-09-10 · 通道 5（task-eaecfdd6，接管）：前会话现场七件原样封存 `43f7c3e4`，续做 `1af6a226`、清点 `0384f10d`，票面转 resolved；完成记录、验证强度与六道判断题见上。分支已推 origin；等推送方派非作者评审后重放。
