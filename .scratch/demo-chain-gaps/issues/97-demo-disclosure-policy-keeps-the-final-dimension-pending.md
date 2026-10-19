# 97 演示披露策略把 `SYN-ACCOUNT-01` 的终局一维登成待确认，有效交付之后客户视图的终局仍不展示

Category: enhancement
Status: needs-triage——2026-10-19 Claude Code 子代理立（演示动线重走，[product-strategy-boundary/05](../../product-strategy-boundary/issues/05-demo-journey-criterion-evidence.md)「重走（钉 `8197fa2f`，2026-10-19）」停点三）。
Blocked by: 无（客户在动线上看得见这一维，要等 [96](./96-customer-tracking-view-has-no-first-party-customer-intake.md)）
地盘：种子 `scripts/demo-seeds/data/visibility/`（披露策略新一版）、`seed.sh` 一行、种子 README 组成表 `data/visibility/` 那一行。
出处：VE CONTEXT 生命周期「当前投影、客户可见 ETA、客户服务终局或已批准披露内容首次满足展示条件 → 形成……客户视图版本」；[UC-VE-008](../../../docs/application/visibility-exception/UC-VE-008-PROVIDE-CUSTOMER-END-TO-END-TRACKING-VIEW.md)。

## 现象（实测，钉 `8197fa2f`，只记 `S`）

有效交付登记、PS 终局 `EFFECTIVE_DELIVERY` 形成之后，VE 派生了新投影（带 `DELIVERED` 与 `final-outcome-adopted` 两条），客户视图随之出新版本，终局一维仍是 `PENDING_CONFIRMATION`；里程碑与 ETA 两维 `SHOWN`，说明 `NOT_DISCLOSED`。

成因：种子 `data/visibility/07-disclosure-policy-v1.json`（`SYN-VE-DISCLOSE-V1`）给两户的终局都登了 `PENDING_CONFIRMATION`；`DeriveCustomerViewHandler` 经 `dimensionsFrom` 逐维照策略取，不看投影里有没有终局。

## 三类

- 租户取值：演示租户对 `SYN-ACCOUNT-01` 的终局披露。种子以新一版策略（生效时点晚于 v1）把这一户的终局改成 `SHOWN` 并给内容来处；`SYN-ACCOUNT-02` 保留待确认，留一户作对比。证据记 `S`。
- 机制（要裁，见下）：策略答 `SHOWN` 而投影里还没有终局时，终局一维答什么。
- 产品策略：无。

## 完成判据

- 灌种子后，演示包裹有效交付之后客户视图的终局一维为 `SHOWN`；`SYN-ACCOUNT-02` 照旧待确认。
- v1 原样在册，版本轴可见。

## 要裁的

- 今天视图只看策略：策略一旦对终局答 `SHOWN`，包裹还在途时终局一维也是 `SHOWN`。改成「策略允许且投影里已有终局才展示，否则待确认」是 VE 的判法，归 VE owner；若裁定要改，另起机制票，本票只做种子。

## 规模

S（种子）
