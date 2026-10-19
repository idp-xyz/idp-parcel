# 02 第一期：收寄记录保全本次交付的来源文档，三种接收观察都存；每版判断有版本、追加版指名前版，已有结果带回原判断种类与当前有效版本

Category: enhancement
Status: ready-for-agent——2026-10-19 会话 `idp-parcel-9b` 的作者子代理照票 01 拆出（代决）
Blocked by: 01 进 main（ADR-0199、NO CONTEXT 与 UC-NO-002 先落）。同改收寄编排与 PG 适配器的 operator-channel/45、no-reception-identity/03 已于 main `91ea0883` 进 main，以它们为底。
父票：[01](./01-node-reception-preserves-the-delivery-source.md)
出处：票 01 裁定分期之「一、来源保全」与「已有结果的答复带回原判断种类与当前有效版本」那一问；ADR-0199 决定一、决定二。
地盘：
- internal/nodeoperations/domain/payload_canonicalization.go（来源文档的形状）及其测试
- internal/nodeoperations/ports/ports.go（`ReceptionRecord` 加来源文档、判断版本、前版）
- internal/nodeoperations/application/receive_delivered_unit.go 及其测试
- internal/nodeoperations/adapters/postgres/reception.go 及其测试
- migrations/node_operations/（下一个空号；今天是 0007）
- internal/nodeoperations/adapters/http/receive_delivered_unit.go 及其测试（已有结果两格信息）
- cmd/parcel-frontline-import/intake_import.go（明细写原判断种类与当前有效版本）
- cmd/parcel-api 下新起一份真库用例文件
- docs/product/MECHANISM-INVENTORY.md（重生成）
- .scratch/at-coverage-inventory.md

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 现象（钉 main `91ea0883`，读码）

- `ports.ReceptionRecord`、`domain.NodeIntake` 与 PG 行模型都没有号码格；`Handle` 把 `domain.CanonicalizeDeliveryPayload` 交回的规范化文档写成 `_`，只留摘要。
- 拒收只存拒收原因；仅扫描只存键、摘要、种类与时刻。证据、交付方只在形成与待识别两格的 intake 里有。
- 已有结果的 HTTP 答复只有 `outcome` 与记录上本来就有的几格；仅扫描一支重投时只有 `outcome`。
- 未收寄与待确认两格没有版本；ADR-0196 追加的冲突版不指名它接续的是哪一版。

## 做什么

照 ADR-0199 决定一、决定二：

1. **来源文档**。domain 定收寄面来源文档的形状：自带形状版本；作业实物、外部标识观察（原样）、接收观察、证据、拒收原因、交付方、节点、发生时间（UTC）；缺席格记空串、不省键。编排三支（明确接收、明确拒收、仅扫描）都把它写进记录。指纹照旧按 NOC-1 收寄面那几格算，`CanonicalizeDeliveryPayload` 与 `unversionedDeliveryContentDigest` 一个字节都不改。
2. **ADR-0196 冲突版**带引起它的那次后到送站的来源文档（与它沿用的摘要同源）。
3. **判断版本与前版**。未收寄、待确认两支也签判断版本（同一签发器）；形成与待识别两格的判断版本就是收寄结果版本，不另签。ADR-0196 冲突版指名前版：先到那件被依的那一行所在链此刻的当前有效版本。
4. **存储**。迁移加来源文档列、判断版本列、前版列；库面以（租户，前版）唯一守住「一版至多一个后继」。既有行不回填来源文档（号码从未留存）；新行必带来源文档与判断版本的守法由实施定（例如按约束只拦新写入），但不得给既有行补造来源。迁移头注写明既有行为空的理由。
5. **已有结果的答复**带 `recordedOutcome`（原判断种类）与 `currentVersion`（沿前版链走到头的那一版），HTTP 答复与前线导入明细都带；`outcome` 仍是 `EXISTING_RESULT`，两格不进 outcome 词表。
6. **AT 点名**：`AT-NO-047`、`AT-NO-048`；`.scratch/at-coverage-inventory.md` 照它的扫描口径更正。

## 不做

- 重核（票 03）、同一作业实物的并入与仅扫描待确认的续办（票 04）、身份核对读不动的翻转与身份核对态（票 05）。
- 把证据、交付方、拒收原因并进指纹（ADR-0199 越权风险点）。
- 来源冲突那一次的内容落册（ADR-0199 能力边界）。
- 复核目录与管理台展示来源文档（票 03 为人工重核要用时再透出外部标识观察）。

## 完成判据

- [ ] 编排用例：明确接收、明确拒收、仅扫描各一条，记录带来源文档，外部标识观察逐字等于入参（带两端空白的号在线口原样存）；同一来源重投答已有结果，首存的来源文档不被覆盖——同一来源只在证据上不同也如此（`AT-NO-047`）。
- [ ] 编排用例：ADR-0196 一码两物之后，重投先到那件原判断的来源，答已有结果，`recordedOutcome` 是 `INTAKE_FORMED`、`currentVersion` 是追加的冲突版；冲突版的前版是原判断（`AT-NO-048`）。仅扫描一支重投时 `recordedOutcome` 是 `RECEPTION_UNDECIDED`。
- [ ] 真库用例（cmd/parcel-api，经生产装配）：三种观察落册后读回来源文档逐字一致；来源文档为空的既有形状行照常重放，不答冲突、不报错。
- [ ] 指纹不变：NOC-1 与无版本两版既有摘要的重放与冲突用例照旧绿。
- [ ] 适配器用例：（租户，前版）重复写入被库面拦下。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/nodeoperations/...`、`cmd/parcel-api`、`cmd/parcel-frontline-import`、`internal/architecture/...`、`migrations/...` 全绿；机制清点在本笔的干净检出上重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。
