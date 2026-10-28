# 01 内部合规限制形成与解除没有生产入口

Category: enhancement
Status: in-progress——2026-10-28 会话 tops-20（idp-parcel-91）立并认领（用户「现在就接线」），分支 claude-lw-cc
Blocked by: 无

## 做什么

把 `ManageInternalRestrictionHandler`（建立、决定）接成 `cmd/parcel-api` 的运营决定口：授予照现有运营决定种类的写法另立种类，租户与操作者取自认证信封，生产装配真库用例；演示租户授予种子照 `scripts/demo-seeds/data/access` 写法补。依据 UC-CC-011、ADR-0334（决定七、越权风险点 1）。

## 完成判据

- 持授予的运营人员经 `/v1` 能建立内部合规限制并作确认/解除决定；准入随之阻断或放开（真库用例）。
- 未授予答 403；重放答已有；租户取值不写死。
