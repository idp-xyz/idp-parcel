# 渠道适配缝设计备忘:外部承运商与渠道数据从哪个缝进来

本文只做一件事:把外部承运商/渠道对接的每一类数据,预先指到既有限界上下文的所有权
与端口缝位上,防止第一个真实渠道到来那天,适配器被塞进错误的上下文。**只指缝,不
实现,不排期,不改首发范围**——独立面单渠道服务不进入首发生产、不建设向承运商询价
竞价的采购过程,均以[首发试点范围](../product/PILOT-SCOPE.md)与
[首发开发主线](../product/PARCEL-NETWORK-FIRST-RELEASE-DEVELOPMENT-BASELINE.md)为准。

进入条件沿用开发主线的能力范围判据:**必须能指出一类真实存在的承运或渠道商业模式
需要它,且该模式在目标客户群里是常规形态**。指得出来时,按本文缝位立票;任何取值
仍由价卡或商业政策版本携带,不得内置为常量。

## 缝位

外部渠道 API 常见的五种能力,加上账号凭证一件,各自的落点:

| 渠道数据 | 所有权上下文 | 既有缝位 | 要点 |
|---|---|---|---|
| 运单创建 / 订舱应答 | `transport-fulfillment` | 运输委托、订舱与承运接受机制 | 外部「创建运单」在本仓语言里是向承运商发起运输委托并收订舱应答;**承运接受才成约**,渠道回执不是承诺 |
| 面单 / 标签获取 | `parcel-shipment` | 面单交易 | 领域缝已在;独立面单渠道服务不进首发生产,启用属范围变更,先回试点范围文档 |
| 轨迹采集 / 状态同步 | `transport-fulfillment`(承运/移动/交付)、`node-operations`(节点事实) | 实际履约事实登记;设备与外部事实的身份时间不代铸([ADR-0023](../adr/0023-work-fact-identity-and-time-are-minted-by-the-device.md)) | 外部轨迹必须先由事实所有者收编为自己的事实;`visibility-exception` 只消费已接受事实形成投影,**不得把渠道轨迹直插投影** |
| 清关状态 / 监管结果 | `customs-compliance` | 外部监管结果接收(装配面已有 `/customs/external-results` 端点) | 六层分立、同层冲突不覆盖;渠道推送只是传输形态之一 |
| 费用查询 / 公布资费 | `parcel-pricing`(资费)、`transport-fulfillment`(实际收费发生项) | BUY 价卡登记册;运输收费发生项 | 承运商公布资费是某张 `BUY` 价卡的**来源**(CONTEXT「价格方向」词条);发生项无金额字段,金额责任归 `settlement-accounting` |
| 渠道账号 / 凭证 / 关系 | `party-commercial` | 渠道关系与账号授权;关系矩阵已冻结于[代理商、Carrier 与 Carrier Service 关系开发交接](./party-carrier-channel-relationship-development-handoff.md) | 账号持有人、合同相对方、实际承运商是不同角色,不得合并成一个「渠道」对象 |

## 批量文件与平台回调也是渠道

Excel 批量导入、ERP 推送、电商平台回调,与在线 API 同属**接入渠道**的不同形态,
都落在 [ADR-0055](../adr/0055-business-endpoint-intake-has-an-unconfigured-grade.md)
的 Intake 缝上:渠道未配置时一律如实答 403,不因形态是文件就另开旁路。渠道本身属
实例半边;任何真实渠道 Intake 之前,ADR-0055 Decision 五点名的两项机制未决(载荷
规范化摘要、准入范围装配)须先闭合。

## 不做清单

- **不立「渠道中心」上下文。** 五类数据的所有权分属既有上下文;建一个合库的渠道
  读写面会穿 [CONTEXT-MAP](../domain/CONTEXT-MAP.md) 的所有权边界(与
  [ADR-0077](../adr/0077-master-data-catalogue-read-follows-the-operations-read-pattern.md)
  拒绝跨上下文合库读面同一条理由)。
- **不建通用 Carrier 主数据平台。** 已由关系交接文档冻结。
- **不实现任何适配器、不排期。** 本文是缝位登记,不是工作包。

## 触发与消费方式

第一个真实渠道商业模式被指名时:按上表缝位立票,一类数据一张票,落在所有权上下文
的 `adapters/` 下;涉及新增外部事实形态时先回该上下文 `CONTEXT.md` 对词汇,不在
适配器里发明第二套语言。
