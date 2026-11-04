# 商业版本每版一个对象页：按身份读回单条的读口 + 管理台对象页

Category: enhancement
Status: resolved——2026-11-04 两张子票进 main（01 `1dabc6f45`，02 `068b54033` + `559e5012d`，生成物 `c65181f23`）
出处：[commercial-registers-edit-containers spec](../commercial-registers-edit-containers/spec.md) 决定五把「每版一个对象页」记为后续（要后端读口）；用户 2026-11-04 答「按你的建议全部做」。

现状：服务产品、客户合同、供应商协议与九本政策册只有列表读口（`GET /commercial-service-products` 等，租户内按发布时刻倒序、页大小由接入面裁为固定上限）；管理台的行详情在工作台右栏，只能从已取回的列表里找行。一个版本没有自己的地址：引用链接只能带检索词落到目录，深链、另开标签、分享都指不到「这一版」。

切片：PN-02，主责 party-commercial，非关务专项。权威：CONTEXT「商业版本」（稳定身份、不可覆盖）、ADR-0077（目录查阅不触发判断）、ADR-0022（状态码只说答案有没有形成）、ADR-0055（Intake 先于一切请求内容）。

## 决定（ADR-0361）

1. **读口**：`GET /commercial-version-readbacks/{kind}/{objectId}/{version}`，挂操作者查阅读面。`kind` 是领域的商业对象类别闭集（`CommercialObjectKindNamed`：`SERVICE_PRODUCT`、`CUSTOMER_CONTRACT`、`SUPPLIER_AGREEMENT`、`ACCEPTANCE_RULE_PACKAGE`、`PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY`、`PRICE_RULE`、`SETTLEMENT_POLICY`、`CREDIT_POLICY`、`AUTHORIZATION_RULE`、`CUSTOMER_SERVICE_RULE`）；身份是（租户、类别、对象、版本），范围不是键。答 `{ outcome: "COMMERCIAL_VERSION_READ", kind, version: <与该类别目录行同形的一份> }`；不在册与别的租户同形，答 200 `COMMERCIAL_VERSION_NOT_FOUND`；类别或标识不合法 400。接受判断时点策略与接受前财务控制声明没有自己的版本（随规则包 / 合同），不在本口。
2. **读面**：三个目录读口各加按身份读一条的方法（`Find…`），Postgres 实现与各自的 `List…` 共用同一条语句、只多两格键过滤，不另写第二套行转写。
3. **管理台**：每本册的对象页地址 `#/<页>/<objectId>/<version>`（商业规则页带 `?register=<册>`），从工作台右栏「打开对象页」进；对象页用 `DetailPageTemplate`：壳（标识、版本、状态）、这一版的各格、版本历史（从目录取）、技术详情；动作：收尾…（对话框）、在目录中查看。「发新版本」仍在目录页右栏（预填状态住在目录页）。

机制：读口与地址判读。产品策略：对象页形态。租户取值：无。

## 子票

| 票 | 内容 | 状态 |
|---|---|---|
| [01](./issues/01-version-readback-endpoint.md) | 后端：按身份读回单条（ADR、端口、Postgres、HTTP、测试） | resolved（2026-11-04 进 main `1dabc6f45`） |
| [02](./issues/02-version-object-pages.md) | 管理台：四本册的对象页与右栏入口 | resolved（2026-11-04 进 main `068b54033`） |
