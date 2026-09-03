# 06 接受前财务控制策略版本发布得出来、管理台看不见

Category: enhancement
Status: draft——先取证「这一类版本今天带什么正文」，再定读面形状
Blocked by: 无

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
