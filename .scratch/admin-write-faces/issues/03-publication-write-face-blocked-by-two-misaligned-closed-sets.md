# 03 发布口的在线写面卡在两套对不齐的封闭集上，先答词表再谈落点

Category: question
Status: resolved——三问已答、落点已裁并落地（2026-09-03，MCP-3，owner 授权自决），见文末「裁决与交付」；两处顺带核出的缺口另立票 06、07
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

## 裁决与交付（2026-09-03 · MCP-3，owner 授权自决；取证锚 `c93abba`）

**三问的答案，都是代码里已经有的事实，不是新裁——本票只把它们摆到操作者看得见的地方。**

1. **九类与六本册（今天七本）是两条分类轴，本来就不该对齐。** 权威在后端
   `query_commercial_policies.go` 的头注：「种类命名**册子**而不是商业对象类别：接受前财务控制
   声明挂在客户合同版本下、时点锚声明挂在接单规则包版本下，拿对象类别当种类名会指错拥有者」。
   发布轴是 `CommercialObjectKind`（版本是哪一类），册轴是「谁拥有这本正文 / 声明」。逐册对应：
   接单规则包 ← `ACCEPTANCE_RULE_PACKAGE` 版本（正文经 `RULE_PACKAGE_BODY` 通道）；接受前财务
   控制 ← 挂 `CUSTOMER_CONTRACT` 版本的声明（`PRE_ACCEPTANCE_CONTROL` 通道，`0007` 的
   `object_kind=2`），**不是** `PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY` 版本；商业价格政策 ←
   挂 `PRICE_RULE` 版本的正文（`PRICE_POLICY_BODY` 通道，`0010` 的 `object_kind=6`）；结算政策 ←
   `SETTLEMENT_POLICY` 版本；时点锚 ← 挂 `ACCEPTANCE_RULE_PACKAGE` 版本的声明（`AS_OF_POLICY`
   通道，`0005`）；授权规则 ← `AUTHORIZATION_RULE` 版本（取消授权经 `CANCELLATION_AUTHORITY`）；
   信用政策 ← `CREDIT_POLICY` 版本（`CREDIT_POLICY_BODY` 通道，`0020`）。**处置不是对齐词表**
   （对齐会把两个拥有者压成一个），**是在管理台把对应关系说出来**：册名旁一句「谁喂它」
   （`policyKindSources`），发布签提示句写明 kind 是发布轴、与册名不逐字对应、`declarations`
   里的通道不是 kind。
2. **`CREDIT_POLICY` 发布之后落在信用政策册。** 本票立票时那一句「没有独立正文册」已过期：
   票 party-commercial-context-gaps/03 落了正文表（`0020`，`877444a`），读面第七本册随之上了
   页面（`policy-rows.test.ts` 已钉「信用政策是第七本册」）。问题消失，不是被裁掉。
3. **`AS_OF_POLICY` 没有自己的版本。** 它是 `DeclarationChannel` 的一格，随所属接单规则包
   版本在同一次发布的 `declarations` 里登记（`0005` 归属键取规则包版本四维）；它的「版本」
   就是规则包版本。所以它不在九词里是对的，册在页面上也是对的——缺的只是页面没说这件事，
   现在册名旁那一句说了。

**落点裁决**：发布签摆在**商业规则与策略页**，作最后一签「受控发布（JSON 镜像）」。理由：
九类里六类的结果显示在本页的册里，本页是「写签跟着读签走」能走到的最大一页；专页是九类
结果一个都不在场的纯写页（票面已排除的第二条路，理由不变）。票面排除本页的理由（同屏两套
词）由上面第 1 条的处置解掉：两套词各自是什么、怎么对应，签上和册名旁都写了，页面教的是
真规则。按 ADR-0101 决定一，这一签是受控批量口的在线镜像、不是运营配置员的主路径，故列末签；
各册的逐字段表单按决定八逐册另裁（票 07）。

**顺带纠正一句八类共用的提示**：`snapshotHint()` 里「本页不逐字段建表单，因为『渠道原始载荷 →
登记快照』的翻译属渠道接入契约，随 PAR-INT-01 提供」——这条理由已被 ADR-0101 收窄为只适用
客户渠道载荷，对操作者面不成立，改为「本签是受控批量口的在线镜像（ADR-0101），逐字段表单按
各册实施票另建」。`api.ts` 里「本表没有页面在消费」那段注释同步改写。

**交付**：`CommercialPoliciesPage.tsx`（两签：政策册 / 受控发布；册名旁「谁喂它」一句）、
`presentation.ts`（`policyKindSources`、发布提示句改写、共用提示句改写）、`api.ts`（注释）、
`policy-rows.test.ts`（钉每本册都有那一句、两对近形词互相点名、提示句含九词且不再说
PAR-INT-01）。验证：`tsc --noEmit` 退 0、`node scripts/run-tests.mjs` 38/38（原 36 + 新 2）。
纯前端，Go 侧零改动；`vite build` 不声称（本机 `node_modules` 缺件，见票 pricing/05 MCP-4
Comment，未在共享树上 `pnpm install`）。

**两处顺带核出的缺口，另立票**：
- **票 06**：`PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY` 版本本身没有任何册可看（发布成功后
  管理台上找不到它，只能被解析读到）——与 `customer-account` 当初「有写面无读面」同族。
- **票 07**：九类发布的运营主路径（逐字段表单或模板导入）按 ADR-0101 决定八逐册裁形，本票
  只落了 JSON 镜像签。
