# 12 `PN09-W06`：待认领到货（登记、认领与撤销、处置规则、处置、收货引用与查阅）

Category: enhancement
Status: in-progress（Standards 两处阻断已修；Spec 轴当轮没有交卷，尚未进 main）
Blocked by: 02、09、11

依据：[UC-WF-005](../../../docs/application/warehouse-fulfillment/UC-WF-005-REGISTER-CLAIM-AND-DISPOSE-OF-UNCLAIMED-ARRIVALS.md)；[UC-WF-004](../../../docs/application/warehouse-fulfillment/UC-WF-004-CONFIRM-RECEIPT-AND-CLOSE-OUT-AN-INBOUND-PRE-ADVICE.md)（引用待认领到货的收货确认）；[仓配履约 CONTEXT](../../../docs/domain/warehouse-fulfillment/CONTEXT.md)「入库与加工」「待认领到货」；[ADR-0384](../../../docs/adr/0384-warehouse-access-words-customer-faces-and-the-switched-off-answer.md) 决定二、三、四、五、八。

## 做了什么

- **接入身份与迁移**：运营决定增 `UNCLAIMED_ARRIVAL_CLAIM`、`UNCLAIMED_ARRIVAL_DISPOSITION`（`migrations/access_identity/0063_unclaimed_arrival_decisions.sql`），准入两格同取字面量；作业事实类型增 `UNCLAIMED_ARRIVAL`（能力词 `WORK_FACT_REGISTRATION`，作业范围 `WAREHOUSE`）。
- **数据结构**：`migrations/warehouse_fulfillment/0016_unclaimed_arrival_lifecycle.sql` 扩展 `unclaimed_arrival`（支持作业事实直接登记 `WORK_FACT` 与收货转来 `DIVERTED_RECEIPT`），给 `receipt_confirmation` 增可选 `unclaimed_arrival_id`，新建认领、认领撤销、处置及各仓处置规则修订两张表。
- **直接登记**：`POST /warehouse-fulfillment-unclaimed-arrival-registrations`（先认证再读载荷），支持 `INBOUND` / `RETURN`、按事实身份判重与冲突、停用自营仓照记、代理仓答 `WAREHOUSE_NOT_ACCEPTING/AGENT`、无仓答 `WAREHOUSE_NOT_FOUND`。线索不触发货主预报读口的 `unclaimedArrival`。
- **认领与撤销**：`POST /warehouse-fulfillment-unclaimed-arrival-claims`、`claim-revocations`，各自按决定标识判重，同一到货串行；认领核 `party-commercial` 货主存在（查不到答 404），入库到货核同货主同仓可收货预报，退件禁带预报；已有确认收货引用的撤销答 `REVOCATION_NOT_ALLOWED`。
- **收货引用校验**：收货确认可选带 `unclaimedArrivalId`（锁序：预报 → 到货），引用的到货须在 `CLAIMED` 且指向该预报；预报已取消/完结且引用到货时答 `CONTENT_INVALID`，不另建新到货。
- **处置规则与处置**：规则修订走登记册配置写 `POST /warehouse-fulfillment-unclaimed-disposal-rule-registrations`（不判准入）与查阅 `rules`、`rule-revisions`；处置走 `POST /warehouse-fulfillment-unclaimed-arrival-dispositions`，无规则答 `NOT_CONFIGURED`、不借用别仓，认领期限按到货发生时刻经过秒数计（不依赖仓业务时区），期限未到或方式不在规则答 `DISPOSITION_NOT_ALLOWED`，同方式重放答 `EXISTING_RESULT`，换方式答 `STATUS_MISMATCH` 并回显原处置。
- **运营查阅**：`GET /warehouse-fulfillment-unclaimed-arrivals`（支持仓、种类、状态、登记时间范围筛选与分页）与 `GET /warehouse-fulfillment-unclaimed-arrival`。客户面不开放。

## 不在本票

- 退货入库对已认领退件的引用（归 `PN09-W18` 退货包）。
- 更正已登记的到货观察（`BD-WF-013`）、向货主公开待认领到货（`BD-WF-012`）。
- 处置的实物执行与收费发生项（归 `PN09-W20`）。

## 验证

证据层级 `S`，含真库 PostgreSQL 16。主体 `de2ce983d`：`gofmt -l .`、`go vet ./...`、`internal/architecture`、`internal/accessidentity/...`（`-race`）、`internal/warehousefulfillment/...`（`-race`）、`cmd/parcel-api/` 整包（`-race`）全部通过。修订 `02cdeba41` 重跑：`gofmt` 洁净，`go vet` 通过，`internal/architecture` 通过；领域两则与 `TestUnclaimedArrivalShapeAndClaimWindowRejectDirectInserts`、`TestUnclaimedDisposalRuleAndDispositionLifecycle` 连真库通过。

## 评审记录

钉 `de2ce983d`（相对 `b4914f9c2`）。Standards 阻断 2，修订在 `02cdeba41`：

- `unclaimed_arrival_origin_shape`：转出来源的事实标识为空时，`NULL = fact_id` 是 UNKNOWN，CHECK 放行，外键在 `MATCH SIMPLE` 下也不拦。转出来源补上 `diverted_receipt_fact_id IS NOT NULL`。直插缺这一格现被 `23514` 拒。
- 认领期限秒数乘 `time.Second` 在超过 `9223372036` 时回绕为负，期限未到会被判成已到。登记门与库上 CHECK 拒掉超出的值，处置改为按整秒比较。

交付模式只评一轮、只修挡上线的；探针由作者在 `02cdeba41` 重跑，结果见上。Spec 轴当轮没有交卷。
