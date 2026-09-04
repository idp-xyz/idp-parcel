# 05 批务：T2 量尺重核与页面登记集成

Category: chore
Status: resolved——三项销清：第 2 项 `e66eb8c`、第 1 项 `t2-remeasure-1665fdb.md`（MCP-3）、第 3 项见文末 2026-09-04 条（MCP-1）

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
- **根因跟进（已落，9132594）**：Windows 上任何把 .sql 重写成 CRLF 的工具都会凭空造出
  「校验和漂移」，且 build/vet 全无信号，只在真库推进时炸。两路方案里取守卫测试
  （`migrations/eol_guard_test.go`：嵌入迁移资产含 `\r` 即红，另带「一个 .sql 都没走到」
  的空转自检），弃 `checksumOf` 归一（全部既有校验和作废，代价不对等）。守卫经证伪：
  临时 CRLF 化一份 SQL 立即 FAIL 报文件与偏移，还原后绿。

## 进展

- 2026-08-31（MCP-1）**第 2 项销项**：01–04 交付的五张页面（`group-legal-entities`、
  `business-parties`、`channel-product-catalog`、`customs-ports-paths`、`cod-ledger`）已全部
  登进 `liveIds` 与导航，最后一张随 `e66eb8c` 补上。`cod-ledger` 是此前唯一缺口：页面自
  `d815c5e` 起就在发 `GET /collection-subledgers`、端点自 `b456678` 起在装配表上，只差登记
  这一行——`b456678` 标题写着「页登半边」，实际只改了 `navigation.ts` 的出处引用。缺口是
  用户可见的：`Workbench` 的档位全由 `readinessOf` 派生自 `liveIds`，于是该页显示成「页面
  骨架」、「代收与清分」分区显示「已接线 0/1」，而页面真在发请求——正是「未接线看着像已
  接线」的反面一格。同笔核过反向漂移（登了 live 却不发请求者为零）：前端调用的路径逐条
  对得上 `cmd/parcel-api/endpoints.go` 的装配表。补后档位 23 已接线 / 14 骨架 / 1 演示 /
  0 规划占位，锚 `e66eb8c`。
  验证于临时 worktree 检出 `e66eb8c` 提交态：`go build`、`go vet`、`go test -count=1 ./...`
  全绿且**含真库**（同刻单跑门禁用例得 `PASS` 非 `SKIP`），`apps/admin-web` `pnpm build` 绿
  2704 模块。这一跑同时可充第 3 项「全仓绿（真库口径注明）」的取证；第 3 项余下的基线接线态
  同步与本批 spec 收口未做，第 1 项 T2 量尺重核亦未做，故本票仍 in-progress。
  `e66eb8c:main` 已推，同链带出 22 笔他会话未推提交，均在上述验证覆盖内。
- 2026-09-01（MCP-3）**第 1 项销项**：T2 量尺按 `1665fdb` 重核完毕，只读取证、代码零改动。
  产物 [t2-remeasure-1665fdb.md](../t2-remeasure-1665fdb.md)，取数命令固化在
  [t2-census.ps1](../t2-census.ps1)，三个锚点原始输出留在 `t2-census-out-<sha>.txt`。
  棘轮普查：四族零调用点 33/35/13/20 与上一轮 `7d68475` **逐名一致**（专为「同数掩盖换名」
  跑的对差为空），动静全在分母——二族 +3、三族 +4、四族断言文件 +20。
  outbox 消费清单：装配面结论已回写
  [消费清单](../../outbox-handoff-consumption-map/report.md) 与
  [被取代的旧清点](../../handoff-consumer-inventory/report.md) 各自新增的 as-of 节，
  两处主表判据栏与他人段落均未动。
  三件后续留给票主定，本票未开：①上一轮「四族断言文件 123」是问法漂了一格（含 `_test.go`），
  真值 118，但不外溢到 84/20；②二族新名 `NewHandler` 不带上下文词根，门禁按名字建索引会在
  第二个同名出现时静默判错；③四族跨包重名由基线的 3 涨到 6，基线「不存在第四个待发现」
  已按其自身口径过期。
  第 3 项「基线接线态同步 + 本批 spec 收口」不在本轮范围，本票维持 in-progress。
- 2026-09-04（MCP-1）**第 3 项销项，本票转 resolved**。
  - **全仓绿（真库口径）**：钉 `08e62ec`（今日已推 origin/main）在干净检出、DSN 55432 上跑
    `gofmt -l .` 空、`go build ./...` 与 `go vet ./...` 退 0、`go test -p 1 -count=1 ./...` 退 0 零 `FAIL`
    （含真库）；`apps/admin-web` 的 `tsc --noEmit` 退 0（`pnpm build` 在本机因 PostCSS 插件缺失不可用，
    见 workflow.md 本机环境，非源码问题）。机制清点在同一检出上重生成与提交版一致。
  - **基线接线态同步**：开发主线「机制半边现状」节末补一段 2026-09-04 补记——`collection-remittance`
    首次出现生产代码这一定级触发条件成立、其归入哪个 PN 切片留待受托裁断（本票不替产品定级）；
    `internal/accessidentity` 如实记为无 `CONTEXT.md` 的机制代码；本批另三线不改任何 PN 行的定级；
    接线态权威仍是 `liveIds`，基线不另设登记也不抄档位数。
  - **本批 spec 收口**：五票全 resolved，spec 状态行同笔改。
  - 上一条 MCP-3 留给票主定的三件后续（四族断言文件真值 118、二族 `NewHandler` 无上下文词根、
    四族跨包重名 3→6）**不在本票收**：它们是 T2 量尺自身的口径问题，随下一次量尺重核处置，
    记在 [t2-remeasure-1665fdb.md](../t2-remeasure-1665fdb.md) 即够，本批不为它们开新票。
