# ADR-0352：登记方案记着基础版之外规范化版本的价卡，要部署显式打开一个开关，默认关；门在价卡登记用例里、仓储之前，登记口、草稿发布与受控 CLI 共用这一道；开关只管登记、不管读，关着时同一版的重放也拒

Status: Accepted（2026-11-01，Claude Code 会话代 PP owner 与运维决（代决）：起因是冻结清单 2026-10-29 两条「登记 PPC-6 价卡前要求显式部署开关」，后一条因价卡导入能造出 `PPC-6` 而升级；设计由协调方派单、作者落笔，用户未逐条过目，越权风险点待 PP owner 复核，开关语义与发布顺序另请运维过目。部分停用 [ADR-0348](./0348-a-price-card-may-declare-total-charge-bounds-and-key-a-surcharge-table-by-remote-tier-and-plans-using-them-are-written-as-ppc-6.md) 越权风险点 6 关于「代码里没有门」的几句；部分停用 [ADR-0101](./0101-operator-facing-registration-payload-shape-is-product-defined.md) 决定五一句（答案代数六格的列举）；回答 [ADR-0349](./0349-the-price-card-import-template-advances-to-ppt-2-and-its-structured-workbook-is-a-second-carrier-read-through-the-same-path.md) 越权风险点 5，无条款停用。）<!-- 索引注记 -->｜**判据**：价卡登记用例在交仓储之前看方案记的规范化版本，不是基础版（`CurrentCanonicalizationVersion()`，今天是 `PPC-5`）且部署没把 `IDP_PARCEL_PRICING_REGISTER_NEWER_CANONICALIZATION` 设成恰为 `true` 时，答登记结果代数新增的一格 `NEWER_CANONICALIZATION_NOT_ENABLED`，不写登记册、不发「参数已登记」交接；登记口、草稿发布（交既有登记用例）与受控 CLI 都经这一道门，`parcel-api` 与 `parcel-pricing-register` 各在装配读一次环境；基础版的卡不受影响；门不读库，已在册的同一版重放在开关关着时也拒；开关只管登记，不管任何读价卡的路径；发布顺序是先升级所有读价卡的进程、再开开关

Date: 2026-11-01

## Context

取证于 main `5907d4cde`，只记 `S`。

- [ADR-0348](./0348-a-price-card-may-declare-total-charge-bounds-and-key-a-surcharge-table-by-remote-tier-and-plans-using-them-are-written-as-ppc-6.md) 决定四让带总额上下限或按偏远档位查表的方案写 `PPC-6`，越权风险点 6 把只认 `PPC-5` 的旧进程遇到 `PPC-6` 价卡的后果逐路写明：试算整次答价卡读不出，评价与售价预估在唯一适用的卡是 `PPC-6` 时答 unavailable，序列与目录登记 / 复核之后的受影响范围扫描是租户级的、整笔失败并波及 `parcel-pricing-feed` 的周更燃油序列。它把「第一张 `PPC-6` 价卡登记之前，所有读价卡的进程必须已是新构建」写成文档约束，并把开关列为后续加固。
- [ADR-0349](./0349-the-price-card-import-template-advances-to-ppt-2-and-its-structured-workbook-is-a-second-carrier-read-through-the-same-path.md) 之后，导入能造出 `PPC-6` 的卡：任何持登记写授予的操作者都能经导入、批准、发布走到登记这一步（其越权风险点 5）。管理台的价卡编辑器给租户用之前，这道门要在代码里。
- 价卡进登记册只有一条路：`RegisterPriceCardHandler.Handle` 调仓储的 `Register`。在线登记口 `/pricing-price-card-registrations`、草稿发布口 `/pricing-price-card-draft-publications`（`PublishPriceCardDraftHandler` 收的是具体的登记用例，ADR-0101 决定五）与受控 CLI `cmd/parcel-pricing-register` 都经它，别处没有写价卡行的代码（取证于 `5907d4cde`）。
- 「所有读价卡的进程都已是新构建」不是产品能从库里或从自身判断出来的事实：读价卡的进程有 `parcel-api`、`parcel-dispatch` 与各登记 CLI，滚动发布时新旧构建并存多久、哪台机器上跑着哪个构建，只有部署方知道。

## Decision

**一、门在价卡登记用例里，一处。**（机制）

- 门放在 `RegisterPriceCardHandler.Handle`：零值受理门与交接口装配检查之后、仓储 `Register` 之前。登记口、草稿发布与受控 CLI 都经这里，不在各入口各挡一次——各挡一次就是每个入口各一份判断，日后新增入口时漏装一份编译仍绿。
- 判的是「方案记的规范化版本不等于基础版」，基础版即 `domain.CurrentCanonicalizationVersion()`。今天挡的就是 `PPC-6`；将来再有更新的版本同此办，门不用回来改。基础版的卡一律不受影响：字节、摘要、答复与此前逐字相同。

**二、开关是部署参数，默认关，只认恰为 `true`。**（机制）

- 环境变量 `IDP_PARCEL_PRICING_REGISTER_NEWER_CANONICALIZATION`，与 `IDP_PARCEL_GOVERNANCE_SELF_AUTHORITY` 等同属随部署走的参数，命名同风格。去首尾空白后恰为 `true` 才开；缺省、空、`TRUE`、`1`、`yes` 都按关读。开错的代价（旧进程读到新卡整笔失败、波及燃油序列）远大于没开（登记答未启用、可重试），所以宁可多拒。
- `parcel-api` 在 `run` 里读一次、带启动日志（开与不开都出声），经启动参数交给登记口与草稿发布口的装配，同一进程里各口用同一个值；`parcel-pricing-register` 在 `main` 里读一次交给装配。值经 `RegisterPriceCardDeps.NewerCanonicalizationEnabled` 注入应用层，领域包不读环境、不依赖 HTTP 或 `pgx`。
- 开关不是租户取值，也不是产品策略：它是部署方对「这一次部署里所有读价卡的进程都已升级」的断言。产品不替它给默认的「开」。

**三、关着时的答复是登记结果代数的一格，不是错误。**（机制；部分停用 ADR-0101 决定五对答案代数六格的列举）

- 新增 `PriceCardRegistrationNewerCanonicalizationNotEnabled`，对外码 `NEWER_CANONICALIZATION_NOT_ENABLED`，与 `NOT_ACCEPTED`、`UNDECIDED` 并列。它说的是「什么都没写，部署放开后原样重试即可」，与内容冲突（续办靠治理裁决）、未决（登记与否未知）都不同。
- 在线登记口照既有登记答复的形状答 200 带这一码；草稿发布口答 `PUBLICATION_NOT_LANDED` 并把这一码作为登记那一格原样交回。受控 CLI 退出码 `4`，单列而不并进 `2`（治理答案）或 `3`（未决），帮助文字写明怎么开；种子与批量脚本能把它与别的失败分开处理。管理台把这一码译成业务话：这张价卡用了较新的计价结构，当前环境尚未允许登记，先确认所有服务都已升级，再请运维开开关后重试。

**四、门不读库：已在册的同一版重放在开关关着时也拒。**（机制，明示取舍）

- 门只看请求里那张卡记的版本。开关关着时，哪怕同一版已在册（例如开关开着的那次部署登过，后来回滚或关掉了），重放也答未启用，而不是已在册；也就不补发那一封「参数已登记」。
- 取这一边，是为了让门在仓储之前判、不碰登记册：关着的部署里任何较新版本的登记都不读不写那张表，门的行为与库里有什么无关，测试与推理都只看一个布尔量。代价是关着时重放拿不到「已在册」，而那一封交接本来在开关开着的那次已经交过。

**五、草稿发布被门拒时，草稿留在已批准，同一份批准开关打开后可再发布。**（机制）

- 照发布用例对别的登记拒答（冲突、不可比、未受理）的既有处理：答 `PUBLICATION_NOT_LANDED`、登记那一格原样交回、草稿不推进，不另造状态。开关打开后对同一份草稿再发布即落定，不用重批。

**六、开关只管登记，不管读；发布顺序与回滚。**（运维约束）

- 开关不影响任何读价卡的路径：已在册的 `PPC-6` 卡在开关关着的部署里照常被试算、评价、预估与受影响范围扫描读到——新构建两版都认（ADR-0348 决定四）。关掉开关不会让已登记的卡消失或改变答复。
- **发布顺序**：先把所有读价卡的进程（`parcel-api`、`parcel-dispatch`、各登记 CLI 与其他按价卡评价的消费者）升级到认这一版的构建，确认没有旧构建还在跑，再在登记价卡的进程（`parcel-api`，或跑受控 CLI 的那一次）上设开关。
- **回滚**：回滚到只认 `PPC-5` 的旧构建之后，已在册的 `PPC-6` 卡仍按 ADR-0348 越权风险点 6 所写的那几种答复出错即停（不会算错金额），租户级的序列与目录登记 / 复核会整笔失败。开关不能防这一格：它只拦住新的登记，不能把已在册的卡变回旧构建认得的样子。所以开开关之前要确认这一版不再回滚，或接受回滚时先停用那些卡（价卡收尾，[ADR-0234](./0234-an-older-price-card-version-leaves-new-selections-only-through-an-explicit-closure-for-a-named-successor.md)）。
- 演示：`scripts/demo-seeds` 只在登记 `PPC-6` 演示卡的那一次 CLI 调用设开关；`scripts/demo-seeds/demo-run/up.sh` 起的进程出自同一次编译，设开关。共享预览环境的进程由协调方自己配。

## Alternatives considered

- **在每个入口各挡一次（登记口、草稿发布、受控 CLI）。** 否决：每个入口各一份判断，日后新入口漏装一份编译仍绿；登记用例本来就是这些入口唯一的汇点。
- **门放在仓储里、开关关着时已在册的同一版答已在册。** 否决：门就要读库，关着的部署里较新版本的登记会碰登记册；为一个只在回滚 / 关开关之后才出现的情形，让门的行为依赖库里有什么，不值。
- **开关写死成「允许 `PPC-6`」。** 否决：再有新版时得回来改门、改名；「基础版之外」一句话覆盖今后各版。代价是将来若要分版放开（只放 `PPC-6`、不放 `PPC-7`），要另立决定把布尔量换成版本集。
- **新造一个错误（HTTP 4xx 或 409）。** 否决：登记答复的既有形状是「200 带结果码」，治理与受理拒答都走结果代数；新造错误会让管理台与 CLI 各多一条处理路径，也会让它看起来像调用方的错。
- **存在库里、由操作者在管理台开。** 否决：这是部署事实，不是租户配置；放进库就成了持某格授予的操作者能替运维断言「所有进程都已升级」。
- **不做门，靠文档约束。** 否决：导入编辑器给租户用之后，操作者经导入、批准、发布就能走到登记，文档约束拦不住（ADR-0349 越权风险点 5）。

## Consequences

- 部署不设开关时，任何方案记着 `PPC-6` 的价卡都登不进来：在线登记口与草稿发布口答 `NEWER_CANONICALIZATION_NOT_ENABLED`，受控 CLI 退 `4`。管理台照译成业务话。基础版的卡照旧。
- 已在册的 `PPC-6` 卡（演示库里的 `SYN-PLAN-CN-SG-FULL-03`）不受影响，读路径一处没改。
- `RegisterPriceCardDeps` 多一格布尔量，零值即关：没传它的装配（含各测试）得到的是关着的门，基础版的登记照旧。
- `parcel-api` 启动日志多一行，说明开关开没开。

## 越权风险点

1. **默认关、只认 `true`。**（决定二）运维拼错值时登记答未启用而不是放行；代价是多一次排查。归运维。
2. **关着时重放也拒。**（决定四）已在册的同一版在开关关着时重放答未启用，不补发交接；依赖重放补交接的流程（本口落地之前登进册的卡借重放补那一封）在关着的部署里对较新版本的卡不成立。归 PP owner。
3. **开关不防回滚。**（决定六）开开关是对「这一版不再回滚到旧构建」的承诺；回滚之后已在册的较新版本卡仍让旧构建出错即停，租户级序列与目录登记 / 复核整笔失败。归运维与 PP owner。
4. **一个开关管所有较新版本。**（决定一、二）将来有 `PPC-7` 时，开着的部署也放开它；需要分版放开要另立决定。归 PP owner。
5. **开关是进程级、不是租户级。**（决定二）一个部署里所有租户同开同关；按租户放开不接。归运维。
6. **开关不管草稿。**（决定一、六）开关关着时，导入照样能把 `PPC-6` 的草稿存进价卡草稿册（`price_card_draft`）——门只在登记处。滚动窗口里旧构建的草稿列表读口（`ListPriceCardDrafts`）逐行重建、遇到它不认的版本整批返回错误，会让该租户的草稿列表整页失败。本门明示不管这一格，由「先升级所有读价卡的进程、再开开关」的发布顺序兜：升级完成之前不要让操作者导入带新结构的卡。归运维与 PP owner。

## Links

- [ADR-0348](./0348-a-price-card-may-declare-total-charge-bounds-and-key-a-surcharge-table-by-remote-tier-and-plans-using-them-are-written-as-ppc-6.md)：`PPC-6` 与旧构建读到它时的各路答复；本记录停用其越权风险点 6 里「代码里没有门」「列为后续加固，不在本轮」那几句，滚动发布约束本身照旧
- [ADR-0349](./0349-the-price-card-import-template-advances-to-ppt-2-and-its-structured-workbook-is-a-second-carrier-read-through-the-same-path.md)：导入能造出 `PPC-6`；本记录回答其越权风险点 5，无条款停用
- [ADR-0267](./0267-a-key-added-to-an-already-canonicalized-body-is-a-shape-change-even-when-old-documents-stay-byte-identical.md)、[ADR-0014](./0014-versioned-canonicalization-shape-for-content-digest.md)：换号与「旧构建答不认这一版」
- [ADR-0101](./0101-operator-facing-registration-payload-shape-is-product-defined.md)：发布交既有登记用例；本记录停用其决定五对答案代数六格的列举，登记结果代数多一格，发布照样把登记那一格原样交回
- [ADR-0146](./0146-product-strategy-is-a-third-class-between-mechanism-and-tenant-values.md)：门与开关是机制，开不开是部署方的断言
- [ADR-0180](./0180-admission-scope-tenant-mapping-is-an-access-identity-register-and-self-authority-is-a-deployment-parameter.md)：部署参数的同类先例（`IDP_PARCEL_GOVERNANCE_SELF_AUTHORITY`）
- 票 [pricing-engine-gaps/03](../../.scratch/pricing-engine-gaps/issues/03-newer-canonicalization-registration-deployment-switch.md)
