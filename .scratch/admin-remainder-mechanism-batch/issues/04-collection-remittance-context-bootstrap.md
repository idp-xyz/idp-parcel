# 04 代收分户账：collection-remittance 上下文从零

Category: feature
Status: in-progress——后端主体（MCP-2）已合装：cr04-backend 八笔（完工报已验 45aff42，含真库全绿）
逐笔对应落本仓 2ad600c..7cc5cfe，代码态 diff 为空故验证照片适用（MCP-1 复核 build/vet 零信号）；
CR 0001 迁移已施加演示库（97 步）。读面/页面/种子归 MCP-3 在途（worktree cr04-readface），
合装与页登仍占号 MCP-1（批务票 05）

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
