# 09 transport-fulfillment：履约判断方法与出入向连接器

Category: enhancement
Status: needs-triage——2026-09-24 通道 4 随票 02 立（登记册逐行拆分划出的产品策略，TF 一张）；逐项先核执行器有无
Blocked by: 无（第 1、3 项里公开承运商接口的参考配置那半等 03）
地盘：transport-fulfillment 的连接器适配器、派送发起与各判断方法所在的领域 / 应用层，`cmd/` 对应装配点。
出处：[票 02](./02-split-parameter-register-and-retriage-deferrals.md)——[参数登记册](../../../docs/product/PILOT-PARAMETER-REGISTER.md) `PAR-INT-03`、`PAR-INT-07`、`PAR-NET-07`、`PAR-NET-09`、`PAR-NET-12`、`PAR-NET-15` 行内「〔ADR-0146 拆分〕」点名的部分；[ADR-0146](../../../docs/adr/0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md) 决定一「连接器形态」、决定二「公开承运商接口归参考配置」。已知缺口沿[开发主线](../../../docs/product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)「按四项判据重定级」表 PN-04 行。

## 做什么

1. **外部轨迹来源连接器**（`PAR-INT-03`）。`TrackingSource` 一家实现都没有，端口注释称「设计而不是欠账」；按 ADR-0146 连接器形态归产品。已知缺口：重定级表 PN-04 行第三项。
2. **派送发起策略**（`PAR-NET-09`）。派送执行器与端点都在（`cmd/parcel-api/assemble_delivery_dispatch.go`），缺内置的发起策略；拍频是租户取值，可出参考配置。已知缺口：同上。
3. **待核：运输委托 / 订舱 / 承运接受的交互连接器**（`PAR-INT-07`）。核现有委托订舱编排之外有无出向连接器；公开承运商订舱接口按参考配置出。
4. **待核：揽收控制成立与交接证据证明力的判断形态**（`PAR-NET-07`「实际接收与控制成立规则、证据规则」、`PAR-NET-15`「交出/接收/拒收/封签/点验证据来源及证明力」）。ADR-0135 与交接登记编排已定了裁决值与转出引用；核「哪些证据足以成立」有无判断形态，没有则内置形态，租户选形态并映射其伙伴的证据。
5. **待核：POD 有效性与接收方核验的判断形态**（`PAR-NET-09`「接收方核验、POD 证据集合与有效性」）。
6. **待核：缩容与超配缺口的处置方法**（`PAR-NET-12`「缩容与超配缺口处置」）。
7. **履约段粒度政策**（票 02 暂缓清单：[ADR-0096](../../../docs/adr/0096-a-fulfillment-segment-identity-is-declared-not-derived.md) 决定四判为「租户组织运输控制的方式……本产品不代租户拟」）。两次控制事实何时算同一段（一次揽收一段、一车一趟一段、一承运商一线路一段）是判断方法，做成内置形态，租户选；段身份仍由登记方声明。
8. **派送任务合并规则**（票 02 暂缓清单：[ADR-0114](../../../docs/adr/0114-delivery-dispatch-is-triggered-by-entering-a-declared-delivery-segment-and-pulls-requirements-by-reference.md) 决定二判为「租户的运营政策（实例半边），本记录不代拟」）。同一派送段里的对象怎样并成一个任务（形态举例：同一收件地点引用并一单）做成内置形态，租户选；不选照旧一对象一任务。
9. **总单号的公开格式与校验**（票 02 暂缓清单：[ADR-0113](../../../docs/adr/0113-carrier-master-document-is-an-independent-register-keyed-by-declared-reference-and-version.md) 决定二「号码格式、号段与校验规则属实例半边」）。公开标准的部分（如空运单号的校验位）按参考配置出，号段仍是租户取值。

履约伙伴择优的排序形态与路由排序策略族同源，归票 04（`PAR-NET-16`），本票不做。

## 不做

- 不接任何真实伙伴的凭证，不替租户定拍频、证据集合或容量阈值。

## 完成判据

- 每项要么有执行器（连接器至少一种形态能对合成对端跑通，带测试），要么记下已有执行器的证据；登记册对应行同步收短。
