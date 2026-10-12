# 06 演示对比面：SYN-CONTRACT-02 挂合同规则只改首次索赔时限，账户 02 索赔答定制时限、材料继承产品版

Category: enhancement
Status: in-progress——2026-10-12 10:4x 通道 1 认领（用户令独立完成），分支 `mcp1-csr06`，基 `8c41be83`；按「开工取证」节的建议另起子集，不重放封存那笔。此前：ready-for-agent——2026-10-10 通道 1 发布：拆法作者通道 3（`task-2b404e22`），通道 1 经用户 19:1x 授权认可并裁定拆法清单所附七问
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

## 开工取证（2026-10-12 10:3x，通道 1，钉 main `80558703`；只读，未动种子，未认领）

做什么 1 要先判封存分支 `salvage/demo-seeds-syn-account-02-wip` 能不能当起点。量法：`git diff ed662238 86eb081f`（`ed662238` 是它与 main 的合并基）。

- **范围比本票宽。** 那一笔动了 10 个文件：本票要的第二客户商业主体（`register-parties.json` 的 `SYN-PARTY-SHIPPER-02`、`SYN-ACCOUNT-02`、`SYN-REL-CUSTOMER-02`，`publish-batch.json` 的 `SYN-CONTRACT-02` 与 `SYN-SETTLEMENT-TERMS-02`，新 `resolution-key-syn-account-02.json`）之外，还带了账期信控（`SYN-FIN-CONTROL-TERMS-01`、`SYN-CREDIT-TERMS-01`）、两张 SGD 价卡、一本 SGD 分户账、账户 02 的 `CUSTOMS_HOLD` 披露规则与合同 02 的索赔资格册——后几样属计价、代收、VE 的演示面，不是本票完成判据点名的东西。
- **没跑过。** 五条新发布项的 `contentDigest`（`PCC-1:…`）是写进 JSON 的值，封存提交信写明「一字未改、非集成候选」；没有一次 `seed.sh` 灌库的记录证明它们与发布批重算的摘要相等。
- **与 main 分叉。** 合并基之后 main 上 `scripts/demo-seeds` 另有三笔：`fd536468`（网络目录改采参考配置）、`b35c821a`（规则包受理前财务控制格采 `submission-receipt@1`）、`c38cc751`（`seed.sh` 登应收结算账户）。后两笔与封存那笔的账期信控、`seed.sh` 同段相邻，重放要逐块核。
- **注释已过时。** 封存 `seed.sh` 里那段写「『按客户定制时限/材料』今天册上没有适用维，专属诉求另走领域建模」——ADR-0176 与本 spec 02–04 已让它有路，照原样进 main 会留一段与代码相反的话。
- **两户的解析键都要列 `CUSTOMER_SERVICE_RULE`**：封存那笔已给账户 01 的键补了这一类、给账户 02 的新键也列了，这一半正是本票走通所需。

据此建议：**不整笔重放，按本票要的子集另起**——第二客户商业主体 + 两户解析键列 `CUSTOMER_SERVICE_RULE` + 一版壳指名 `SYN-CONTRACT-02`、正文只登首次索赔期限的客户服务规则，注释按 ADR-0176 重写；封存那笔的计价 / 代收 / VE 演示面另立票或随各自切片。账户 02 的解析键要 `SETTLEMENT_POLICY`，所以 `SYN-SETTLEMENT-TERMS-02` 与它引的合同 02 绑定（信控政策）要一并带上，带到哪一格为止，开工时以 `seed.sh` 对干净演示库灌通为准、不凭读 JSON 定。走通（做什么 3）要账户 02 的一票委托真接受再提一封索赔，前提是 `submit-one-shipment.sh` 能按账户 02 提交——未核。
