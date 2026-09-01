# 04 代收分户账：collection-remittance 上下文从零

Category: feature
Status: resolved——完成判据经 MCP-5 于 `d11e0f0` 逐项核实全达（核实记录见文末 Comment）；
后端主体（MCP-2）、读面/页面/种子（MCP-3）与合装页登（MCP-1）三方交付均已入库

## 准入依据

spec 范围裁定 + 基线 `PA-CR-01`（COD 代收回汇是目标客户群常规形态）。CONTEXT-MAP 今天只有
一行占位；无 `CONTEXT.md`、无 `internal/` 包、无迁移。

## 做什么

1. `docs/domain/collection-remittance/CONTEXT.md`：领域语言先行——代收指令（随委托指定 COD
   金额与币种）、代收事实（节点/承运回报的实收）、代收分户账（按客户/币种的受托保管账，
   代收款是客户的钱不是收入）、回汇批次（周期归集与支付）、差异事项（实收≠指令）。
   CONTEXT-MAP 该行同笔展开（改前在频道声明占号）；GLOSSARY 新词同笔。
2. `internal/collectionremittance`：域模型 + 分户账记账写入方（追加式、分配守恒）+
   `migrations/collection_remittance`（新迁移目录要接 `migrations/migrations.go` 与
   `internal/platform/migrate/plan.go`）。
3. 受控 CLI 登记/记账入口 + 读面（`SYN-` 准入）+「代收分户账」页组件；注册条目报 05。

## 边界

- 与 SA 分界：SA 管结算应收应付与经营核算；CR 管受托代收资金的保管、归集与回汇。
  两边不共享表、以引用交接。
- 不建支付通道集成（实例半边）；汇率、回汇周期、手续费一律不进代码。
- 上下文骨架取舍（分户账粒度、批次不可覆盖形状）难逆转 → ADR。

## 完成判据

`CONTEXT.md` 与 GLOSSARY/CONTEXT-MAP 一致；`SYN-` 种子灌入后代收分户账页非空册且回汇批次
实例格显式未配置；全仓绿（报绿注明含不含真库）。

## 后端主体裁量（MCP-2，08-28）

形状取舍已定并记 [ADR-0082](../../../docs/adr/0082-collection-subledger-is-keyed-by-four-dimensions-and-posted-append-only.md)。
票面之外另定的几件，记在这里供读面与种子对齐：

- **依据种类五值**（`COLLECTION_FACT` / `ALLOCATION` / `REMITTANCE_BATCH` / `DISCREPANCY` /
  `CORRECTION`）。`ALLOCATION` 是票面没点名的一格：清分（待清分 → 应付客户）的依据是代收
  指令而不是代收事实——一层来源事实说的是「有人报了这笔钱」，说不出「它是谁的」。
- **分户账键不是记账的输入**，由依据推出（事实→指令→键、指令→键、批次自带、差异→指令→键、
  原记账自带）。记账命令因此也不带币种。
- **入账无法冲正**，明认为代价：去向侧没有账外位置。来源侧记错走追加更正事实，那笔钱在账面
  上如何退出取决于真实渠道退款与银行退回形态，属实例半边，本切片不猜也不预留位置。
- **答案十格、退出码五档**：余额不足（`UNDERFUNDED`，退出码 4）与未决（3）分开——前者确定
  没落账、等实收或先清分后重跑即可，后者连落没落都不知道。

后端主体交付：`docs/domain/collection-remittance/CONTEXT.md`、
`migrations/collection_remittance/0001_collection_subledger_and_remittance.sql`（六表）、
`internal/collectionremittance/{domain,ports,application,adapters/postgres}`、
`cmd/parcel-collection-register`（七命令）。读面、页面与种子仍归 MCP-3；页登与装配仍占号 05。

## Comments

- 2026-09-01 · MCP-5：**收口。** 状态行停在 08-28 的在途口径，而三方交付此后都已入库；
  MCP-2/3/4 相继下线，无人翻状态。在 `d11e0f0` 上逐项核过完成判据：
  `docs/domain/collection-remittance/CONTEXT.md` 在、CONTEXT-MAP 有本上下文行、GLOSSARY 六个
  词条在（代收货款、代收指令、代收事实、代收分户账、回汇批次、差异事项）；迁移
  `0001_collection_subledger_and_remittance.sql`、`internal/collectionremittance/**` 四层、
  受控 CLI `cmd/parcel-collection-register` 在；读端口 `CodSubledgerCatalogueRead`、真库读
  适配器 `cod_subledger_catalogue.go`、HTTP 端点 `/collection-subledgers`、页面
  `pages/collection/CodLedgerPage.tsx` 与导航、`liveIds` 登记、`scripts/demo-seeds/data/collection/`
  十份 `SYN-` 种子全部在树上。全仓 `go build`/`go vet`/`go test -count=1 ./...`（含真库）绿。
  据此改 resolved。

  **一处过程记录，因为它值得下一个人知道**：我核 GLOSSARY 时先用 `git show … | Select-String`
  搜「代收」，得零命中，据此差点判「GLOSSARY 缺词、票不可收口」。零命中是假的——PowerShell
  管道把中文解成了乱码，搜索词匹配不上。这正是 `docs/agents/workflow.md` 本机环境节警告的那
  一格（「零命中恰恰就是自查想要的结果」），而我当天早些时候刚往那条补过一句。改用直读文件
  的方式重搜，六个词条都在。**核「有没有」这类判据时不要走 PowerShell 文本管道。**
