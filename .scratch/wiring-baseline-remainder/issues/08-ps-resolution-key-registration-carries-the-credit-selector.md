# PS 登记面补信用二维：含 `CreditPolicyObject` 的登记行要能形成立得起来的键

Category: enhancement
Status: resolved——2026-09-09 16:0x 通道 4（task-18658c56，接续 3；分支 `mcp4-wbr08` 基 main `74ef0da8`，tip 见完成记录）：登记面 / 登记行 / 闭包键 / 登记入口四处齐承载信用二维，含 DSN 自验全绿；进 main 的 SHA 由推送方重放后另记。此前 in-progress——2026-09-09 12:0x 通道 4 认领（task-6eb31b85，分支 `mcp4-wbr08`，基 main `74ef0da8`；PS 迁移号取 0020，0021 已预给 wbr/01）。此前 ready-for-agent——2026-09-09 通道 1 代裁立票（用户授权自决）：ADR-0127 Consequences 点名「PS 登记面欠一格」，二选一（像 `PriceRuleObject` 那样在
登记面拒 / 补两维）**取补两维**。理由：ADR-0127 决定四把 SA 账期分支的授信额度改为从闭包交出的信用依据取，「found=false = 闭包没采用信用政策——租户登记的解析键
没要求这一项，恢复动作是补解析键与正文」——若登记面拒绝 `CreditPolicyObject`，没有任何租户能把这一项登进解析键，ADR-0127 整条路就没有入口；拒是把
机制半边的缺口写成长期事实。PS 地盘（历史归通道 2）
Blocked by: 无（PC 侧 `CreditSelector`（等级 × 费用类型）已在 main，ADR-0127）

## 条目

`internal/parcelshipment/adapters/partycommercial/commercial_resolution_keys.go`：`ResolutionKeyRegistration.validate` 今天放行 `CreditPolicyObject`
而不承载信用二维；含它的登记行经 `FormResolutionKey` 形成的闭包键缺 `CreditSelector`，PC 侧闭包解析对「请求信用依据的键必带 `CreditSelector`」
答`输入未受理`（ADR-0127 决定二）——一个登记得进去、永远立不起来的键。

## 做法（照结算三维的先例，同一文件里已有全套形状）

1. **迁移**（`migrations/parcel_shipment/00NN_resolution_key_credit_selector.sql`，号按目录顺延）：登记表加两列 `credit_level` / `credit_charge_type`（可空），
   加一条 CHECK 镜像「含则必填、不含则必缺」：`required_bases` 含 `CREDIT_POLICY` ⇔ 两列都非空（与 `..._settlement_paired` 同形，命名同族）。
2. **登记面**：`ResolutionKeyRegistration` 加两字段（逐维列出，不收 `pcdomain.CreditSelector` 整个——同文件对结算三维的头注说明了为什么逐维：本登记面不得承载
   闭包该解出的东西；信用二维不含合同维，所以这里逐维只是与结算同形，不是被迫）；`validate` 加 `validateCredit(needsCredit bool)`，判据同 `validateSettlement`：
   含 `CreditPolicyObject` 则两维必填、不含则两维必缺；`AuthorityLevel` 走 `NewAuthorityLevel`（非空；开票时写成「走 PC 的 `Named` 反查（集外拒）」是写错了事实——
   PC `authority_grant.go` 里 `AuthorityLevel` 是 `struct{ requiredValue }`，全包无封闭集、无 `AuthorityLevelNamed`，商业权限等级是租户的版本化业务授权，PC 不预设它有哪几档；
   通道 1 于 2026-09-09 14:1x 核过并裁改此句）、费用类型走 `NewChargeTypeReference`（非空）。
3. **键**：`FormResolutionKey` 加 `creditSelectorFromRow`，两维全缺交零值（「本次不要信用依据」的正常形状），在场则折成 `pcdomain.CreditSelector`。
4. **`PriceRuleObject` 那一支不动**：它拒的理由（价格规则的选择维在本登记面无从表达）与本票无关；本票只把「需要额外维度」清单里的信用那一项从拒改为承载。
5. 测试：登记面单元（含则缺一维拒 / 不含则给了拒 / 齐则键上 `CreditSelector` 在场）；PG 真库（迁移过 CRLF/BOM 哨兵；CHECK 两向各一条）；闭包键往返
   （`FormResolutionKey` 出的键送 PC `ResolveCommercialClosure` 的替身或真实现，请求信用依据时不再答`输入未受理`）。
6. 基线：若 `production_wiring_baseline.txt` / 可达性基线因此变动，按头注纪律记数、钉 SHA。

## 完成判据

含 `CreditPolicyObject` 的登记行能登记且形成的闭包键带 `CreditSelector`；不含时两维必缺；库内 CHECK 与登记面同判据两道镜像；含 DSN 跑
`./internal/parcelshipment/...` 与 `./migrations/`，反向依赖含 `cmd/*` 的包带 DSN 跑；gofmt / vet 0。

## 边界

不动 PC；不动 SA；不动 ADR-0127 正文（本票是它 Consequences 点名的后续，完成记录回指即可，不 supersede）。

## 完成记录

分支 `mcp4-wbr08`，基 main `74ef0da8`（未 rebase；远端 main 此刻 `56ed4111`，awf/21 与 awf/24 动的是 PC 与 admin-web，与本分支触及文件无重叠）。三任会话同一分支往上做（12:0x 认领；14:0x、15:0x 两次全通道中断后接续，接手方按 parallel-sessions「接手别人在途产出」先自列判据再对照 diff）。下表 SHA 为分支上的，作封存出处；进 main 的 SHA 由推送方重放后另记。

| 分支 SHA | 内容 |
|---|---|
| `fe7c7253` | docs(scratch)：票面转 in-progress，PS 迁移号取 0020 |
| `920bab06` | feat：`/tdd` 第一层（登记面单元）——`ResolutionKeyRegistration` 加 `CreditLevel` / `CreditChargeType`（逐维列出，与结算三维同形，不收 `pcdomain.CreditSelector` 整个）；`validateCredit` 守「含则必填、不含则必缺」，与 `validateSettlement` 只差没有「要信用就要合同」那一条——信用政策没有由闭包解出的一维。票面做法 2 的 `AuthorityLevel` 那半句改为 `NewAuthorityLevel`（非空，通道 1 14:1x 裁）。`TestKeyRegistrationRefusesDefaultsAndBareCalls` 新增信用政策要两维 / 两维缺一 / 不要信用却带维度三格 |
| `821282dc` | feat：第二层（PG 真库）+ 第三层（闭包键往返）——迁移 `0020_resolution_key_credit_selector.sql` 加 `credit_level` / `credit_charge_type`（可空）、`..._credit_paired`（一条整体 CHECK 镜像登记面判据）与 `..._credit_not_blank`（空串冒充在场）；store INSERT / SELECT 带两列、缺席写 NULL 不写空串、重放比对把两维一并比（换维判`内容冲突`）；`ResolutionKeyRow` 加两字段；`FormResolutionKey` 经 `creditSelectorFromRow` 折成 `pcdomain.CreditSelector`，两维全缺交零值。新增 `TestARegisteredCreditBasisFormsATwoDimensionSelector` / `TestChangingACreditDimensionIsAContentConflict` / `TestTheDatabaseMirrorsTheCreditPairingRule` / `TestACreditKeyResolvesAgainstTheClosure`。两层合一笔：登记行 ↔ 登记面往返本身就要 `creditSelectorFromRow`，拆开中间那笔会红 |
| `9e509489` | feat：登记入口——`cmd/parcel-commercial/translate.go` 的 `resolutionKeyDocument` 加 `credit` 节（`level` × `chargeType`），`keyRegistrationFromJSON` 照 settlement 节先例逐维过 `NewAuthorityLevel` / `NewChargeTypeReference`；缺席整节即两维全缺，给了节却少一维在翻译层响亮失败、不折成缺席。新增 `TestCreditSelectorTranslatesAsTwoDimensions`（拒收：带法人维 / 两维缺一 / 两维写成空串）。接续 3 对判据自列时发现的漏，见下「地盘之外多的一处」 |
| （本笔） | docs(scratch)：Status → resolved + 本完成记录 |

**逐条对完成判据**：

1. 「含 `CreditPolicyObject` 的登记行能登记且形成的闭包键带 `CreditSelector`」——`TestARegisteredCreditBasisFormsATwoDimensionSelector`（真库：`Register` 落行 → `FormResolutionKey` 读回；`MinimumIdentityEstablished` 成立、两维读回不变形、`RequiredBases` 含信用政策、结算维不被顺手带上）；`TestACreditKeyResolvesAgainstTheClosure`（同一把键送 PC 真实现 `ResolveCommercialClosure`：`UNIQUELY_RESOLVED`、信用依据被采用、额度出自命中的那份政策正文）。
2. 「不含时两维必缺」——登记面 `TestKeyRegistrationRefusesDefaultsAndBareCalls/不要信用却带维度`；库内 `TestTheDatabaseMirrorsTheCreditPairingRule/不要信用却带维度`（拒它的必须是 `..._credit_paired`，换成别的错不算守住）；翻译层 `credit` 节缺席即两维零值。
3. 「库内 CHECK 与登记面同判据两道镜像」——登记面三格（信用政策要两维 / 两维缺一 / 不要信用却带维度）与库内五格（要信用却两维缺一 / 全缺 / 不要信用却带维度 / 维度写成空串 / 齐备则放行）逐格对得上；库内那五格绕开登记面直插，断言的是约束名。
4. 「含 DSN 跑 `./internal/parcelshipment/...` 与 `./migrations/`，反向依赖含 `cmd/*` 的包带 DSN 跑」——见验证。
5. 「gofmt / vet 0」——见验证。

**接续单点名的三问**（第三层「闭包键往返」是否已被 `creditSelectorFromRow` 那半覆盖）：

- 真库往返用例——有。`TestARegisteredCreditBasisFormsATwoDimensionSelector` 与 `TestChangingACreditDimensionIsAContentConflict` 都经 `pgtest.Pool` 真库落行再读回，不是内存替身。
- 闭包键消费侧——有。`TestACreditKeyResolvesAgainstTheClosure` 用的是 PC 真实现 `pcdomain.ResolveCommercialClosure`，不是替身；生产消费侧 `commercial_basis.go` 拿的是整把 `pcdomain.ClosureResolutionKey`，`Credit` 是键上既有的一维（ADR-0127 决定二），无需改动。
- 登记面 ↔ 登记行 ↔ 闭包键三处同形——是；但**第四处不同形**：唯一构造 `ResolutionKeyRegistration` 的生产入口 `register-resolution-key`，其文档面只有 `settlement` 节、没有 `credit` 节——含 `CREDIT_POLICY` 的登记文档在进程口无从携带两维，`validateCredit` 必拒，票面理由那句「没有任何租户能把这一项登进解析键」在进程口仍然成立。已补（`9e509489`）。

**地盘之外多的一处**（占号广播 15:5x 已点名）：`cmd/parcel-commercial/translate.go` + `translate_test.go`——只加 `credit` 节与翻译一段、一条测试，不改既有签名；不碰同目录 `publish_batch_test.go`（通道 3 在途）。原单地盘写「只动 adapters/partycommercial 与 PS postgres 那一侧」是为与通道 2 不相撞，不是判定入口不在票内。`scripts/demo-seeds` 的 `resolution-key-syn-account-01.json` 不动：它示范四项必需依据加结算三维，没要信用依据。

**验证**（树 `D:/tops/idp-parcel-mcp4-wbr08`，tip `9e509489`，工作树干净）：

- `gofmt -l .` 空；`go build ./...` / `go vet ./...` 退 0。
- 反向依赖（`go list` 全仓 `Deps` + `TestImports` + `XTestImports` 反查 `adapters/partycommercial`、`adapters/postgres`、`cmd/parcel-commercial` 三包）：`cmd/parcel-api`、`cmd/parcel-commercial`、`cmd/parcel-dispatch`、`tests/bentocontract`，加 PS 自己的 `nodeoperations` / `transportfulfillment`（已在 `./internal/parcelshipment/...` 内）。
- 含 DSN（55432 门禁库；通道 1 15:57 广播窗口关后跑）`go test -count=1 -v ./internal/parcelshipment/... ./migrations/ ./internal/architecture/... ./cmd/parcel-api/ ./cmd/parcel-commercial/ ./cmd/parcel-dispatch/ ./tests/bentocontract/`：exit 0，**PASS 1990 / FAIL 0 / SKIP 0**，22 包 ok / 2 无测试（`adoptconsume`、`ports`），15:58:26→15:59:15；`adapters/postgres` 48.4s、`cmd/parcel-dispatch` 29.4s、`cmd/parcel-api` 16.0s、`adapters/partycommercial` 5.0s——真 PG 跑过。信用相关用例（登记面三格、真库往返两条、库内镜像五格、闭包往返一条、翻译一条三格）在日志里逐条 PASS。日志 `%TEMP%\mcp4-wbr08-dsn.log`。
- 架构门禁 `./internal/architecture/...` 有 / 无 DSN 各一次 ok：`production_wiring_baseline.txt` / `production_type_reachability_baseline.txt` 未漂——本票只给既有类型加字段、给既有入口加一节，不新增生产类型、不接新缝，做法 6 无事可记。
- 红一次的证据：第一层见 `920bab06` 提交说明；第二 / 三层把 `creditSelectorFromRow` 临时短路成恒交零值时往返用例报最小身份不成立、闭包用例报 `INPUT_NOT_ACCEPTED`（`821282dc` 提交说明）；入口一层加节前 `TestCreditSelectorTranslatesAsTwoDimensions` 报 `json: unknown field "credit"`。

**红线自查**：不动 PC（`internal/partycommercial/**` 零改动）；不动 SA；不动 ADR-0127 正文；`PriceRuleObject` 那一支照旧拒（`TestKeyRegistrationRefusesDefaultsAndBareCalls/价格规则仍不承载` 仍在）；注释中文，跨文件引用皆符号名 / 约束名 / ADR 号，无行号无计数；两维无默认——登记面、库内、翻译层三处都是「含则必填、不含则必缺」，空串在库内 `..._credit_not_blank` 与翻译层构造门各拦一道；等级与费用类型的取值全由夹具给（`level-commercial` / `charge-freight` 是合成 `S`）。

**判断题**（供评审）：

1. 信用节 JSON 字段名取 `level` / `chargeType` 而非 `authorityLevel`——照 `pcdomain.CreditSelector.Level` 与登记行 `credit_level` 的同族命名；若 owner 要与 PC 发布批里信用政策正文的字段名对齐，改的是一个 tag，测试字面跟着改。
2. `creditSelectorFromRow` 只在两维全缺时交零值，一维在场一维缺席时交构造门的错——库内 `..._credit_paired` 已让这种行进不了库，这里是第三道镜像的读回半边；保留是为了「库被绕开」时不静默折成缺席。
3. 两层合一笔（`821282dc`）是前一任的取舍，接手方核过 diff 原样提交；理由是拆开中间那笔会红。
4. 未 rebase 到 `56ed4111`：与在途 main 无文件重叠，推送方重放即可。
5. 接手方通道 4 是 `9e509489` 的作者、`920bab06` / `821282dc` 的非作者读者，未跑 `/code-review` 两轴——语义评审仍靠非作者通道。

## 进 main 记录（2026-09-09 17:3x，通道 1 推送）

分支 `mcp4-wbr08` 五笔在隔离树重放到 `3c9a41bd` 之后（链上前有 adle/02 三笔），零冲突、内容与分支逐文件零差：`fe7c7253→c587a66b` / `920bab06→9661ce3b` / `821282dc→42d84aaa` / `9e509489→678fd8f6` / `10e90845→8dad9960`。清点在链 tip 重生成 `62e19b1b`（parcelshipment 迁移 19→20，合计 154→155；与 wbr/01 同笔清点）。推送方在 `62e19b1b` 干净检出含 DSN `go test -p 1 -count=1 ./...` 一次：101 ok / 0 FAIL / 16 无测试，568 s；探针 `TestACreditKeyResolvesAgainstTheClosure` 含 DSN PASS / 无 DSN SKIP（评审 Spec 非阻断 ① 要的非作者含 DSN 复跑即此）。**远端 `main = 62e19b1b`**。分支指针改名 `merged/mcp4-wbr08`。

## Comments

**评审 ← 通道 2 · 钉 `10e90845` · 17:04**（基 `74ef0da8`，隔离树 `%TEMP%\idp-review-wbr08`；原文在通道 1 台账 `task-1a9f4f2c`）

- **Standards**：阻断 0。非阻断（判断项）① `validateCredit` / `creditSelectorFromRow` 与 `validateSettlement` / `settlementSelectorFromRow` 同形——Duplicated Code 判断项，但票面做法明写「照结算三维先例同形」，仓规优先不作违规；第三个选择器出现时再抽 `countGiven`。② `translate_test.go` `TestCreditSelectorTranslatesAsTwoDimensions` 只测「含」向，「credit 节缺席 ⇒ 两维零值」无断言——由 `*creditSelectorDocument == nil` 结构保证且 `validateCredit` 兜底，可选补格。无发现：注释全中文；跨文件引用皆符号 / 约束名 / ADR 号；adapters 非测试文件未新增 import，pgconn 只进 `_test.go`；0020 无 BOM、CR = 0；CHECK 两向 + `_not_blank` 与 store `nullableText` 写 NULL 一致；三层皆「含则必填、不含则必缺」无默认。gofmt 空、build/vet 0。
- **Spec**：阻断 0。非阻断 ① 真库与闭包往返用例（`TestARegisteredCreditBasisFormsATwoDimensionSelector` / `TestChangingACreditDimensionIsAContentConflict` / `TestTheDatabaseMirrorsTheCreditPairingRule` / `TestACreditKeyResolvesAgainstTheClosure` / `TestKeyRegistrationRefusesDefaultsAndBareCalls`）无 DSN 全 SKIP（评审处 11 SKIP / 58 PASS），作者 1990/0/0 未经非作者复跑——建议推送方带 DSN 复跑（已在进 main 记录里做）。② `cmd/parcel-commercial/translate.go` `credit` 节不在做法 1–6，但 `register-resolution-key` 是唯一构造 `ResolutionKeyRegistration` 的生产入口，不加则完成判据 1 在进程口不可达；判为票内必要非蔓延。无发现：做法 1（两列可空、`_credit_paired` ⇔ 镜像、`_credit_not_blank`）、做法 2（逐维、`NewAuthorityLevel` 非空、不收整个 `CreditSelector`）、做法 3 与 PC `reference_closure.go` `minimumIdentityEstablished` 同判据、做法 4 `PriceRuleObject` 仍拒、做法 6 `internal/architecture/` 零改动；边界 PC / SA / ADR-0127 正文零改动；判断题 2 与 PC「部分给出即输入未受理」一致。
- **结论**：可推送。推送方处置：非阻断两条随票记，不另立票（① 等第三个选择器；② 作者可选补）。
