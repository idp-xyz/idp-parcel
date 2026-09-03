# 18 外部承运凭证登记册：凭证指向哪个载运对象，今天没人答

Category: enhancement
Status: ready-for-agent
Blocked by: 无（`16` 已 resolved）

## 缺口

票 `16` 的收编执行器要把一条素材认领为「就明确外部承运凭证所指对象」的事实（TF CONTEXT
「外部承运轨迹事实」生命周期节：「素材带有发生时间**且对象、凭证可关联** → 事实成立」）。它通过
`ports.ExternalCarrierCredentialResolver` 问「这份凭证此刻指向哪个载运对象」，而全仓**没有任何生产实现**
——机制清点（`e697c9a`）「缺」名单已如实列入。执行器对此答`未决`（`TrackingCredentialRegistryUnconfigured`），
不留痕：那是本上下文自己的缺口，不是源的缺陷。

TF CONTEXT「外部承运凭证」词条早已定义了要登记的东西：分配方、真实标识对象、适用范围、版本、替代关系；
CONTEXT-MAP 把「外部承运凭证的身份和版本」列在 TF 拥有清单里。**缺的是那本登记册的代码与写面。**

## 做什么

1. TF 领域新立「外部承运凭证」聚合（或值对象＋登记册），按 CONTEXT 词条五件事建模；作废、失效、替代只改变
   适用关系，不删历史凭证（CONTEXT 规则节）。
2. `ports` 加登记册仓储；`adapters/postgres` 落表与迁移（`transport_fulfillment/0012`），真库实跑。
3. 以该登记册实现 `ports.ExternalCarrierCredentialResolver`：登记过 → `CredentialResolved` 并交回对象；
   没登记过 → `CredentialUnknown`（执行器据此留痕）。**`CredentialRegistryUnconfigured` 只在装配处没接
   登记册时出现**，接上之后这一格不再由本实现产出。
4. 登记写面（管理台或 `cmd/parcel-*-register` 形状）随实施票按 ADR-0101「登记频次 × 操作者角色 × 载荷结构」裁。

## 红线

- 凭证不解释为包裹的当前运单号（CONTEXT 词条）；「真实标识对象」是登记出来的，不从单号格式推断。
- 不填任何承运商的凭证格式、校验规则（实例半边）。
- 不动 `16` 落的执行器分格：`未知`留痕、`未配置`未决两格的语义由端口注释已定。

## 完成判据

登记册有实现与真库测试；`ExternalCarrierCredentialResolver` 有生产实现且机制清点「缺」名单里该口消失；
`16` 的执行器用例矩阵里「凭证不认识→留痕」一格能在真实现上复现。`gofmt -l` 空、`go build`/`go vet` 退 0、
`go test -count=1 ./...` 绿并注明含不含真库。

## 参照

票 `16` 完成记录「刻意留下的三格」第 2 条；TF CONTEXT「外部承运凭证」词条与规则节；
`internal/transportfulfillment/ports/external_tracking_fact.go` 的 `ExternalCarrierCredentialResolver`。
