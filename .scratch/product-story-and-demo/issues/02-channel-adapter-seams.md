# 02 渠道适配缝设计备忘

Category: feature
Status: ready-for-agent
Owner: MCP-3

写 `docs/design/channel-adapter-seams-design-note.md`:把外部承运商/渠道的五类数据
(运单创建、标签获取、轨迹查询、费用查询、状态同步)与批量/平台接入,各自指到既有
上下文的端口缝位。目的是第一个真实渠道到来那天不把适配器塞进错误的上下文。

## 要求

- 只指缝,不实现、不排期、不改首发范围(面单渠道服务不进首发生产、不建询价竞价)。
- 每类数据写清:所有权上下文、既有机制/端口、事实流向(如外部轨迹须先由
  transport-fulfillment/node-operations 收编为自己的事实,VE 只消费已接受事实,
  不得直插)。
- 批量文件/电商平台回调定位为 ADR-0055 接入渠道 Intake 的渠道形态实例半边,
  点名「渠道不限在线 API」;不动已接受 ADR 正文。
- 写明不做清单:不立渠道中心上下文、不建通用 Carrier 主数据平台(引 party-carrier
  关系交接)、不实现任何适配器。
- 触发条件:能力范围判据点名真实商业模式时按本文缝位立票。
- `docs/README.md` 设计文档段加一行入口(占号纪律)。

## 完成标准

文档落位,README 索引行加上,自己提交,票面记 resolved + SHA。

## Comments
