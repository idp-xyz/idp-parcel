# 08 隔离形态：主链命令面按 ADR-0091 逐口放行合成写

Category: enhancement
Status: in-progress——2026-09-24 通道 6 认领（通道 1 改派 task-a25599eb；原卡通道 4 零提交已撤），分支 `mcp6-oc08`，基 `443a472e`；拆法经用户授权通道 4 自决认可；ADR-0149 决定五写明本票的隔离放行不因生产渠道落地而退场
Blocked by: 无
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 乙轨——**不是操作者渠道**，是让隔离环境里的主链先答业务结果的那条路
地盘：`cmd/parcel-api` 端点表里主链命令面的装配行与各自的隔离 Intake 类型（照 `IsolatedPartyIdentityIntake` 与隔离提交口的形状），各上下文 `adapters/http` 里需要的隔离 Intake 实现。
出处：[ADR-0091](../../../docs/adr/0091-isolated-form-extends-to-the-write-path-by-graded-switches.md) 逐口放行（先例：隔离提交口、`/commercial-*` 身份族）；[psb/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md) 格 7、11、12、22 实测 `403 ACCESS_CHANNEL_NOT_CONFIGURED`，端点表注释写明这几行「两个开关都换不了」。

## 做什么

1. 逐口归类（与 04 同一张表）：节点收寄、场外揽收与尝试、承运商首次有效收寄判断、交接、移动、派送任务登记与发起、交付、段关闭、有效时间判断、关务外部结果、监管凭证登记、外部资金事实——ADR-0100 决定四、五明文不给这些口开操作者渠道。
2. 按 ADR-0091 逐口放行：只收 `SYN-` 租户的合成写，来源身份与时间照 ADR-0023 由请求如实携带、不代铸；一口一笔、一口一个变量，不并进既有开关。按上下文拆笔（NO、TF、CC、SA）。
3. 放行后在隔离环境按 psb/05 的动线重走这几格，答复写回 psb/05（那张票的第 3 步归通道 2 或其接手者）。

## 不做

- 生产上这些口的真渠道（09）；不放宽 `SYN-` 前缀门禁。

## 完成判据

- 隔离环境里上列各口对合成写答业务结果而不是 `ACCESS_CHANNEL_NOT_CONFIGURED`；生产形态（开关为 nil）逐字节不变（带装配测试）。
