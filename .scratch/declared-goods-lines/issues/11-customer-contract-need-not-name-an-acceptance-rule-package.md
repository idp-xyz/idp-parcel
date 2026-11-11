# 11 客户合同不必指名接单规则包

Category: enhancement
Status: ready-for-agent
Blocked by: 无

依据：[ADR-0376](../../../docs/adr/0376-fba-submission-conditions-are-judged-as-required-documents-platform-identifiers-may-be-supplied-before-acceptance-and-rule-packages-follow-the-service-product.md) 决定三（PC CONTEXT 合同不变量已改）；ADR-0153 决定四（规则包按申报的服务产品选）；ADR-0032（合同指名只作确认）；ADR-0267 决定三（同号改形的前提）。

## 要做的

- 客户合同正文的「接单规则包」（`contractContent.rulePackage`）改为可缺：领域构造不再拒缺；规范化 `PCC-1` 同号改形，没指名即不编码，指名了的旧文档逐字节不变（钉一条改形前取证的摘要常量）。
- 外壳与正文都指名时仍要相等（`ConsistentAcceptanceRulePackage`）；外壳不指名、正文也不指名，商业解析照旧按申报的服务产品选规则包。
- 落库：合同正文表的规则包列改为可空（新迁移）；读回、目录行与查阅口照实答缺。
- 受控批文与在线登记两口（`registrationjson`、HTTP 草稿载荷）收不带规则包的合同正文。
- 管理台客户合同表单的规则包一格改为可选，说明「多产品合同不指名，规则包随服务产品选」。

## 完成判据

- 一份不指名规则包的合同版本登记成立；同一货主账户下两个服务产品各配一份规则包，申报哪个产品就解出哪一份，不答指名引用未确认。
- 指名了规则包的旧合同摘要常量不变；PC 与管理台各自的测试与三道门通过。

## 不做的

合同按产品各指名规则包（ADR-0376 Alternatives 否决）；接受前财务控制策略仍须合同指名，不动。
