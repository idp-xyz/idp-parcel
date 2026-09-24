# 13 各命令口的载荷规范化形状：ADR-0055 决定五第一项逐口解

Category: enhancement
Status: ready-for-agent——2026-09-24 随 ADR-0149 立（用户授权通道 4 自决）；按上下文拆笔
Blocked by: 无
父票：[psb/15](../../product-strategy-boundary/issues/15-operator-channel-per-adr-0100.md) 丙轨实施
地盘：各上下文 `domain` 里命令载荷的规范化形状与摘要（NO、TF、CC、SA 各一笔），`adapters/http` 的译装不改答复。
出处：[ADR-0149](../../../docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md) 决定四第一条；先例 `PSC-1`（parcel-shipment）、`PCC-1`（party-commercial）。

## 做什么

1. 逐口列出作业事实与外部结果命令口，各自定一版规范化形状、带形状版本前缀，摘要进信封。
2. 同身份同摘要答重放、同身份异摘要答内容冲突；已有形状的口（如提交口）只核对不重做。

## 完成判据

- 每个口有形状版本与往返用例；未定形状的口在清单上标明，真渠道不对它开。
