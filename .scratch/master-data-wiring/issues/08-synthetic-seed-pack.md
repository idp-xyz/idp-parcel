# 08 合成 S 主数据种子包

Category: feature
Status: draft
Owner: MCP-2

一套隔离合成 S 主数据种子,经四个登记 CLI(parcel-pricing-register /
parcel-network-register / parcel-customs-register / parcel-commercial)灌入本机库,
让七页在 demo 环境有数据可看。

## 纪律(红线映射)

- 命名走合成口径(SYN- 前缀,对齐 PN-02 合成任务包),不影射任何真实企业。
- 证据层级只记 S;不进参数登记册、不改任何「待提供」状态。
- 种子 JSON + 运行脚本入库(可复现),但明确标注仅限隔离环境;不写生产默认值。

## 完成标准

脚本一键灌入干净库零报错;七页(票 07 后)可见种子数据;自己提交,票面 resolved + SHA。

## Comments
