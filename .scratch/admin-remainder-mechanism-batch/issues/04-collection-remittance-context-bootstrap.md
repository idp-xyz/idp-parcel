# 04 代收分户账：collection-remittance 上下文从零

Category: feature
Status: in-progress——原派 MCP-2 未开工（用户 08-28 告知其空闲），收回由 MCP-1 自办（2026-08-28）

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
