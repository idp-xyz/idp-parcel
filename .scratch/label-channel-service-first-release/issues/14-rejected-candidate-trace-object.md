# 14 落选留痕挂在哪个对象上，不清楚

Category: enhancement
Status: draft
Blocked by: 01, 12

## 缺口

`PAR-NET-16` 要求给落选者留痕。今天有一个形似的东西但**不是它**：
`internal/networkrouting/domain` 的 `RouteCandidate` 带 `CandidateOutcome` 与
`CandidateReason`（`visibilityexception/adapters/networkrouting/derive_on_initial_route.go`
消费它）——那是**路由**候选的留痕。

渠道候选是否复用它、还是归 `parcel-shipment` 面单交易侧，**没有代码可指**。

## 为什么被两票阻塞

- `01` 裁定择优住哪个上下文——留痕对象大概率跟着择优走。
- `12` 产出候选集合的形状——留痕挂在候选上，候选长什么样先得有。

## 做什么

定落选留痕的对象与生命周期：挂在哪、留多久、谁读它。一件要正面回答的事：**留痕是不是
事实**——若是，它就有所有者与不可覆盖的要求；若只是一次判断的过程记录，随判断版本走即可。
两者的存储与读面形状不同。

## 红线

- 不复制第二套候选：留痕引用候选，不拷贝它的内容。
- 不填任何候选内容或金额（实例半边）。
- 若判定要复用 `RouteCandidate`，**要说明渠道候选与路由候选为什么是同一种东西**——它们的
  合格判据不同（一个看网络可达，一个看渠道约束），复用要有理由不能靠形状像。

## 完成判据

留痕对象有明确归属与形状并落地；`gofmt -l` 空、`go build`/`go vet` 退 0、
`go test -count=1 ./...` 绿并注明含不含真库。

## 参照

[能力形状盘点](../capability-shape-inventory.md)第三段；票 `01`、`12`。
