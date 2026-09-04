# 06 接受前财务控制策略版本发布得出来、管理台看不见

Category: enhancement
Status: blocked——取证已由 report.md A 组代做（只有壳）；MCP-3 2026-09-04 裁**②**：不按「只有壳」收口，正文表是缺口本体，另立 [party-commercial-context-gaps/07](../../party-commercial-context-gaps/issues/07-pre-acceptance-financial-control-policy-has-no-content-table.md)；本票的读面等正文落地后照 `?kind=` 分派加一格
Blocked by: party-commercial-context-gaps/07

## 裁决（MCP-3，2026-09-04，owner 授权自决）

**取②，不取①。** 票面完成判据第二条允许写「这一类版本只有壳、列壳没有信息量」并把它记成**长期事实**——那句话与
PC CONTEXT 正面冲突：词条明写策略「定义适用范围、共同通过条件和失败处置」，Rules 明写「合同要求组合控制时，策略必须
明确每项控制的适用范围、判断顺序、共同通过条件和失败或补偿责任」。壳不是这一类版本的形态，是**机制半边还没做**（与
`party-commercial-context-gaps/03` 补信用政策/供应商协议正文、ADR-0104 补客户服务规则正文同一形）。把机制缺口写成长期事实，
下一个读票 03 提示句的人会以为它不必再建。

取证（report.md A 组，锚 `08e62ec`）本票认可不重做：`migrations/party_commercial/` 无策略正文表；`0007` 是合同级声明
（答「要不要」，头注自己写「策略版本回答『控制怎么做』」）；SA `LoadControlPolicy` 读闭合 + 声明，不读策略正文。

**本票因此转 blocked，不收口也不做**：读面列的是正文，正文不在就没有可列的列面；等 pc-gaps/07 落表后，本票按 ADR-0077
通例在 `?kind=` 分派上加一格、与既有七册同形，那一步才是本票自己的活。**票 03 那句「今天没有册可看」的提示句不改**——它今天
仍是真话，改的时机是读面落地那一刻，与本票同笔。

**能力边界**：读过本票、PC CONTEXT「接受前财务控制策略」词条与 Rules 三句、`0007` 迁移全文、report.md A 组该条；没读
`pre_acceptance_control_policy.go` 全文与 ADR-0044/0054/0079 正文——裁的是「壳是不是长期事实」这一问，不裁正文形状（归 pc-gaps/07）。

## 缺口

发布口对象类别 `PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY`（`CommercialObjectKind` 第 5 类）的
版本可以发布成功，但管理台没有任何一本册列它：「商业规则与策略」页的接受前财务控制册列的是
**挂在客户合同版本下的声明**（`0007`，`object_kind=2`，答「这份合同要不要控制」），不是策略
版本本身（答「控制怎么做」）。发布之后操作者在管理台找不到自己刚发的那一版，只能靠它被
解析选中时间接看见。

与票 [04](./04-customer-account-register-has-no-read-face.md) 当初记的 `customer-account`
「有写面无读面」同族。发现于票 [03](./03-publication-write-face-blocked-by-two-misaligned-closed-sets.md)
取证（2026-09-03，锚 `c93abba`）。

## 先取证

- 这一类版本今天在库里有没有正文表（`migrations/party_commercial/` 里哪一份），还是只有
  `commercial_version` 上的版本壳？没有正文表的话读面能列的只有壳。
- 结算政策解析（ADR-0044）实际采用的控制策略版本从哪张表读——那条读路径就是读面该转写的列面。

## 完成判据

要么读面上多一本册（沿 `?kind=` 分派加一格，与既有七册同形、ADR-0077 通例），要么票面写明
「这一类版本只有壳、列壳没有信息量」并把它记进票 03 那句「今天没有册可看」的提示里作为长期
事实。两条都算完成；不算完成的是继续让它发布得出来而看不见。

## 边界

不动发布口、不动领域；`cmd/parcel-api/endpoints.go` 若要加行按共享接线文件纪律占号。
