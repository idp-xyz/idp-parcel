# VE 目录登记无进程级入口，六个登记方法只有测试调用

Category: enhancement
Status: ready-for-agent

来源：[票 09](./09-ve-rule-and-policy-registries-have-no-writer.md) 的 B 半边。09 按 A/B 拆分
交付：A 半边（`catalog_registration.go` 写入方、`register_catalog.go` 登记用例六方法、ports
登记口、迁移 0019）随 `b394adf` 落 main，09 转 resolved；**B 半边在此票承载**——没有本票，
09 收口那一刻这一格就没人再看（[票 13](./13-production-ownership-bridge-has-no-assembly-point.md)
记录过同一失效形状，2026-08-21 MCP-3 受用户委托裁定按「02→13」同款拆票，不回退 09）。

## 缺什么

`RegisterCatalog` 用例（六个登记方法覆盖五类七表）在 `cmd` 全树零引用——租户运营方今天
没有任何进程级路径往 VE 目录里登记内容，五处 `*_NOT_CONFIGURED` 哨兵的「门」只造到用例层。

## 落点更正（本票开票时一并裁）

09 票 2026-08-21 的拆分评论把 B 半边写成「装配接线（`tenantBoundCustomerViewDerive` 所在的
`assemble.go`）」——**这是误绑**。登记是操作者动作不是信封消费：入口形状按
[票 12 的裁定](./12-governance-registration-has-no-process-entry.md)走**受控 CLI**
（与 `cmd/parcel-pricing-register` 同形），**不碰 `assemble.go`，不占号**。
`assemble.go` 里那个哨兵（披露策略空册 → 客户视图四维待确认）等的是**目录内容**
（实例半边，CLI 就位后由租户登记解除），不是代码接线。

## 落地约束

- 新受控 CLI（命名循 `cmd/parcel-pricing-register` 惯例，如 `cmd/parcel-ve-register`），
  覆盖六个登记方法；执行者身份按票 12 裁定的双轨（通道技术身份入口自取 + 登记内容里的
  批准责任标识显式必填）。
- 写入侧防重叠红线原样（`ErrAmbiguousCatalog` 同款，09 票面已钉）；只追加不改写。
- 目录内容全部属实例半边：本票只建口，不填值；验证用隔离行只记 `S`，不进生产装配。
- 真库验测：`go test -p 1 -count=1` 含 DSN，报状态写明含 PG。

## 参照

票 09（A 半边交付记录与红线）、票 12（受控 CLI 与执行者身份裁定）、ADR-0022→ADR-0003
（自报身份不作授权依据）；PAR-VIS-01/05/07/08/09（内容行，待提供是常态）。

## Comments

- 2026-08-21 MCP-3（受用户委托裁断）：随 09 收口开票，纠正 B 半边落点（CLI 而非
  `assemble.go`）。CC 侧同形缺口不入本票——[票 06](./06-cc-case-config-registries-have-no-writer.md)
  仍开着，其进程入口属 06 自己的余量，由 06 票面承载。
