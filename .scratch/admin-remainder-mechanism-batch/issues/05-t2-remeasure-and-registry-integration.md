# 05 批务：T2 量尺重核与页面登记集成

Category: chore
Status: in-progress——MCP-1

## 做什么

1. **接线批开工前置（08-27 终盘记的欠账）**：按 T2 量尺重核——棘轮普查对新 HEAD 重跑、
   outbox 消费清单按新 HEAD 重取证；结论回写各自报告（保质期自注同笔更新）。
2. **页面登记占号**：01–04 交付的页面组件统一接入 `page-registry.tsx` 与导航登记，
   `tsc --noEmit` 提交态验证；`liveIds` 仍是接线态唯一权威，不另设第二处登记。
3. 批收口：全仓绿（真库口径注明）+ 基线接线态同步 + 本批 spec 收口。

## 批收口前的登记欠账

- ~~共享演示库 postgres@55432：`parcel_shipment/0008` 已施加记录的校验和与当前迁移工件
  不一致（0008 内容曾被 8dfe2e4 事后修改），演示库现状无法非破坏性推进迁移~~
  **已处置（2026-08-28，MCP-5 诊断 + MCP-1 执行）**，且原记成因是错的：0008 自 8dfe2e4
  新增后从未被改；真实成因是 `checksumOf` 对嵌入原始字节做 sha256、行尾进哈希，同一份
  SQL 的 CRLF/LF 两形算出两个值。演示库 0008 记录是 CRLF 形（08-27 CRLF 构建施加），
  0015 记录是 LF 形（08-28 LF 构建施加），工作树后来被整体 CRLF 化，两形各卡一条构成
  推进死锁。处置三步：工作树 0008/0015/0016 三份 .sql 强制重检出归一 LF（`git checkout`
  对「干净」文件不重写磁盘，须先删再检出）；演示库 `parcel_migration.applied_migration`
  里 0008 那行校验和 UPDATE 为规范 LF 值（两形逐字节同 DDL、无语义差，正当性与六实验
  隔离验证见 MCP-5 完工报）；迁移推进 `steps=96 applied=2`（customs 0013、party_commercial
  0016 落库），补灌口岸三笔、路径两笔、发布批重放（ECON 版本新发布）、渠道产品四项，
  全部 REGISTERED/REPLAYED。
- **根因跟进（未做，批收口前裁）**：Windows 上任何把 .sql 重写成 CRLF 的工具都会凭空造出
  「校验和漂移」，且 build/vet/test 全无信号，只在真库推进时炸。MCP-5 盘了两路：归一
  `checksumOf`（治本但全部既有校验和作废，代价大）或加守卫测试（嵌入迁移资产内含 `\r\n`
  即失败，挡在提交前）。倾向守卫测试；落点在 `internal/platform/migrate` 或 `migrations`
  包测试，MCP-2 释号 `migrations.go` 后再动，避免同包撞车。
