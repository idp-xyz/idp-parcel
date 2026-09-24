# 12 可达性证据从版本化网络目录折出：视图修订、服务区域、候选与可执行性

Category: enhancement
Status: draft
Blocked by: 07
父票：[04](./04-routing-product-strategy-first-cut.md)「接路由证据取数侧」那一步（可达性一侧）
地盘：network-routing 目录的内容列（服务区域覆盖、节点对区域的覆盖等，新迁移，号开工时在频道预留）、目录登记口、postgres 取数侧、证据视图端口；[ADR-0068](../../../docs/adr/0068-versioned-network-catalog-structure-precedes-rule-content.md) 状态行与两处护栏注释（`0008` 迁移头注、目录适配器 `NetworkCatalog` 的类型注释）。
出处：[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定七；07 的 ADR；[ADR-0075](../../../docs/adr/0075-customer-address-is-carried-with-the-routing-request.md)；[ADR-0052](../../../docs/adr/0052-network-evidence-catalogue-has-an-unconfigured-grade.md) 与 [ADR-0053](../../../docs/adr/0053-network-fact-families-are-derived-not-registrable.md)（三格、不得退成空证据）；[`first-tenant-runway/03`](../../first-tenant-runway/issues/03-network-resolution-layer.md) 的 Answer。

## 做什么

1. **第一步**：视图修订改由目录修订锚派生（first-tenant-runway/03 的 Answer「`0007` 与 `0008` 不合流」一节；漏掉的症状是静默错判，不是报错）。
2. 按 07 的决定给目录补内容列的**形状**（服务区域覆盖文法、节点对区域的覆盖角色等），登记口经领域构造门收；迁移不种任何默认行。
3. 取数侧按判断键的 `asOf` 选版，折出服务区域解析、候选（07 定的首版生成形态）、路径可执行性（含临时网络可用性调整）与目录内可得的硬约束。关务资格按 07 定的来源；来源未接时如实答状态未知，不答满足。
4. 证据视图端口按 ADR-0075 收地理解析投影——导出签名改动，开工前在频道报窗口。本票用合成投影测，PS 侧携带归 13。
5. 目录为空，或判断时点没有适用的路由策略版本 → `未配置`；登记了而解不出 → 照旧响亮上抛，不退成`未配置`，也不退成空证据。
6. 同笔：07 的 ADR 里 ADR-0068 决定六的部分停用在此生效，两处护栏注释一并改。

## 不做

- 不做时间投影、段链与成本（归 14、15）；不改 PS。

## 完成判据

- [ ] 真库用例：合成目录上可达性判断得出可达、不可达、资料不足各一；空目录与无适用策略各答`未配置`；目录改一笔后视图修订随之变。
- [ ] 应用层可达性用例经真取数侧跑通（证据层级 `S`）。
- [ ] ADR-0068 状态行、两处护栏与代码同笔。
