# 06 演示对比面：SYN-CONTRACT-02 挂合同规则只改首次索赔时限，账户 02 索赔答定制时限、材料继承产品版

Category: enhancement
Status: draft——2026-10-10 通道 1 发布：拆法作者通道 3（`task-2b404e22`），通道 1 经用户 19:1x 授权认可并裁定拆法清单所附七问
Blocked by: [04](04-ve-claim-eligibility-checks-contract-and-inherits-rows-from-product-base.md)
父票：[spec](../spec.md)
地盘：`scripts/demo-seeds` 与取证处；碰种子与真库取证，走并行会话那条路。
出处：spec「演示对比面」；建模票 [01](01-service-rule-keyed-by-product-cannot-express-per-customer-claim-terms.md)「来处」。

## 做什么

1. 第一步先补前提：main 的演示种子里 `SYN-ACCOUNT-02` 只出现在 VE 数据（索赔授权空表、披露策略），第二客户的商业主体（账户、合同等）在本地封存分支 `salvage/demo-seeds-syn-account-02-wip`（`86eb081f`，未进 main、未推 origin）。以那一笔为起点先核是否完整、落 main，或者另起；票面写清走了哪条。
2. 种子加一版挂 `SYN-CONTRACT-02` 的客户服务规则，只改首次索赔时限。
3. 走通一次：账户 02 的索赔答定制时限、材料继承产品版；账户 01 不受影响。证据只记 `S`。

## 判断项（拆票时裁定）

- **单独一张（Q7）**：不并进 04，让 `S` 证据不混进 04 的完成判据。

## 完成判据

- [ ] 第二客户的商业主体在 main 的种子里。
- [ ] 账户 02 索赔答定制时限、材料继承产品版；账户 01 答复不变（`S`）。
