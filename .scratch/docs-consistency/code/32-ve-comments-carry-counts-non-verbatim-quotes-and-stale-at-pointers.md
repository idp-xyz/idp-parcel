# 32 VE 代码注释里有会腐的计数、不是原文的「」引文与错指，测试 Covers 仍点已迁走的 `AT-TF` 号

Category: chore
Status: in-progress——完工待评审与进 main，分支 claude-cmtA
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

## 完成记录（分支 claude-cmtA）

只改注释与测试说明文字，不改任何可执行语句、标识符、字符串字面量。`git diff -U0` 里 Go 文件的非注释行变动为零；`gofmt -l` 无输出；`go build ./...`、`go vet` 通过；`go run ./tools/at-coverage-counts -check` 退 0；`scripts/test-full.sh ./internal/transportfulfillment/... ./internal/customscompliance/... ./internal/visibilityexception/... ./internal/architecture/...` 全绿（三张票同在分支 claude-cmtA，一次跑完，38 个包 ok，无 FAIL）。

改了 `internal/visibilityexception` 与 `cmd/parcel-dispatch` 共 24 个 Go 文件、38 处，另改 AT 盘点 12 行，按类：

1. **会腐的计数**（票面五处，仍在原位）：`adapters/parcelshipment/derive_on_final_outcome.go` 包注释「四路投影」改「各投影路」；`cmd/parcel-dispatch/assemble.go` 的 `newTenantBoundProjectionDerive` 头注改为「`routes_*.go` 里经 `sources.projectionDerive` 取用的各投影路」，不再列路数与路名；`adapters/postgres/notification_policy.go`「另外三个视图」改「其余在装配期固定租户的视图」；`derive_on_transport_handover.go`、`derive_on_effective_delivery.go` 的「第三个源」改「新的源」。同类的「三本册子」三处（`adapters/http/query_claims_recovery_records.go` 两处、`_test.go` 一处）去掉计数。
2. **不是原文的「」引文与错指**：
   - `query_claims_recovery_records.go`：「三判分步」改引「证据、客户索赔与追偿」一节「收到客户索赔、通过资格审核和确认赔偿责任是不同判断」；「赔付金额的计算」（VE CONTEXT 里没有）改引同节「`UC-SA-007` 唯一形成赔付和追偿金额」。
   - `application/send_disposition_request.go`：改引「原因、责任与处置协调」一节「案件范围缩小、改派、归并或关闭前必须盘点全部未完成处置请求」与「案件责任、响应与闭环」一节「独立的新原因、新责任范围或新的异常事件建立关联新案件」。
   - `adapters/customscompliance/derive_on_external_result.go`：加限定「CC CONTEXT「外部结果与规则时效」一节」，引文改为 CC 原句「不得据此猜测提交、补造缺失层次或按最后到达直接改变当前判断」。
   - `application/derive_customer_view.go`：`AT-VE-158` 前标 UC-VE-008，`AT-VE-187` 前标 UC-VE-003。
3. **Covers 迁号**：`derive_on_movement_fact_test.go`（五处）、`raise_departure_gate_signal_test.go`（两处）、`departure_gate_signal_rule_test.go`、`cmd/parcel-dispatch/movement_fact_projection_test.go`（三处）的 `AT-TF-157`–`161` 全部改点 `AT-VE-189`–`195`，对应关系按 docs-consistency/04 完成记录：157 → 189、158 → 193、159 → 190、160 → 191 与 194、161 → 192 与 195。仓内 Go 文件已无 `AT-TF-157`–`161` 字样。AT 盘点：VE 的 `AT-VE-189`–`195` 七行去掉「Covers 现点 UC-TF-005……改点名归 code/32」的注记；`AT-TF-157`–`161` 五行保留在 ① 并加注「UC-TF-005 此行只作引用，场景以……为准，上列测试的 Covers 已改点该号」，不挪桶、不动计数。
4. **同类残留（票未点名）**：按「CONTEXT…「…」」逐一对 VE CONTEXT 核逐字，不是原文的改成原文：`domain/fact_supersession.go`、`fact_supersession_test.go`、`customer_view_test.go`（两处）、`exception_case_test.go`、`tracking_projection_test.go`、`application/derive_customer_view_test.go`（「客户服务终局只能来自 `parcel-shipment`」在 CONTEXT 里没有，改引「客户可见性与通知」一节「`parcel-shipment` 提供的客户服务终局」）、`form_eta_test.go`（「预测必须保存……」改引「ETA 与可见性缺口」一节「每次 ETA 必须明确预测对象……」）。位置词「上一条」三处改指测试函数名（`claim_recovery_test.go` 两处、`derive_on_initial_route_test.go` 一处）；变更叙述「如今」「接通」「此前恒有一维核不了」四处改成现状陈述（`handle_claim_test.go` 三处、`claim_recovery_test.go` 一处）。

### 已漂移或已不存在的位置

票上列的位置全部仍在，没有已被后来的票顺手改掉的。

### 代决

- `AT-TF-157`–`161` 在盘点里没有挪桶：这五行是 UC-TF-005 的引用行，没有自己的测试点名，也不是「机制未落地」；是否该改桶或删行归 docs-consistency/10（UC-TF-005 那一半）定，本票只让行文不再说「Covers 仍点它们」。
- `claim_service_rules.go` 引 PC CONTEXT 的「委托被接受时固定其适用的…客户合同版本」带 `…`、各段在 PC CONTEXT 里逐字存在，保留。
- VE 的「第二个返回值」类序数是同一函数签名内的说法，不是跨文件引用，没动。

### 越权风险点

- `AT-TF-157`–`161` 五行在盘点 ① 里仍记着测试，而测试已不点它们；若协调方认为引用行不该留在 ①，需要另做一次分桶判断，会改动总览计数（本票未动计数）。
