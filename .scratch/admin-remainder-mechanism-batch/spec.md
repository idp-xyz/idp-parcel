# 管理台余量机制批（产品就绪宣布后的第一批机制线）

Category: feature
Status: resolved——五票全 resolved（各票主责与提交见票面状态行；批务票 05 于 2026-09-04 由 MCP-1 收口）；批级判据的取证与基线补记见票 05 文末

宣布产品就绪（2026-08-28 受托认可轮，计划复核记录末条）后，用户指示「全面并行工作吧，我需要
全部完成」。本批把管理台余量里**不等租户就能完成的机制半边**全部立票推进；完成后管理台余量
收敛为「纯租户钥匙」一类。

## 范围裁定（受托，2026-08-28）

- **入批四线**：参与方身份生命周期（01）、产品—渠道映射目录（02）、口岸与申报路径目录（03）、
  代收分户账上下文从零（04）；另设批务线（05：T2 量尺重核 + 页面登记集成）。
- **COD 准入裁定**：跨境出口小包在中东/东南亚等目的市场以 COD 为常规收款形态，「承运/渠道商
  代收货款并周期回汇」是可指名的真实商业模式，按基线能力范围判据准入；形态结论已登记基线
  产品需求假设 `PA-CR-01`，实例值一律留空。
- **不入批（等租户，不可代劳）**：批 C 事务链页（面单交易、接受前人工复核、节点/运输查阅、
  关务案件数据、异常分诊与案件、索赔与追偿、费用/对账/核销/经营核算）压在 `PAR-INT-01`
  数据侧墙上；`PAR-NET-14` 规则正文。墙降后按既有裁定另立接线新票，不挂本批。
- **红线照旧**：只建机制；实例值留空拒默认；隔离合成只记 `S`（ADR-0078 `SYN-` 门禁）；
  中文注释；跨文件引用符号名不用行号；改中文源文件不用 Set-Content。

## 地盘与共享点

| 票 | 主责 | 地盘 | 共享点约束 |
|---|---|---|---|
| 01→02（同族串行） | PC | `internal/partycommercial/**`、`migrations/party_commercial/**`、商业登记 CLI | 迁移先看目录现有最大序号再占下一号 |
| 03 | CC | `internal/customscompliance/**`、`migrations/customs_compliance/**`、`cmd/parcel-customs-register/**` | 同上 |
| 04 | CR（新上下文） | `docs/domain/collection-remittance/**`、`internal/collectionremittance/**`、`migrations/collection_remittance/**`、新登记 CLI | 新迁移目录要接 `migrations/migrations.go` 与 `internal/platform/migrate/plan.go`；改 CONTEXT-MAP 那一行前在频道声明 |
| 05 | 批务（MCP-1） | `apps/admin-web/**`（`page-registry.tsx`、导航登记）、本目录 | **页面登记占号在 05**：各线交付页面组件文件并报注册条目，不自改 `page-registry.tsx` |

worktree、逐文件 add、不推（完工报已验 SHA 与验证强度给 MCP-1）照 `docs/agents/parallel-sessions.md`。
`cmd/parcel-dispatch/assemble.go` 本批无票碰它。`cmd/parcel-api` 装配三件（`endpoints.go`、
`main.go`、装配测试）随读面接线**占号在批务票 05**（MCP-1）：各线交付 `adapters/http` 处理器
与真库读适配器并报端点行，不自改装配三件——开批时那句「endpoints.go 谁也不碰」在此更正，
读面接线绕不开装配点，绕开的写法（页面直连库）才是要禁的。

## 完成判据（批级）

各票票面判据之外：全仓 `go build ./...` 与含真库 `go test -p 1 -count=1 ./...` 绿（报绿注明
含不含真库）、`apps/admin-web` 的 `tsc --noEmit` 提交态绿、新页 `SYN-` 种子非空册且实例格
显式未配置、基线接线态与本批票面同步收口。
