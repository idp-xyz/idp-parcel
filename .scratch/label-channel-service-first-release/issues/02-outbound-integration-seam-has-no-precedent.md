# 02 出向集成缝在本仓没有先例：失败、超时、重试与幂等的表达要先定一次

Category: enhancement
Status: ready-for-human
Blocked by: 无

## 为什么先立它

两份盘点各自撞上同一件事：**本仓至今没有任何出向集成**。按
`http\.Client|http\.Get|http\.Post|http\.NewRequest|http\.DefaultClient` 扫全仓 `*.go`
**零匹配**（实测于 `9e6d53a`）——`net/http` 的用法全在服务端一侧。

于是取面单（[能力形状盘点](../capability-shape-inventory.md)第二段）与轨迹源接收
（[轨迹源盘点](../tracking-source-seam-inventory.md)第一段）会同时问出同一个问题：
出向调用的失败、超时、重试与幂等在本仓怎么表达。**两票各答一次，答案就会分叉**，而分叉
之后再统一要动两处已经接了真渠道的适配器。

## 做什么

只定形状，**不接任何真实外部系统**：

1. 出向调用的结果代数：把「对端明确拒绝」「对端答了但答案不确定」「没形成答案（超时/
   连不上）」分开——判据同 [ADR-0022](../../../docs/adr/0022-http-status-carries-answer-formed-not-business-verdict.md)
   在入向侧立的那一条，方向相反但分界线是同一条。
2. 幂等：出向重试与「对端已经受理过一次」怎么分辨；本仓有 `outboxintent.EnqueueOnce`
   与 inbox 的重放纪律作内向先例，出向要不要对称，在这一票里答。
3. 超时与重试归属：归适配器还是归编排；重试次数与退避是不是实例参数。
4. 凭证的表达位置——**只定位置不填值**（账号与密钥属 `PAR-INT-02`，实例半边）。

## 红线

- 不接真实渠道或轨迹源，不写任何渠道字段名、账号、频率（[ADR-0088](../../../docs/adr/0088-label-channel-service-enters-the-first-release-service-forms.md) Decision 五）。
- 不新造领域语言（spec「边界」节）。
- 结论要么落成可被两票消费的类型与文档注释，要么落 ADR——**不要只落在某一个适配器里**，
  那等于把先例藏进一处实现。

## 完成判据

- 出向缝的形状有一处**可被引用的定义**（类型或 ADR），取面单票与轨迹源票都能指向它。
- `gofmt -l` 空、`go build ./...`/`go vet ./...` 退 0、`go test -count=1 ./...` 绿并注明含不含真库。

## 参照

[能力形状盘点](../capability-shape-inventory.md)第二段、[轨迹源盘点](../tracking-source-seam-inventory.md)第一段；
`internal/platform/outboxintent`、`internal/platform/inboxconsume`（内向先例）。
