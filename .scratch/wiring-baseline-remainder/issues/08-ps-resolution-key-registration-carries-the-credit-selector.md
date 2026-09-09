# PS 登记面补信用二维：含 `CreditPolicyObject` 的登记行要能形成立得起来的键

Category: enhancement
Status: in-progress——2026-09-09 12:0x 通道 4 认领（task-6eb31b85，分支 `mcp4-wbr08`，基 main `74ef0da8`；PS 迁移号取 0020，0021 已预给 wbr/01）。此前 ready-for-agent——2026-09-09 通道 1 代裁立票（用户授权自决）：ADR-0127 Consequences 点名「PS 登记面欠一格」，二选一（像 `PriceRuleObject` 那样在
登记面拒 / 补两维）**取补两维**。理由：ADR-0127 决定四把 SA 账期分支的授信额度改为从闭包交出的信用依据取，「found=false = 闭包没采用信用政策——租户登记的解析键
没要求这一项，恢复动作是补解析键与正文」——若登记面拒绝 `CreditPolicyObject`，没有任何租户能把这一项登进解析键，ADR-0127 整条路就没有入口；拒是把
机制半边的缺口写成长期事实。PS 地盘（历史归通道 2）
Blocked by: 无（PC 侧 `CreditSelector`（等级 × 费用类型）已在 main，ADR-0127）

## 条目

`internal/parcelshipment/adapters/partycommercial/commercial_resolution_keys.go`：`ResolutionKeyRegistration.validate` 今天放行 `CreditPolicyObject`
而不承载信用二维；含它的登记行经 `FormResolutionKey` 形成的闭包键缺 `CreditSelector`，PC 侧闭包解析对「请求信用依据的键必带 `CreditSelector`」
答`输入未受理`（ADR-0127 决定二）——一个登记得进去、永远立不起来的键。

## 做法（照结算三维的先例，同一文件里已有全套形状）

1. **迁移**（`migrations/parcel_shipment/00NN_resolution_key_credit_selector.sql`，号按目录顺延）：登记表加两列 `credit_level` / `credit_charge_type`（可空），
   加一条 CHECK 镜像「含则必填、不含则必缺」：`required_bases` 含 `CREDIT_POLICY` ⇔ 两列都非空（与 `..._settlement_paired` 同形，命名同族）。
2. **登记面**：`ResolutionKeyRegistration` 加两字段（逐维列出，不收 `pcdomain.CreditSelector` 整个——同文件对结算三维的头注说明了为什么逐维：本登记面不得承载
   闭包该解出的东西；信用二维不含合同维，所以这里逐维只是与结算同形，不是被迫）；`validate` 加 `validateCredit(needsCredit bool)`，判据同 `validateSettlement`：
   含 `CreditPolicyObject` 则两维必填、不含则两维必缺；`AuthorityLevel` 走 `NewAuthorityLevel`（非空；开票时写成「走 PC 的 `Named` 反查（集外拒）」是写错了事实——
   PC `authority_grant.go` 里 `AuthorityLevel` 是 `struct{ requiredValue }`，全包无封闭集、无 `AuthorityLevelNamed`，商业权限等级是租户的版本化业务授权，PC 不预设它有哪几档；
   通道 1 于 2026-09-09 14:1x 核过并裁改此句）、费用类型走 `NewChargeTypeReference`（非空）。
3. **键**：`FormResolutionKey` 加 `creditSelectorFromRow`，两维全缺交零值（「本次不要信用依据」的正常形状），在场则折成 `pcdomain.CreditSelector`。
4. **`PriceRuleObject` 那一支不动**：它拒的理由（价格规则的选择维在本登记面无从表达）与本票无关；本票只把「需要额外维度」清单里的信用那一项从拒改为承载。
5. 测试：登记面单元（含则缺一维拒 / 不含则给了拒 / 齐则键上 `CreditSelector` 在场）；PG 真库（迁移过 CRLF/BOM 哨兵；CHECK 两向各一条）；闭包键往返
   （`FormResolutionKey` 出的键送 PC `ResolveCommercialClosure` 的替身或真实现，请求信用依据时不再答`输入未受理`）。
6. 基线：若 `production_wiring_baseline.txt` / 可达性基线因此变动，按头注纪律记数、钉 SHA。

## 完成判据

含 `CreditPolicyObject` 的登记行能登记且形成的闭包键带 `CreditSelector`；不含时两维必缺；库内 CHECK 与登记面同判据两道镜像；含 DSN 跑
`./internal/parcelshipment/...` 与 `./migrations/`，反向依赖含 `cmd/*` 的包带 DSN 跑；gofmt / vet 0。

## 边界

不动 PC；不动 SA；不动 ADR-0127 正文（本票是它 Consequences 点名的后续，完成记录回指即可，不 supersede）。
