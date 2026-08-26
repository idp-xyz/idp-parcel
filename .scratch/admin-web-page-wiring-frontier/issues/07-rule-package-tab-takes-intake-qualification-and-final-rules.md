# 接单规则包页签扩正文：收寄资格与终局规则

Category: enhancement
Status: ready-for-agent

自[票 04 的导航裁决](./04-registered-but-unreadable-rows-need-a-nav-ruling.md)。**不新增端点、不加页签**——这两样是接单规则包这一个商业对象的其余正文面，归 `/commercial-policies?kind=ACCEPTANCE_RULE_PACKAGE` 那一格。

## 事实（实读代码，锚 `dc611a3`）

1. 归属由 0013 迁移抬头写死：收寄资格与终局规则挂**接单规则包**（`object_kind=4`）；产品与合同是采用方，不是本族的主键。
2. 表形：
   - 收寄资格 = `intake_qualification_content`（壳）+ `intake_allowed_source`（允许来源，封闭二值 `NODE_INTAKE` / `OFFSITE_PICKUP`）+ `intake_qualification_ref`（资格引用）。**两张子表纪律不同**：允许来源不允许空，资格引用允许显式空（真没有硬资格）。
   - 终局规则 = `final_rule_content`（壳）+ `final_rule_declaration`（按责任结果封闭四值 `EFFECTIVE_DELIVERY` / `RETURN_COMPLETED` / `SERVICE_TERMINATED` / `REGULATORY_DISPOSITION` 进主键，各带一个 `final_kind`）。
3. 种子发布批的 `SYN-RULEPKG-01` 声明里，`INTAKE_QUALIFICATION` 与 `FINAL_RULE` 两项都灌过（`publish` 输出可见 `ALREADY_REGISTERED`）。
4. 现状读面：`OperationsCatalogue.ListAcceptanceRulePackages` 已经父 LEFT JOIN 子一次取回 `acceptance_rule_package_rule`，页面 `RulePackageRecord.rules[]` 已上列。本票是在同一行上再挂两族正文。

## 要做什么

- `ports.AcceptanceRulePackageRow` 扩字段（不新增行类型）：加收寄资格与终局规则两族，各自带一个「壳在不在」的显式布尔。
- `ListAcceptanceRulePackages` 的语句扩连接。**注意 `json_agg` 的多重 LEFT JOIN 会做笛卡尔积**——三族子表同时挂上去会把每族的行数乘起来。用相关子查询各聚各的（`(SELECT json_agg(...) FROM ...)`）而不是继续加 `LEFT JOIN` + `GROUP BY`；这一条是本票最容易踩的坑，现有语句只有一族所以看不出来。
- 传输层 `RulePackageRecord` 扩字段；页面在接单规则包页签上把两族展开。
- 无需动 `cmd/parcel-api`、无需动放行面枚举、无需动 `page-registry.tsx`——端点与页面都已经 live。**本票不碰任何共享接线文件。**

## 三条形状约束

1. **壳在不在要显式布尔**，判据与票 01 的 `contentRegistered` 一模一样：无父行 = 未声明；有父行零子行 = 明确的空。0013 自注：领域要求「至少一行」子声明，SQL 表达不了，因此有父零子在**内容读口**是坏数据——但目录上列不重建领域对象、不形成判断，照 `ListAcceptanceRulePackages` 既有那条注释的先例，上列如实交回空集合，拦坏数据仍归内容读口。这一条要在新代码里原样重申一次，别让下一个人以为该在这里抛。
2. **允许来源与资格引用不能合成一栏。** 前者不允许空、后者允许显式空，两栏的「空」含义不同。
3. **终局规则的四个责任结果是封闭集**，读回集外取值即坏数据，上抛。中文取 CONTEXT 原词进 `presentation.ts` 词表。

## 完成标准

- `/commercial-policies?kind=ACCEPTANCE_RULE_PACKAGE` 答 `200`，`SYN-RULEPKG-01` 一行上同时可见规则集、允许来源、资格引用（或显式空）与四个终局结果。
- 真库测试钉住：三族互不串行数（笛卡尔积回归）、壳缺席与零子行可分辨、租户隔离、`limit` 非正即拒。
- 全仓 `go test -count=1 ./...` 绿（含真库）；页面层 DOM 取证照票 01 脚本。
