# 32 VE 代码注释里有会腐的计数、不是原文的「」引文与错指，测试 Covers 仍点已迁走的 `AT-TF` 号

Category: chore
Status: ready-for-agent——R3（不急）：只是注释，不改行为；读注释的作者会被计数、假引文与错指误导。2026-10-21 文档自洽 04 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc04。
Blocked by: 无
出处：[audit/ve.md](../audit/ve.md) VE-20、VE-21（归 A，但改的是 Go 注释，修订票不改代码，故立本票）；VE-19 的迁号（本票同批，UC-VE-002、UC-VE-004 新立 `AT-VE-189`–`195`）。取证于 main `205eb254`，下列各处原样仍在。

## 现象

三件同根（注释与现行文档、与 AGENTS.md「改文档」的引用规范不符），合成一张：

1. **会腐的计数**（VE-20）：
   - `adapters/parcelshipment/derive_on_final_outcome.go` 包注释「终局投影是四路投影里唯一一条来源为 parcel-shipment 的」。
   - `cmd/parcel-dispatch/assemble.go` 的 `newTenantBoundProjectionDerive` 头注「九路投影共用一份……不要为揽收、交付、交接、终局、初始路由、异常旅程、关务案件、申报提交再复制八份包装」，外部追踪、外部结果、移动事实与两路补派生都不在列。
   - `adapters/postgres/notification_policy.go`「理由同本包另外三个视图」。
   - `adapters/transportfulfillment/derive_on_transport_handover.go` 与 `derive_on_effective_delivery.go`「第三个源接上来时依同一条不变量……」，给出替代关系的适配器早已不止三个。
2. **不是原文的「」引文与错指**（VE-21）：
   - `adapters/http/query_claims_recovery_records.go` 引「CONTEXT『三判分步』」与「CONTEXT 边界『赔付金额的计算』」：两句在 VE CONTEXT 里都查不到（`receive_claim.go` 那一处已把「三判分步」写成转述并引了原句，可照它改）。
   - `application/send_disposition_request.go` 引「（CONTEXT）已经把『关闭后还要动作』引去关联新案件」：VE CONTEXT 没有这句原文。
   - `adapters/customscompliance/derive_on_external_result.go` 引「不得按最后到达直接改变当前判断」：这句出自 CC CONTEXT，写在 VE 包里不加限定。
   - `application/derive_customer_view.go` 写「`UC-VE-003` 的对客可见判断（`AT-VE-158`）」：`AT-VE-158` 在 UC-VE-008。
3. **Covers 仍点迁走的号**（VE-19）：UC-TF-005 `AT-TF-157`–`161` 里全程追踪自己的行为已迁入 UC-VE-002 `AT-VE-189`–`192` 与 UC-VE-004 `AT-VE-193`–`195`（逐条对应见 [docs-consistency/04](../issues/04-ve-docs.md) 完成记录）。`internal/visibilityexception/adapters/transportfulfillment/derive_on_movement_fact_test.go`、`internal/visibilityexception/application/raise_departure_gate_signal_test.go`、`internal/visibilityexception/adapters/postgres/departure_gate_signal_rule_test.go`、`cmd/parcel-dispatch/movement_fact_projection_test.go` 的 Covers 仍点 `AT-TF-157`–`161`。

## 证据

- AGENTS.md「改文档」：「引另一个文件的某句或某段 → 优先用符号名，无符号可指时用引文，一律**不用行号，也不用计数**」，并明说「**Go 注释里的跨文件引用同受此约束**」。
- VE CONTEXT「证据、客户索赔与追偿」：「收到客户索赔、通过资格审核和确认赔偿责任是不同判断」；「案件身份、范围与隔离」「案件责任、响应与闭环」两节的重开与关联新案件各条。

## 三类

机制（注释）。无产品策略、无租户取值。

## 规模

S。

## 最小的诚实修法

1. 计数改成符号名或删去：例如「`routes_*.go` 里共用 `projectionDerive` 的各投影路」「本包在装配期固定租户的各视图」「新的源接上来时」。
2. 引文改成逐字原文，或去掉引号改为转述并指明出处；CC 那句写成「CC CONTEXT」；`AT-VE-158` 前标「UC-VE-008」。
3. Covers 改点新号，`AT-TF` 号随 UC-TF-005 那几行改为引用后一并去掉（UC-TF-005 那一半由 [docs-consistency/10](../issues/10-cross-cutting-docs.md) 改）；AT 盘点 ① 里那几行随之移行。

## 要裁的

无。
