# 六项零证据 PBC 的取证推进：先复核 PBC-02/03 建模阻塞，再按简报取证序走

Category: enhancement
Status: needs-info

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

- 2026-08-24 MCP-1（步骤 2/3 取证落地 + 一项事后发现，两件分开记）：

  **一、取证证据 `6355d12`**（验证于该提交态的隔离 worktree：gofmt/build/vet 零信号，
  全仓 `go test -p 1 -count=1 ./...` 零 FAIL，DSN 已设、PG 各包实跑非跳过，255 秒）。
  六项就此不再零证据，证据全部落 `tests/bentocontract/`、对同一候选（`v0.1.0-rc.2`
  + checksum）登记：
  1. **PBC-02**：场景壳把真实 `adapters/postgres.ShipmentRequests` 装进框架
     `RunRepositoryContract` 三腿全过；壳只做翻译（ADR-0031 结果代数 ↔ 哨兵错误、
     expectedRevision 经重建门盖回聚合），端口签名未改——即步骤 1 复核所指的绑定方式。
  2. **PBC-03**：委托仓储与框架 Outbox Store 同一 Transactor 事务，提交同现、回滚同隐。
  3. **PBC-04（四面取三）**：同键同摘要重放答原结果且恰追加一条观察；同键异摘要接入
     冲突、原内容保留；四路并发同命令零第二份委托、竞态败者重试收敛原结果。
     第四面「不能创建第二个 EventID」当轮判为不可取证：`委托已提交`生产发射器在
     internal/ 零存在（`shipment-request.submitted` 于 `79e088d` 零命中）。
  4. **PBC-07**：ErrCommitUncertain 两个真实方向（提交实落库/实回滚）都按稳定请求
     关联（完整来源身份）收敛，零第二份委托；编排首步 FindPreserved 即那次查询。
  5. **PBC-05（形状面）**：基线信封过框架 v1 校验、Payload 键集恰四标识字段、同委托
     分区顺序、租约过期 At-Least-Once 重投同一份（jsonb 归一后与首领字节等、与原件
     语义等）。生产发射器缺席同上，如实记边界。
  6. **PBC-09（产出面）**：`proof.go` 按 rc.2 恰九字段严格产出（候选身份锚本包常量、
     RFC3339Nano、单 JSON 值、坏注入拒绝）；校验权威留框架侧不镜像。专用合同命令未建，
     等真实协调运行需要可执行工件时再立并按简报开 PBC-08 精确路径豁免。
  边界重申：以上为证据登记，不宣称任何 PBC「通过」与闸门状态（ADR-0026）；简报闸门表
  的证据行不在本轮占号内，未动。

  **二、事后发现（转 needs-info 的原因）**：证据落库后清点工作树，发现未合入的平行
  完成——分支 `mcp4-bento-pbc02` @ `1f7462b`（2026-08-21 23:47），提交信自述六项全部
  取证、票 03 收口，且含本轮判为缺席的机制半边：「委托已提交」意图端口与 Outbox 适配
  器（internal/ 生产代码）、按简报两段式的事务编排、简报信封基线定版与闸门表证据格
  更新。该现场于 2026-08-24 16:5x 量得：工作树干净、目录 mtime 停在 2026-08-21
  22:28——按并行会话纪律是全提交的死现场，分支指针即凭据，无未提交物。其全绿验证
  基于 2026-08-21 的基线，未对现行 main 复验。本轮领票前提（「第 1 步已由 MCP-4 完成，
  待办步骤 2/3」）漏点了这棵树——「换人之后先清点」的又一实例。
  **重启条件（裁断轮/用户）**：裁 `1f7462b` 去留——整支采用（需对现行 main 重整+复验，
  与 `6355d12` 的重叠部分二选一）、只取发射器半边、或弃之另立发射器实现票。main 上
  已验的 `6355d12` 为现行证据，不构成回退理由。裁定落地前，PBC-04 第四面与 PBC-05
  发射器半边保持未决。
