# 10 授权交付关系：参与方关系多一种「授权交付方」角色，PC 答授权交付判断，NO 侧立防腐适配器

Category: enhancement
Status: ready-for-agent——2026-10-19 票 07 的作者子代理照 ADR-0212 拆出。代决：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」，2026-10-19 答「需要」
Blocked by: 无（ADR-0212 已接受，PC CONTEXT「参与方关系」已随票 07 补上授权交付关系一句）
父票：[07](./07-verifiable-delivery-scope-for-customer-numbers.md)
出处：[ADR-0212](../../../docs/adr/0212-a-customer-number-hit-needs-a-verifiable-delivery-scope.md) 决定一、三、九；PC CONTEXT「参与方关系」；CONTEXT-MAP `party-commercial → node-operations`；参数登记册 `PAR-NET-17`。
地盘：
- `internal/partycommercial/domain/party_relationship.go`（`PartyRole` 多一种；授权交付的适用范围怎样写）及测试
- `internal/partycommercial/ports`、`adapters/postgres`：授权交付判断读口（新文件）及测试
- 既有参与方关系的登记写面与受控批量口：只需认新角色，先 grep 既有登记口与 CLI 再动
- `migrations/party_commercial/`：只在角色或范围落库带 CHECK 时才要，取当时下一个空号
- `internal/nodeoperations/ports`（授权交付判断端口，新文件）、`internal/nodeoperations/adapters/partycommercial`（新防腐适配器）及测试；装配在票 12
- `scripts/demo-seeds/data/commercial`：演示租户一个合成交付方参与方、一条授权交付关系
- `docs/product/MECHANISM-INVENTORY.md`（重生成）

## 做什么

1. **角色**。`PartyRole` 多「授权交付方」：持有方是交付方参与方，相对方是客户账户所属的参与方；适用范围收到客户账户，可再收到节点。范围怎样写进 `CommercialScopeReference` 由本票定，写进完成记录；若要改 PC CONTEXT 的措辞，先改 CONTEXT。生命周期照参与方关系：候选、生效、到期、撤销、替代。
2. **授权交付判断读口**：输入（租户，交付方参与方，客户账户，时刻），答复是、否、未配置之一，读不动交回错误。
   - **是**：交付方就是该客户账户所属参与方本身；或该时刻有一条生效的授权交付方关系，适用范围覆盖这个客户账户（有节点范围的，也覆盖开立节点）。
   - **未配置**：本租户一条授权交付方关系都没登记过，交付方又不是该客户账户所属参与方本身。
   - **否**：其余情形。
3. **NO 防腐适配器**。把 PC 的答复译成 NO 端口值，不把参与方关系的形状带进 NO。装配在票 12 接，本票只立适配器与它的用例。
4. **演示**。种子登一个合成交付方参与方，再登一条它替 `SYN-ACCOUNT-01` 送站的授权交付关系，证据记 `S`。

## 不做

- 送站交接确认的运营决定口与范围成立（票 12）。
- 按设备上报的交付方推定授权（ADR-0212 决定一）。
- 「操作者代客提交」等别的授权（ADR-0139 决定三）。

## 完成判据

- [ ] 领域与真库用例：本人答是；生效关系覆盖答是；关系未生效、已到期、已撤销、范围不覆盖这个客户账户或节点，都答否；本租户空册答未配置；别的租户的关系不算。
- [ ] 登记写面与受控批量口能登记新角色，重放与冲突照既有参与方关系口的代数。
- [ ] NO 防腐适配器用例：每种答复（含读不动）各译一格。
- [ ] 演示种子 `seed.sh` 退 0。
- [ ] Go 门禁：`go build ./...`、`go vet ./...` 退 0，`gofmt -l .` 无输出；带 DSN 跑 `internal/partycommercial/...`、`internal/nodeoperations/...`、`cmd/parcel-commercial`、`cmd/parcel-api`、`migrations/...`、`internal/architecture/...` 全绿；机制清点重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）：分支、作者自验、非作者评审、推送方在干净检出上带 DSN 跑全量。

## 规模

S
