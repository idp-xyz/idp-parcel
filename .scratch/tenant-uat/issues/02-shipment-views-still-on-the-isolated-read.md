# 运单、待受理复核、受限运单处置三个读口仍挂隔离读

Category: bug
Status: ready-for-human

出自第 1 站 1.13、1.19。验收卡见 [stations/01-entry.md](../stations/01-entry.md)。

## 看到的

首个管理员进「运单」，页面说「接入渠道未配置」，让人去完成渠道配置。工作台「待受理复核」「受限运单处置」两张卡说「未接通——这项数据的接入还没配置好」。

## 实际原因

`cmd/parcel-api/endpoints_shipment_views.go` 里这三个读口挂的是 `isolatedReadFace()`。放行它的开关只接受 `SYN-` 前缀的合成租户，能看到的客户账户是 `assemble_isolated_read.go` 的 `isolatedReadCustomerAccounts` 那份写死的名单。验收环境按生产形态没有开这个开关。页面把这件事说成了「渠道还没配置」，这个账号也没有地方去配。

一个真实租户今天在管理台上看不到自己的运单。

## 先不改

验收环境不开这个开关：开了，这一格会显示成正常，缺口还在。修法（读口改走操作者授予，或另有口径）要先定，再动代码。
