# 03 发布口的在线写面卡在两套对不齐的封闭集上，先答词表再谈落点

Category: question
Status: ready-for-agent——但**第一步不是写页面**，是答下面三个领域问题
Blocked by: 无（票 [02](./02-remaining-registries-take-online-registration-faces.md) 已把
`publication` 排出本批，本票承接）

## 缺什么

`/commercial-publications` 是商业上下文八类写面里唯一一个已在端点表、有传输层与真装配、
前端也有快照提示与答案代数词表，**却没有任何一页挂它的登记签**的。票 02 逐页找过落点，
四张候选读面页（客户与合同、供应商协议、服务产品、商业政策）都不能摆，成因不是落点不好选。

## 为什么不能摆：两套封闭集不是子集关系

逐词比对于 `0d07866`。

发布口的**对象类别**九词（`apps/admin-web/src/pages/party/presentation.ts` 的
`registrationSnapshotHints.publication`）：

    SERVICE_PRODUCT / CUSTOMER_CONTRACT / SUPPLIER_AGREEMENT / ACCEPTANCE_RULE_PACKAGE /
    PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY / PRICE_RULE / SETTLEMENT_POLICY /
    CREDIT_POLICY / AUTHORIZATION_RULE

商业政策页的**册子** chip 六词（同目录 `api.ts` 的 `CommercialPolicyKind` 与
`presentation.ts` 的 `commercialPolicyKinds`）：

    ACCEPTANCE_RULE_PACKAGE / PRE_ACCEPTANCE_CONTROL / PRICE_POLICY /
    SETTLEMENT_POLICY / AS_OF_POLICY / AUTHORIZATION_RULE

精确同词只有三个（`ACCEPTANCE_RULE_PACKAGE`、`SETTLEMENT_POLICY`、`AUTHORIZATION_RULE`）；
两对近形而不同词（`PRICE_POLICY` 对 `PRICE_RULE`、`PRE_ACCEPTANCE_CONTROL` 对
`PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY`）；`AS_OF_POLICY` 有册却不在九词里；
`CREDIT_POLICY` 在九词里却无册。**数在这里是论点本身**，故锚 SHA；这几处一旦被改齐，
本票的举证随之作废，那正是本票希望发生的事。

根因写在 `api.ts` 的 `kindColumns` 头上：**种类命名册子而非商业对象类别**。两条分类轴，
部分词碰巧重合，而重合让人以为它们是同一套。

## 三个必须先答的问题

1. **九个对象类别与六本政策册是什么关系？** 一个类别一本册？多类别共一册？还是册按读面
   切分而类别按发布切分、两者本来就不该对齐？若是最后一种，管理台要说清这件事，而不是把
   两套词并排摆出来。
2. **`CREDIT_POLICY` 发布之后落在哪？** 今天读面上没有它的册（页面注释自称「没有独立正文册，
   封闭集里如实没有它」）。发布一版信用政策会成功吗？成功之后谁能看见？
3. **`AS_OF_POLICY`（时点锚声明）有册却不在发布九词里，它的版本从哪来？** 若它不经发布口
   形成，那它与另外五本册的写入路径不同，这一点在读面上完全看不出来。

三问未答之前，发布签摆在任何一页都会把一套对不齐的词表教给操作者——判据与票 02 里否掉
「按身份切登记签」用的是同一句：页面不教一条不真的规则。

## 已排除的两条路（不要重走）

- **摆在商业政策页**（九类里盖得最多的那页）：chip 词与快照 `kind` 词在同一屏并排，两对
  近形不同。操作者照 chip 抄一个 `PRICE_POLICY` 进快照会被受理门拒，而那个词是页面刚教的。
- **另开发布专页**：不是因为要动 `page-registry.tsx` / `navigation.ts` / `liveIds`（那是
  MCP-3 的地盘，加行即可），而是因为发布口在管理台**没有读面**——九类的读口是各自的 list，
  没有 `/commercial-publications` 的 GET。专页会是纯写页，九类结果一个都不在本页可见，
  是「登进去看不见」的最大化版本。

## 完成判据

- 三问各有答案，落在该落的地方：属领域语言的进 `partycommercial` 的 `CONTEXT.md`，属难逆
  取舍的进 ADR，属实例状态的进参数登记册。**不在本票正文里另立第二套口径。**
- 词表对齐之后再定落点，落点判据仍是「写签跟着读签走」。
- 若结论是两套词表本来就不该对齐，则管理台要有一处显式说明，且本票记下这条结论供
  `customer-account`（票 02 记的另一个缺口）参照——那一格是「有写面无读面」，可能同源。

## 参照

[ADR-0085](../../../docs/adr/0085-registry-write-faces-enter-the-endpoint-table-with-unconfigured-grade.md)；
票 [02](./02-remaining-registries-take-online-registration-faces.md) 的 MCP-3 Comment
（本票的举证与裁定出处）。
