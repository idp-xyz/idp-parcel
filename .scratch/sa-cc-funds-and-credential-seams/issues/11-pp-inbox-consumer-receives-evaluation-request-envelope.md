# `parcel-pricing` 没有 inbox：SA 发出的评价请求信封到不了计价，UC-SA-002 步 2 后半「`parcel-pricing` 采用……形成不可变计价输入快照」只能由测试直接调

Category: enhancement
Status: draft——2026-09-10 17:5x 通道 5 立票（task-a93cb825；票 [08](08-sa-evaluation-request-orchestration-records-source-references.md)「要裁的」1 裁「走信封」时，因 PP 无 inbox 消费者先例而点名另立），只写票面未动代码；取证锚 `66cad4c4`。要裁的一条，归 PP owner
Blocked by: [08](08-sa-evaluation-request-orchestration-records-source-references.md) 的登记册与发信封半边（没有那封信封，消费者无物可收）——**08 已进 main（2026-09-11 16:3x，main 上代码 `c9aae85f` / `8d75bfc7`），信封 `settlement-accounting.evaluation-request.submitted` 载荷 `{tenantId, evaluationRequestId}`、分区主体 租户 / 评价请求、SA 读口 `EvaluationRequests`（按铸造 ID 取）在，硬阻已解；本票仍 draft，开工前要 PP owner 裁「要裁的」那一条入口形状（2026-09-11 通道 1 推送方记）**；本票落地后 08 的完成判据 4 与 [01](01-buy-evaluation-to-sa-inbox-consumer.md) 的「形成」路才走得通

## 缺口（取证于 `66cad4c4`）

- `Get-ChildItem internal/parcelpricing/adapters -Directory` → `http` / `postgres` / `sourcefeed`，**无 `inbox`**；`cmd/parcel-dispatch/assemble.go` 路由表无任何 `parcelpricing` 消费者。仓内 inbox 消费者先例在 `internal/networkrouting/adapters/inbox`、`internal/parcelshipment/adapters/inbox`、`internal/visibilityexception/adapters/inbox`。
- PP 提供方半边已在：`internal/parcelpricing/adapters/postgres/evaluation_handoff.go` 发 `parcel-pricing.evaluation.recorded`（票 01 缺口节）——PP 会**发**信封，但今天不**收**任何信封。
- 票 08 让 SA 在评价请求登记同事务发 `settlement-accounting.evaluation-request.submitted`（载荷 `{tenantId, evaluationRequestId}`）；PP 侧没有人接。

## 语言从哪里来

- UC-SA-002 步 2：「`settlement-accounting` / `parcel-pricing`：结算提交主要范围、计算目的和合格来源引用；`parcel-pricing` 采用测量、运输收费发生项、其他履约、面单及已解析商业依据形成不可变计价输入快照 → 计价输入版本或待判断」。
- PP `CONTEXT.md`（ADR-0012 决定）：`parcel-pricing`「只消费其他上下文提供的事实快照」；评价的输入是不可变计价输入快照，同一输入、版本清单与内容摘要产生相同结果（ADR-0013 Consequences 那条不变量）。
- 票 08「裁决」1：一条缝两个方向一种机制；同步调用会把 PP 的可用性压进 SA 的写路径。

## 做法

1. `internal/parcelpricing/adapters/inbox/evaluation_request_consumer.go`：收 `settlement-accounting.evaluation-request.submitted`，按 `evaluationRequestId` 经**消费侧适配器** `internal/parcelpricing/adapters/settlementaccounting/`（新；CONTEXT-MAP 加 SA→PP 一条边）读 SA 的评价请求只读口（按键取：租户 + 评价请求 ID；口径照票 [03](03-cc-inbox-consumer-receives-external-funds-fact.md)「裁决」——走只读口、取信封所指那一份、不读写侧登记面），取主要范围、计算目的、合格来源引用三件，交 PP 请求评价的应用入口（形状见「要裁的」1）。
2. PP 形成计价输入快照与评价时在评价上**回指 `evaluationRequestId`**（票 08 做法 3 靠它），随既有 `parcel-pricing.evaluation.recorded` 信封的评价内容可按引用读到。
3. 同请求重放 → PP 答`已存在`（同一请求不形成第二份评价；ADR-0013 不变量「同一评价输入……相同结果」的入口守法）；请求所指的来源引用解析不到（发生项 / 协议版本不在场）→ PP 答待判断并指名，不造输入。
4. `cmd/parcel-dispatch/assemble.go` 路由表加一行（共享接线文件，动前占号）；`application` 不得 import `settlementaccounting`。
5. 真库装配用例：入队一封 → 消费一次 → 一份计价输入快照 + 评价（或待判断）；重投不翻倍。

## 红线

- 消费者只译不算：金额、币种、换算、取整全在 PP 评价里按价卡声明（ADR-0107 / ADR-0013），消费者不预处理任何数值。
- 不改 SA 的信封形（归 08）；不改 PP 既有 `evaluation_handoff.go` 的信封形（票 01 消费它）。
- 真实价卡、真实供应商协议属实例半边 `PAR-SET-03`，夹具全是合成串。

## 完成判据

1. `git grep -w NewEvaluationRequestConsumer -- cmd/`（或等价装配符号）有非测试调用点。
2. 应用层：一封 → 一份评价（带 `evaluationRequestId` 回指）；重投 → `已存在`；来源引用解析不到 → 待判断。
3. 真库装配用例一正一反；CONTEXT-MAP 与机制清点（消费缝 PP→SA +1）同笔。

## 地盘

`internal/parcelpricing/adapters/inbox/`（新）、`internal/parcelpricing/adapters/settlementaccounting/`（新）、`internal/parcelpricing/application/`（若要裁的 1 裁「先立入口」则新编排文件）、`cmd/parcel-dispatch/assemble.go` 一行、`docs/domain/CONTEXT-MAP.md` 一条边。不动 `internal/settlementaccounting/**`。

## 要裁的

1. **PP 请求评价的应用入口是什么形**：PP 今天形成评价的入口是给谁调的（`git grep -n 'func New.*Evaluat' -- internal/parcelpricing/application` 开工前核）——若已有一个收「计价输入 + 计算目的」的应用入口，消费者直接调它并加 `evaluationRequestId` 回指一格；若今天评价只由 PP 内部或测试路径形成，本票先立「按评价请求形成评价」的应用入口。归 PP owner（入口形状与「评价回指请求」那一格是 PP 领域的事）。同时是票 08 裁决 1 的越权风险点：PP owner 是否愿意以消费者形式接。

## 参照

票 [08](08-sa-evaluation-request-orchestration-records-source-references.md)「裁决」1 / 2、[01](01-buy-evaluation-to-sa-inbox-consumer.md)、[03](03-cc-inbox-consumer-receives-external-funds-fact.md)「裁决」（消费侧读口口径）；UC-SA-002 步 2；ADR-0012、ADR-0013、ADR-0107；`internal/parcelshipment/adapters/inbox/effective_delivery_consumer.go`（跨上下文 inbox 消费者 + 消费侧读口先例）；`internal/parcelpricing/adapters/postgres/evaluation_handoff.go`（PP 提供方半边）。

## Comments

- 2026-09-10 · 通道 5（task-a93cb825）：立票，未动代码。能力边界：核过 PP `adapters` 三个子目录与 dispatch 路由无 PP 消费者、仓内三处 inbox 先例；读过 UC-SA-002 步 2、票 08 裁决；**没读** PP `application` 的评价入口签名——「要裁的」1 就是这一处，开工前以代码为准。
