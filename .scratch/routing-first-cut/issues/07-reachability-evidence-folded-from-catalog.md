# 07 可达性证据从版本化网络目录折出：视图修订、服务区域、候选与可执行性

Category: enhancement
Status: in-progress——2026-09-24 通道 5 认领（单 task-2c47b04e-3fa8-4f06-8bc2-4d9ba6d00936），分支 `mcp5-rfc07` 基 `f9fffabe`；迁移号预留 network_routing `0011`。此前：ready-for-agent
Blocked by: 02
父票：[psb/04](../../product-strategy-boundary/issues/04-routing-product-strategy-first-cut.md)「接路由证据取数侧」那一步（可达性一侧）
地盘：network-routing 目录的内容列（服务区域覆盖、节点对区域的覆盖等，新迁移，号开工时在频道预留）、目录登记口、postgres 取数侧、证据视图端口；[ADR-0068](../../../docs/adr/0068-versioned-network-catalog-structure-precedes-rule-content.md) 状态行与两处护栏注释（`0008` 迁移头注、目录适配器 `NetworkCatalog` 的类型注释）。
出处：[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定七；02 的 ADR；[ADR-0075](../../../docs/adr/0075-customer-address-is-carried-with-the-routing-request.md)；[ADR-0052](../../../docs/adr/0052-network-evidence-catalogue-has-an-unconfigured-grade.md) 与 [ADR-0053](../../../docs/adr/0053-network-fact-families-are-derived-not-registrable.md)（三格、不得退成空证据）；[`first-tenant-runway/03`](../../first-tenant-runway/issues/03-network-resolution-layer.md) 的 Answer。

## 做什么

1. **第一步**：视图修订改由目录修订锚派生（first-tenant-runway/03 的 Answer「`0007` 与 `0008` 不合流」一节；漏掉的症状是静默错判，不是报错）。
2. 按 02 的决定给目录补内容列的**形状**（服务区域覆盖文法、节点对区域的覆盖角色等），登记口经领域构造门收；迁移不种任何默认行。
3. 取数侧按判断键的 `asOf` 选版，折出服务区域解析、候选（02 定的首版生成形态）、路径可执行性（含临时网络可用性调整）与目录内可得的硬约束。关务资格按 02 定的来源；来源未接时如实答状态未知，不答满足。
4. 证据视图端口按 ADR-0075 收地理解析投影——导出签名改动，开工前在频道报窗口。本票用合成投影测，PS 侧携带归 08。
5. 目录为空，或判断时点没有适用的路由策略版本 → `未配置`；登记了而解不出 → 照旧响亮上抛，不退成`未配置`，也不退成空证据。
6. 同笔：02 的 ADR 里 ADR-0068 决定六的部分停用在此生效，两处护栏注释一并改。

## 不做

- 不做时间投影、段链与成本（归 09、10）；不改 PS。

## 完成判据

- [ ] 真库用例：合成目录上可达性判断得出可达、不可达、资料不足各一；空目录与无适用策略各答`未配置`；目录改一笔后视图修订随之变。
- [ ] 应用层可达性用例经真取数侧跑通（证据层级 `S`）。
- [ ] ADR-0068 状态行、两处护栏与代码同笔。

## 开工设计（2026-09-24 通道 5，钉 `f9fffabe`；下一任接手照此）

**分层**：折叠是判断方法（ADR-0146 产品策略），不落 postgres 适配器（「适配器只翻译不判断」）。
- `ports`：`NetworkCatalogSnapshot` 从 postgres 适配器挪进 ports，加读端口 `NetworkCatalogRead`（`LoadDefinitionsAt`，目录适配器已实现同名方法）；加关务事实来源端口（逐候选交 `domain.HardConstraintFinding`）；`NetworkEvidenceView.LoadNetworkEvidence` 多收一个随请求携带的内容结构（首版只含地理解析投影，08 / 09 再加服务要求与承诺上界）。
- `application`：目录取数侧（实现 `NetworkEvidenceView`）= 读快照 → 判未配置 → 折服务区域解析、候选、可执行性 → 并入关务事实 → 交 `NetworkEvidence`；生产装配的关务来源是「未配置」实现，逐候选答状态未知（ADR-0148 决定三），测试用替身答满足以证「可达」（证据层级 `S`）。
- `domain`：地理解析投影（寄件段、收件段，各国家 / 地区码与邮编，原样；国家码不成形按资料不足）；服务区域覆盖的匹配（整国家 / 地区，或国家加邮编前缀逐字比）；服务区域解析补一格「起点侧不在覆盖内」——UC-NR-002 层次 1 写的是起止服务区域，现有三格只有终点侧。

**目录内容（迁移 `0011`）**：`service_area_version` 加覆盖国家、邮编前缀数组、始发节点数组、交付节点数组四列，全可空（存量行没有，按「缺哪一格候选在那一格如实不可用」处置），CHECK 钉形状；CONTEXT Language「服务区域」本就是「把客户地址解析为候选收寄节点、交付节点或尾程注入节点」，节点角色随区域版本走。

**折叠规则（ADR-0148 决定二、五、六）**：
- 未配置：目录修订锚不存在，或 asOf 没有 `applicable_scope` 等于判断键服务目的的路由策略版本。
- 候选：asOf 适用的每条线路，首节点至少服务一个区域的始发角色、末节点至少服务一个区域的交付角色，才成候选。
- 服务区域解析（逐候选）：寄件侧或收件侧缺国家码 → 资料不足（缺口点名哪一侧）；首节点所服务的区域都不覆盖寄件地址 → 起点侧排除；末节点所服务的区域都不覆盖收件地址 → 终点侧排除；都覆盖 → 覆盖（引所用收件侧区域版本）。
- 可执行性：线路各连接与节点在 asOf 都有适用版本，且无生效中的临时调整（停运 / 关闭类）作用于该线路、其连接或节点 → 可执行（引线路版本）；否则不可执行（引那条调整或缺版本的连接）。
- 视图修订 = 目录修订锚。

**同笔**：ADR-0068、ADR-0053 Status 行前向指针；`0008` 头注与 `NetworkCatalog` 类型注释两处护栏；`NetworkDefinitions` 退出两个证据视图（初始路由证据视图在 09 之前照旧响亮上抛「解不出」，但`未配置`改由目录与策略判，`0007` 不再被读）。
