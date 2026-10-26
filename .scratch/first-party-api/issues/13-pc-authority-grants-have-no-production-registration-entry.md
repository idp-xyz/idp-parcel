# 13 PC 授权册的授予没有生产登记入口，资料修订口在任何租户上都答未配置

Category: enhancement
Status: in-progress——2026-10-26 会话 `tops-20`（Claude Code）立并认领，分支 `claude-fpa13`。R1（上线前必修：用户 2026-10-26 定首发要资料修订口，见 [spec](../spec.md)「五口上线」）。
Blocked by: 无
出处：票 [10](./10-customer-source-data-amendment-face.md) 作者读码发现：`AuthorityGrantStore.SaveGrant` 非测试调用方为零（`ports.AuthorityGrantRegistered` 注释自承「今天没有任何生产编排发这一格」）；发布批 `AUTHORIZATION_RULE` 只带 `cancellationAuthority` 声明，`DeclarationChannel` 里没有授予通道；于是种子登不进任何授予，PC 对每次资料修订都答 `ErrAuthorityRulesNotConfigured`。

## 做什么

- PC 侧给授权册的授予一个生产登记入口：照该上下文既有的登记/发布写法选一条（发布批新增授予通道，或 `cmd/parcel-commercial` 子命令），带重放与内容冲突代数（照既有登记口），迁移若需要照规矩取号。
- 授予形状要能表达票 10 / ADR-0311 定的直客自助修订等级常量（法人、等级、范围逐字匹配）。
- 演示种子：给 `SYN-TENANT-01` 登修订授予（等级取 ADR-0311 的常量），并在发布批 `sourceDataAmendment` 允许矩阵补演示要走的意图格（补充、显式清空等，现只登了 `CORRECTION`）。
- 租户取值不写死：没登授予的租户如实答未配置。

## 完成判据

- [ ] 生产入口能登授予，重放与冲突代数有用例（含真库）。
- [ ] 演示种子 `seed.sh` 退 0，登进去之后 PC 对演示客户端的修订授权判断答允许（真库用例）。
- [ ] Go 门禁；全量用 `scripts/test-full.sh`；新 AT 与盘点表行。

## 规模

M
