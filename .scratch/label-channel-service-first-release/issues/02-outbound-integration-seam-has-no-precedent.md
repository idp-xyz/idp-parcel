# 02 出向集成缝在本仓没有先例：失败、超时、重试与幂等的表达要先定一次

Category: enhancement
Status: resolved——已落 [ADR-0090](../../../docs/adr/0090-outbound-integration-result-algebra-partitioned-by-recovery-action.md)；实现（平台包）随第一个消费者票落，见文末
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

## 裁决

裁决人：本会话，经 owner 明确授权（2026-09-02）。结论落
[ADR-0090](../../../docs/adr/0090-outbound-integration-result-algebra-partitioned-by-recovery-action.md)，
本节只摘七条要点，**口径以 ADR 为准，不在此复制第二套**：

1. **失败代数按调用方的恢复动作分格**，直接继承 [ADR-0029](../../../docs/adr/0029-retrieval-failure-algebra-is-partitioned-by-recovery-action.md)
   的仲裁规则，不另立。三格：`对端已答复且答复是拒绝` / `答案未确定` / `确证未受理`。
2. **`答案未确定` 是默认格**，`确证未受理` 需要举证。超时一律落前者，无例外——判错方向不对称，
   误报为可重发会产生真实供应商成本与第二个运输标识。
3. **不镜像 [ADR-0022](../../../docs/adr/0022-http-status-carries-answer-formed-not-business-verdict.md)。**
   本票第 1 点原写「判据同 ADR-0022 立的那一条，方向相反但分界线是同一条」——**这句话被裁决更正**：
   ADR-0022 是本仓作为服务端的自律，对端不受它约束。UniUni 恒回 200 是已取证的反例。
   「有没有形成答案」由每家适配器判定，平台层不得看 HTTP 状态码代答。
4. **端口交回封闭代数，不是 `*http.Response` 也不是裸 `error`**（同 ADR-0031 形状）。
5. **不建第二套幂等键**，锚在领域已有身份上，对称 `outboxintent.EnqueueOnce`（本票第 2 点照办）。
6. **未配置即拒**，沿用既有未配置格形状；**超时、重试次数与退避一律实例参数**（`PAR-INT-02`），
   机制不设生产默认值——猜出来的超时会直接改变一次调用落在哪一格。本票第 3 点问的
   「归适配器还是归编排」答案是：判定归适配器，**编排不得自动重试 `答案未确定`**。
7. **落 `internal/platform/` 新包**，`07` 与 `15` 共用（本票红线「不要只落在某一个适配器里」照办）。

### 实现归谁

ADR 定形状，**平台包的代码随第一个消费者票落地**——即 `07`（取面单出向端口）。这不是把活推走：
按本票红线，出向缝的定义要「可被两票消费」，而一个没有消费者的平台包无法证明它装得下什么。
`15` 落地时若发现代数装不下轨迹源的某种情形，**回本票重开**，不在 `15` 里私自加格。

### 本票留下的一个未答项

ADR 第六条要求每个出向端口配一个查询能力（`答案未确定` 的续办只能是查询或对账）。
**各家渠道有没有查询口，本票未取证**——它属各源接口形态，归 `07`/`15` 各自取证。
查不了的要如实记为「该源无查询口」，不得以重发顶替。
