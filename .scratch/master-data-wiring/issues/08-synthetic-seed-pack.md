# 08 合成 S 主数据种子包

Category: feature
Status: ready-for-agent
Owner: MCP-5(2026-08-25 13:02 改派;原派 MCP-2 自 11:40 后无提交、截至 13:02 未响应,用户经通道 3 授权 MCP-3 调度本轮)

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

- 2026-08-25 13:02 MCP-3(调度):两点补充。①种子设计一并满足演示动线的「成套成
  故事」要求(用户已放行,见 `.scratch/product-story-and-demo/issues/04-demo-journey.md`):
  同一合成租户下,服务产品/价卡/参考序列/网络目录/合规规则/商业策略互相引用,
  能讲通「建产品→配价→一单的一生」;不是每表孤立几行。②脚本与 JSON 的入库位置
  由你定并记回本票(建议 `scripts/demo-seeds/` 一类,明确标注仅限隔离环境);本轮
  完工报告改回**频道 3**。
