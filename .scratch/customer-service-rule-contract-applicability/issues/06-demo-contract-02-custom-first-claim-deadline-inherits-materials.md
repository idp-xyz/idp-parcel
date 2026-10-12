# 06 演示对比面：SYN-CONTRACT-02 挂合同规则只改首次索赔时限，账户 02 索赔答定制时限、材料继承产品版

Category: enhancement
Status: 完工，待重放——2026-10-12 通道 1 在分支 `mcp1-csr06b`（基 `c6c65052`）上做完，分支已推 origin，完成记录见文末。此前：in-progress——2026-10-12 11:3x 通道 1 续做（07 已进 main `c6c65052`）：分支 `mcp1-csr06b` 基 `c6c65052`，种子笔 `2b76eab4` cherry-pick 过来，旧分支 `mcp1-csr06` 的认领笔不带（与 main 上的票面改动冲突）。此前：blocked——2026-10-12 10:5x 通道 1：种子子集在分支 `mcp1-csr06`（基 `8c41be83`，认领 `dfebdb5c`，种子 `2b76eab4`，已推 origin）上做完并灌通干净库，但同范围多一份合同后两户闭包都在 `CUSTOMER_CONTRACT` 上`适用冲突`、账户 01 回退，种子不进 main，见文末「灌库取证」。此前：in-progress（分支上认领）；ready-for-agent——2026-10-10 通道 1 发布：拆法作者通道 3（`task-2b404e22`），通道 1 经用户 19:1x 授权认可并裁定拆法清单所附七问
Blocked by: 无（[04](04-ve-claim-eligibility-checks-contract-and-inherits-rows-from-product-base.md)、[07](07-closure-does-not-narrow-customer-contract-by-customer-account.md) 均已解）
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

- [x] 第二客户的商业主体在 main 的种子里。
- [x] 账户 02 索赔答定制时限、材料继承产品版；账户 01 答复不变（`S`）——以闭包与资格读面的答复取证，见完成记录；演示仍停在已提交。

## 开工取证（2026-10-12 10:3x，通道 1，钉 main `80558703`；只读，未动种子，未认领）

做什么 1 要先判封存分支 `salvage/demo-seeds-syn-account-02-wip` 能不能当起点。量法：`git diff ed662238 86eb081f`（`ed662238` 是它与 main 的合并基）。

- **范围比本票宽。** 那一笔动了 10 个文件：本票要的第二客户商业主体（`register-parties.json` 的 `SYN-PARTY-SHIPPER-02`、`SYN-ACCOUNT-02`、`SYN-REL-CUSTOMER-02`，`publish-batch.json` 的 `SYN-CONTRACT-02` 与 `SYN-SETTLEMENT-TERMS-02`，新 `resolution-key-syn-account-02.json`）之外，还带了账期信控（`SYN-FIN-CONTROL-TERMS-01`、`SYN-CREDIT-TERMS-01`）、两张 SGD 价卡、一本 SGD 分户账、账户 02 的 `CUSTOMS_HOLD` 披露规则与合同 02 的索赔资格册——后几样属计价、代收、VE 的演示面，不是本票完成判据点名的东西。
- **没跑过。** 五条新发布项的 `contentDigest`（`PCC-1:…`）是写进 JSON 的值，封存提交信写明「一字未改、非集成候选」；没有一次 `seed.sh` 灌库的记录证明它们与发布批重算的摘要相等。
- **与 main 分叉。** 合并基之后 main 上 `scripts/demo-seeds` 另有三笔：`fd536468`（网络目录改采参考配置）、`b35c821a`（规则包受理前财务控制格采 `submission-receipt@1`）、`c38cc751`（`seed.sh` 登应收结算账户）。后两笔与封存那笔的账期信控、`seed.sh` 同段相邻，重放要逐块核。
- **注释已过时。** 封存 `seed.sh` 里那段写「『按客户定制时限/材料』今天册上没有适用维，专属诉求另走领域建模」——ADR-0176 与本 spec 02–04 已让它有路，照原样进 main 会留一段与代码相反的话。
- **两户的解析键都要列 `CUSTOMER_SERVICE_RULE`**：封存那笔已给账户 01 的键补了这一类、给账户 02 的新键也列了，这一半正是本票走通所需。

据此建议：**不整笔重放，按本票要的子集另起**——第二客户商业主体 + 两户解析键列 `CUSTOMER_SERVICE_RULE` + 一版壳指名 `SYN-CONTRACT-02`、正文只登首次索赔期限的客户服务规则，注释按 ADR-0176 重写；封存那笔的计价 / 代收 / VE 演示面另立票或随各自切片。账户 02 的解析键要 `SETTLEMENT_POLICY`，所以 `SYN-SETTLEMENT-TERMS-02` 与它引的合同 02 绑定（信控政策）要一并带上，带到哪一格为止，开工时以 `seed.sh` 对干净演示库灌通为准、不凭读 JSON 定。走通（做什么 3）要账户 02 的一票委托真接受再提一封索赔，前提是 `submit-one-shipment.sh` 能按账户 02 提交——未核。

## 灌库取证（2026-10-12 10:5x，通道 1，分支 `mcp1-csr06` 的 `2b76eab4`）

**种子子集**（照「开工取证」的建议另起，未重放封存那笔）：参与方登记加 `SYN-PARTY-SHIPPER-02`、`SYN-ACCOUNT-02`、`SYN-REL-CUSTOMER-02`；发布批加 `SYN-CONTRACT-02`（照合同 01 的形状，预付绑 `SYN-FIN-CONTROL-01`，委派人账户 02）、`SYN-SETTLEMENT-PREPAID-02`（相对方货主二号、合同 02、CNY）、`SYN-SERVICE-RULE-CONTRACT-02`（壳指名合同 02，正文只登首次索赔 7 天）；两户解析键都列 `CUSTOMER_SERVICE_RULE`；`seed.sh` 登账户 02 的键并按 ADR-0176 写注释。三项新发布的 `contentDigest` 是先放占位值、由发布批答 `NOT_ACCEPTED` 时交回的算出摘要。

**灌库**：一只新建的空库 `seed.sh` 退 0，发布批 17 项全收。

**闭包**：一次性探针（未提交，跑完即删）按两户解析键成键、交 PC 真解析器解闭包，两户都答 `APPLICABILITY_CONFLICT`，冲突依据 `SERVICE_PRODUCT` 与 `CUSTOMER_CONTRACT`；对 main 原样种子解账户 01 只冲突在 `SERVICE_PRODUCT`（探针没带声明的服务产品，真提交会带）。多出来的 `CUSTOMER_CONTRACT` 是第二份合同带进来的：闭包不按客户账户选合同，立为 [07](07-closure-does-not-narrow-customer-contract-by-customer-account.md)。

**因此**：完成判据一（第二客户商业主体进 main）照原样做会让账户 01 的闭包回退，违反判据二的「账户 01 答复不变」，本票停在这里。07 定了合同按客户账户怎么选之后，分支上的种子按那一选法补齐（多半要给合同 01 / 02 各补一处归属）再灌、再走做什么 3。做什么 3 另有一条与 07 无关的前提仍未核：演示今天停在已提交，没有一步把委托推到已接受（`README` 「运行时只多一步」那段），索赔读的又是接受时固定的闭包——走通要么补那一步，要么只以闭包与层次读口的答复作 `S` 证据，到时再定。

## 完成记录（2026-10-12，通道 1，分支 `mcp1-csr06b` 基 `c6c65052`）

**落点**：`7ef3f934` 是旧分支 `mcp1-csr06` 那一笔种子 `2b76eab4` 的 cherry-pick（提交信里「不进 main」写于 07 未解之时，那时为真；07 解后本笔就是要进 main 的种子）；`6afca6cd` 续做认领；`0bf704a3` 合同 02 正文补 `customerAccount: SYN-ACCOUNT-02`、摘要换成发布批重算值（先放占位值灌一次取 `NOT_ACCEPTED` 交回的算出串），README 改两处过时的话（解析键 1 行 → 2 行、「唯一一份结算约定」→ 两户各一份）并补对比面一段。旧分支 `mcp1-csr06` 的认领笔 `dfebdb5c` 不带：与 main 上本票后来的票面改动冲突，内容已被本票票面覆盖。

**灌库**：新建空库 `seed.sh` 退 0，发布批 17 项全收。

**走通（S）**：一次性探针（未提交，跑完即删、库已删）对两户各做一遍：PS 真解析键登记面 `FormResolutionKey` 成键、带上委托会声明的服务产品 `SYN-PROD-CN-SG-EXPRESS`，交 PC 真 `ResolveCommercialBasisHandler` 解闭包并固定进真解析库；再用**生产的** VE 适配器 `vepartycommercial.ClaimServiceRules`（真闭包读口、真正文点读口、真层次读口）读一项 `SYN-CLAIM-LOSS` 索赔的资格两维。两处替身：PS 回指读口换成「答这份闭包的解析标识」——演示今天停在已提交、没有一笔已接受委托可回指；VE 自己的册换成「声明在场、类型在保」——那三维与本票无关。实测：

| 账户 | 闭包 | 采纳的客户合同 / 客户服务规则 | 首次索赔期限维 | 最低材料维 |
|---|---|---|---|---|
| `SYN-ACCOUNT-01` | 唯一解析 | `SYN-CONTRACT-01` / `SYN-SERVICE-RULE-01` | 已登记，冻 `SYN-SERVICE-RULE-01/v1`（产品版 14 天） | 已登记，冻 `SYN-SERVICE-RULE-01/v1`，`SYN-DOC-POD-01` |
| `SYN-ACCOUNT-02` | 唯一解析 | `SYN-CONTRACT-02` / `SYN-SERVICE-RULE-CONTRACT-02` | 已登记，冻 `SYN-SERVICE-RULE-CONTRACT-02/v1`（合同版 7 天） | 已登记，冻 `SYN-SERVICE-RULE-01/v1`（继承产品版），`SYN-DOC-POD-01` |

账户 01 与 06 之前同一答法（产品版两维）；账户 02 期限读合同版、材料继承产品版。VE 的期限维只冻版本引用与起算事件、日历，天数在规则正文里（`7` 与 `14` 取自种子），截止时刻今天仍留零值（ve-claims-read-seams/03 既有口径）。

**门**：改动只有种子 JSON、`seed.sh` 与 README（加票面）。`cmd/parcel-commercial` 读发布批的那条用例（`TestDemoSeedClearsTheCitationGateAndItsDeclaredDigests`）照绿；全量留给推送方那一跑。证据层级 S。

**判断项**

1. **第二客户照合同 01 的形状配（预付、CNY、同一信控政策），不带封存那笔的账期信控、SGD 价卡、分户账与披露规则**——见「开工取证」：那几样是计价、代收、VE 的演示面，不是本票判据点名的东西，要做另立票。
2. **合同版只登首次索赔期限一行、取 7 天**：比产品版 14 天更严，符合 ADR-0176 决定三「只收紧」的商务纪律（系统不比对，取值是演示租户的 S 实例）。
3. **两户的解析键都列 `CUSTOMER_SERVICE_RULE`**：不列，闭包不采用任何客户服务规则版本，两维恒答未登记，对比面无从谈起。账户 01 的答复因此从「未登记」变成「产品版」——这是「账户 01 不受合同 02 影响」的那一面，不是回退。
4. **走通以探针取证而不补「接受」那一步**：演示停在已提交是 README 写明的现状，补一条接受链是另一件事；探针里除 PS 回指与 VE 自己的册外都是生产件，两处替身已在上面点名。
