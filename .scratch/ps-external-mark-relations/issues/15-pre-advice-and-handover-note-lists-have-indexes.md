# 15 送站预报与送站交接单的两条列表查询没有索引

Category: enhancement
Status: in-progress——2026-10-19 会话 `idp-parcel-9b` 立并代决、认领，分支 `claude-psxm15`，NO 迁移号 0021
Blocked by: 无
父票：[13](./13-admin-web-reads-pre-advices-handover-notes-and-scope-state.md)
出处：票 13 完成记录「没做」与评审。
地盘：
- migrations/node_operations/（0021）
- internal/nodeoperations/adapters/postgres/ 两条列表查询所在文件的测试（只在要钉住查询计划时加）
- docs/product/MECHANISM-INVENTORY.md（重生成）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；2026-10-19 用户答「yes」。

## 现象（读码）

票 13 的预报列表（按节点与预计送达区间、新者在前）与交接单列表（按节点与开立区间）在本租户全表上筛与排序；票 08 的迁移 0013 只为写口与按标识点读建了键，没有为这两条读建索引。演示量下无感，真实租户的预报与交接单逐日累积后，复核页每开一次就是一次全表扫描。

## 裁定（2026-10-19，会话 idp-parcel-9b 代决；机制）

照两条查询的实际谓词与排序各建一条复合索引：（租户，节点，排序时刻）——预报取预计送达时刻、交接单取开立时刻，排序方向与查询一致；节点筛选缺席时的全租户列也能用同一条索引的前缀加排序。不改查询的答复形状与语义。迁移照本仓执行器的约束放在事务里（不用 `CONCURRENTLY`，表今天还小；`runner.go` 头注写明了这一条显式未决）。

## 完成判据

- [ ] 迁移 0021 两条索引，头注写依据（对应哪条查询的谓词与排序）。
- [ ] 用例：在一次性库上灌够数量的合成预报与交接单，`EXPLAIN` 两条列表查询走到新索引（不走 Seq Scan）；既有列表用例照过。
- [ ] Go 门禁；全量用 `scripts/test-full.sh`；机制清点重生成。
- [ ] 走 Go 那条路（[并行会话](../../../docs/agents/parallel-sessions.md)）。
