# 六项零证据 PBC 的取证推进：先复核 PBC-02/03 建模阻塞，再按简报取证序走

Category: enhancement
Status: in-progress

来源：[票 01](./01-reeval-verdict-bento-gate-stays-blocked.md)「差什么」第 3 条，
2026-08-21 MCP-3 受用户委托裁断开票。

## 现状（票 01 取证）

`PBC-02/03/04/05/07/09` 零证据：`RunRepositoryContract` 全仓零调用（ADR-0031 的断言，
于 `b9f6cba` 复核仍成立）；`tests/bentocontract/` 只有 `candidate*`（PBC-01）与
`outbox_inbox_test.go`（PBC-06）。各适配器自己的回滚/事务模板测试是将来取证 PBC-03 的
原料，但没有任何一项已按候选登记为 PBC 证明。

## 步骤（按依赖序，第一步纯取证）

1. **复核 PBC-02/03 的建模阻塞现状**：简报记录的阻塞是聚合版本字段与端口签名，而
   ADR-0030/0031 已动过这一带——阻塞可能已消。产出：逐项「仍阻塞/已消」判定与证据，
   录本票 Comments。
2. 阻塞已消的项，把既有原料（各 `*FollowsTheTransactionalTemplate`、回滚测试等）按候选
   登记为对应 PBC 证明；缺的补。
3. 按简报「下一步」的取证序推进 `PBC-04/07`、`PBC-05`、`PBC-09`。
4. 九项对同一候选（`v0.1.0-rc.2` + checksum）全部通过后，按完成门禁登记 `B-06`——
   **那一步是闸门动作，另行报批，不在本票内**。

## 边界

- 每一步的证据对同一候选登记；实现存在不等于通过（ADR-0026），本票不宣称闸门状态。
- 第 1 步结论若是「仍阻塞」，后续步骤按票 01 口径继续等，本票以第 1 步产出收口，不硬推。

## 参照

票 01；ADR-0026、ADR-0030、ADR-0031、ADR-0017；`docs/design/` 持久化简报（闸门表与取证序）。

## Comments

- 2026-08-21 MCP-3：随票 01 收口裁定开票。
- 2026-08-21 MCP-4（领票，第 1 步取证完成）：**PBC-02/03 的建模阻塞已消**，逐项判定
  （对 main=`3756eeb` 实测；简报所记三处相扣缺口逐一对现行代码）：
  1. 「`domain.ShipmentRequest` 字段全未导出、外部包重建不出任意合法状态」→ **已消**。
     重建门已立：`domain/rehydration.go` 的 `RehydrateShipmentRequest`（ADR-0028/0030，均
     Accepted），今天放行`已提交`与`已接受`两状态，`已拒绝`/`已撤回`由
     `ErrRehydrationStateNotSupported` 显式挡住——ADR-0030「按快照表达力逐状态放行」正是此意。
     合同要跑的插入/加载/版本冲突/作用域隔离在可构造状态内够用。
  2. 「聚合无版本字段且 `Save` 不收预期版本，这一口在端口签名上」→ **已消，且「端口签名」
     这半个问题被 ADR-0031 的逐符号实测解散**。聚合现携 `revision int64`
     （`domain/shipment_request.go`，一次保存一次推进、状态转移不动它）；框架合同实测收的是
     `RepositoryContractScenario[K,A]` 的三个分开接口（`Loader`/`Inserter`/
     `VersionedUpdater.Update(…, expectedRevision)`），**不要求 `Save` 改名或改签名**——绑定处
     写一个把 `request.Revision()` 递给 expectedRevision 的场景壳即可。
  3. 「归档谱系的聚合另带 `EventBuffer` 而当前不带」→ **不再阻塞**。PS 域内 `EventBuffer`
     零出现，而框架合同形状（上述三接口）不含事件缓冲——那是归档谱系自己的形状，不是合同
     要求（ADR-0031 逐符号核过框架模块，非推断）。
  另两件当年缺、现在在的：`migrations/parcel_shipment/0002_shipment_request.sql` 在
  （ADR-0031 Consequences 写「`shipment_request` 表还不存在」的时点已过，该表及 0003-0006
  俱在）；`adapters/postgres/shipment_request.go` 的 `ShipmentRequests` 仓储在。
  **边界**：本判定只回答「阻塞已消」，不宣称任何 PBC 通过（ADR-0026：实现存在不等于通过）。
  步骤 2/3 就此解锁，MCP-4 续办。
