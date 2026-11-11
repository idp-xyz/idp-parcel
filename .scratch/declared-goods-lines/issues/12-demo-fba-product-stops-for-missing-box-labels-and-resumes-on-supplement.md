# 12 演示：FBA 产品缺箱标号停下，受控补充补齐后受理

Category: enhancement
Status: in-progress
Blocked by: 09、10、11（均已提交，未进 main；本票与它们同在分支 `feat/goods-details-admin`）

依据：[ADR-0376](../../../docs/adr/0376-fba-submission-conditions-are-judged-as-required-documents-platform-identifiers-may-be-supplied-before-acceptance-and-rule-packages-follow-the-service-product.md) 三项决定；票 08 完成记录「未做」两项。

## 要做的

- 演示租户 `SYN-TENANT-01` 登一个合成的 FBA 头程服务产品与它的接单规则包：规则包指名这个产品、声明「必要资料」组适用、以 `CROSS_FIELD_CONDITION` 引用 `REFCFG-1:parcel-shipment/submission-conditions/FBA-HEADHAUL@1`；产品的价格规则（不登，见完成记录）、客户服务规则、授权规则（不登，见完成记录）与产品到渠道的映射照小包产品的样子各登一份。
- 演示合同按 ADR-0376 决定三重发一版不在外壳指名接单规则包（随票 11 可缺正文那一格），小包与 FBA 两个产品同一账户各解各的规则包。
- 价卡：演示引擎支持按整票汇总计重的，FBA 产品登一张按整票计费的售价卡（票 08 未做的那一项）；不支持就照实写明。（不登，见完成记录）
- 演示段：提交一票缺两箱箱标号的 FBA 货件 → 停在等待受控补充，查阅面列出缺箱标号的那两箱 → 经受控补充补齐 → 已接受；小包产品同时提交照常受理。
- 演示 README 的 FBA 动线补这一段。

## 完成判据

- `seed.sh --reset` 灌进一次性库退 0；演示段跑通，证据记 `S`，参数登记册不动。

## 完成记录（作者一侧，2026-11-11）

- 提交：分支 `feat/goods-details-admin` 上的 `733f778e8` 与评审修正 `37c172945`（在分支 `feat/demo-fba-supplement` 上写成 `e05687321`、`1661011b1`，基线 `2f0963ca7`，拣入时无冲突），未推送、未进 main。只动 `scripts/demo-seeds/**`，没有 Go 改动。
- 落点：
  - 种子步骤 116，新对象单放三份批，不并进首发那份：首发批的逐项回显被 `cmd/parcel-commercial` 的测试逐字钉着。
    - `publish-fba-product.json`：五项。
      - 服务产品 `SYN-PROD-CN-SG-FBA`：交付条件照快递产品。
      - 接单规则包 `SYN-RULEPKG-FBA-01`：壳指名该产品；适用组在 `SYN-RULEPKG-01` 那几组之外加「必要资料」；`CROSS_FIELD_CONDITION` 一类引用 `REFCFG-1:parcel-shipment/submission-conditions/FBA-HEADHAUL@1`，替掉快递那份的不透明引用；其余类别与声明同快递规则包。
      - 客户服务规则 `SYN-SERVICE-RULE-FBA-01`：账户解析键要客户服务规则，指名快递产品的那条对 FBA 不适用。
      - 客户合同 `SYN-CONTRACT-01` 同起点重发 `v2`：壳与正文都不指名规则包（ADR-0376 决定三，票 11），绑定、委派与交付条件照 `v1`。
      - 结算政策 `SYN-SETTLEMENT-PREPAID-01` 重发 `v2`：正文指名合同版本（ADR-0080），随之指 `v2`。
    - `close-fba-contract-reissue.json`：合同与结算政策的 `v1` 按 ADR-0233 决定三让位给 `v2`。
    - `register-products-fba.json`：服务形态与产品—渠道映射 `SYN-MAP-CN-SG-FBA-01`，渠道引用同快递产品。
  - 演示段 `demo-run/55-fba-supplement.sh`，经首方客户渠道：
    - 补一笔余额；
    - 用 FBA 产品提交一票四箱平台货件，第 2、4 箱不报箱标号；再提交一票快递小包。
    - 小包等到已接受。
    - 货件等到查阅面列出缺项，逐格核对：缺项正好是第 2、4 箱各一条，组是 `REQUIRED_DOCUMENT`，原因是 `EVERY_PARCEL_HAS_PLATFORM_BOX_LABEL`；多一条、少一箱或归错组都算失败。
    - 经 `/v1/shipment-requests/supplements` 整份重报，平台货件与四箱箱标号整份带上，答 `201 SUPPLEMENT_RECORDED`。
    - 新版本已接受、缺项消失、`priorVersionCount` 为 1，逐箱箱标号与补充所发逐一相同；打出两张运单号与管理台详情地址。
    - 查阅答非 200 就地停下并打原始答复。404 也停：查阅直读提交同一笔写下的行，提交答过 201 之后读不到是统一不可见（委托号或令牌不对），不是「还没到」。
    - 接入、货件、箱号等标识都带 55（`SYN-ACCESS-FBA55-`、`SYN-FBA55-`、`SYN-CTN55-` 等），与 `50` 不相撞。
    - 带 `--hold` 停在缺项那一步不补，供在管理台运单详情讲「等客户补 · 缺平台箱标号」。
  - README：
    - 演示 README 的分段表加 `55`；「FBA 动线」一节改写：两个产品各走哪段、为什么共用一张卡（选卡缺产品一维，指票 13）、停下与续走对应 ADR-0376 哪条决定、`55 --hold` 与补齐后在管理台看什么。删掉「还走不到的一步」。「约八次后额度不足」限到 `50`。
    - 种子 README 数据表补一行；去掉一处跨文件的数。
  - `50-fba.sh` 订正两处过时注释：「演示租户没有单列 FBA 产品」与「等受控补充时任务不在跑了」。快递那两票不判提交条件的理由写成 `SYN-RULEPKG-01` 不声明「必要资料」组、也不引用 FBA-HEADHAUL。`50`、`55` 记成可执行，同 10–45。
- 判据：
  - `seed.sh --reset` 灌进一次性库 ✅：经 `up.sh demo_goods12 --reset`（`:19380`）退 0，步骤 116 五项 `PUBLISHED_EFFECTIVE`、两项退役 `REGISTERED`、两项产品登记 `REGISTERED`。
  - 演示段跑通 ✅。合同与结算政策重发 `v2` 改了 `SYN-ACCOUNT-01` 每一票的解析，所以评审修正后重灌一次，各段依序各跑一遍，全部退 0：

    | 段 | 退出码 | 耗时 |
    |---|---|---|
    | `10-main-chain.sh` | 0 | 41 s |
    | `20-customs.sh` | 0 | 45 s |
    | `30-settlement.sh` | 0 | 不到 1 s |
    | `40-exceptions.sh` | 0 | 23 s |
    | `45-pending-disposition.sh` | 0 | 17 s |
    | `50-fba.sh` | 0 | 2 s |
    | `55-fba-supplement.sh` | 0 | 37 s |

    之后 `55` 的取数又收紧一处（404 也停），在同一个栈上再跑 `55` 与 `55 --hold`，都退 0。最后那一趟 `55` 的关键行：
    - `受理判断 SHR-N46MBKOFYXKFM5SGUGBNXD6OMY → request.state=ACCEPTED（快递产品照旧采用 SYN-RULEPKG-01）`
    - `受理任务停下等客户补充 → request.state=SUBMITTED acceptanceTask.state=RUNNING supplementGaps=2 条`
    - `缺项 → group=REQUIRED_DOCUMENT parcelId=DPCL-M2KWNLSTM6KH45MOGCELSQ5UHY reason=EVERY_PARCEL_HAS_PLATFORM_BOX_LABEL`
    - `缺项 → group=REQUIRED_DOCUMENT parcelId=DPCL-UOXV5UADUTRVI3UPOOQ7W5T2MA reason=EVERY_PARCEL_HAS_PLATFORM_BOX_LABEL`
    - `受控补充（补齐第 2、4 箱箱标号） → 201 SUPPLEMENT_RECORDED submissionVersionId=SUBV-OW23WWLYZB5Y5FMAYS3LFSGZGM`
    - `受理判断（补充后的版本） → request.state=ACCEPTED priorVersionCount=1 supplementGaps=无，4 箱箱标号与补充所发逐一相同`
  - 小包与 FBA 同一账户各解各的规则包 ✅，是推断，不是读出来的：查阅面不露采用的规则包。
    - FBA 货件出了「必要资料」组的缺项，只有 `SYN-RULEPKG-FBA-01` 声明这一组。
    - 同一趟的快递小包与 `10`、`20`、`40`、`45`、`50` 的委托都没出缺项、照常接受，也没答指名引用未确认。
  - 管理台详情能看到 ◑：缺项读的就是运单详情页取数的同一个读口，页面也有「等客户补」的写法，但没在浏览器里点过。
  - 证据记 `S`，参数登记册一字未动 ✅。
- 与票面不同的几处：
  - **价卡没另登按整票计费的一张**：同一账户的不同服务产品现在选不了各自的卡，这是缺的机制，不是种子没登。
    - 试算与评价按（租户，定价范围，方向，目的，时点）选卡（parcel-pricing `ResolveInForce`），没有服务产品一维。
    - 定价范围取自账户的结算采用记录（`data/settlement/02-pre-acceptance-estimate-adoption-account-01.json` → `apply_pre_acceptance_control.go` 的 `adoption.PricingScope()`），一户一个。
    - 同范围再登一张售卖卡解成适用冲突；登在别的范围，`SYN-ACCOUNT-01` 的 FBA 委托永远选不到它。
    - FBA 产品因此沿用那张按件计重、逐箱求和的卡 `SYN-PLAN-CN-SG-01`。按产品选卡归[票 13](./13-one-account-prices-each-service-product-on-its-own-card.md)。
  - **价格规则没另登**：价卡以 `SYN-PRICE-RULE-CN-SG` 为价格政策依据。另登一份指名 FBA 的价格规则，要有一张卡引用它才有人读，而这个账户的 FBA 选不到单独的卡（同上）。价格规则本来也进不了解析键：登记面拒它，要另带价格方向一维。
  - **授权规则没另登**：
    - 取消与来源修订两份不挂产品，按范围取，已覆盖 FBA。
    - 价格方向那份挂在价格规则上、成本方向那份挂在快递产品上，两份都还是壳，也不在 `SYN-ACCOUNT-01` 的解析键里。
    - 受理没有用到它们，照登一份只是没人读的对象。
  - **「停下」的读面信号**：等受控补充时受理任务仍是 `RUNNING`（只是续办方换成客户，`waitingOn` 不上读面）。`STOPPED` 只在撤回时出现。所以轮询以缺项出现为停下信号，不等任务离开 `RUNNING`；`50-fba.sh` 原注释说反了，已订正。
  - **建模问题，记下不改，归 PC owner**：合同 `v2` 的交付条件 `tightens` 仍只指快递产品 `SYN-PROD-CN-SG-EXPRESS v1`，`DeliveryConditionReferenceFor` 又不核收紧的是哪个产品，于是 FBA 的交付条件从一层为快递声明的收紧里解出来。问题是：一份多产品合同能不能逐产品收紧交付条件？种子照旧。
- 验证：`go test ./cmd/parcel-commercial/ ./referenceconfig/... -count=1` ok，两笔提交后各跑一次。读种子数据的测试包在 `e05687321` 上全部 ok：
  - `./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./cmd/parcel-pricing-register/`、`./cmd/parcel-customs-register/`、`./cmd/parcel-document-numbering-register/`、`./cmd/parcel-node-operations-register/`、`./cmd/parcel-transport-register/`、`./cmd/parcel-ve-register/`；
  - `./internal/settlementaccounting/adapters/parcelpricing/`、`./internal/parcelpricing/adapters/pricecardtemplate/`、`./internal/transportfulfillment/adapters/partycommercial/`、`./internal/parcelshipment/adapters/partycommercial/`。

  评审修正没碰种子数据，只在 `1661011b1` 上复跑了 `./cmd/parcel-api/`。`bash -n` 三个脚本；`55` 的取数失败两路（连不上、404）另起 shell 试过，都就地停下并打出原因。`down.sh` 已收，库 `demo_goods12` 保留。`shellcheck` 本机没装，没跑。
- 评审：非作者评审一轮，无阻断项，全改或记下：
  - 价卡「另立范围」说错了：另立范围这个账户选不到——改成缺选卡机制、指票 13，README、种子 README、50 与 55 头注与本记录同改；
  - 55 的缺项断言补核组，补充后补核版本数与逐箱箱标号等于所发；
  - 步骤 116「第二个服务产品」与种子 README 两处数——去掉；
  - 50 两处新注释的理由说错——改成快递规则包不声明「必要资料」组、不引用 FBA-HEADHAUL；
  - 50、55 文件权限没进索引——`git update-index --chmod=+x`；
  - 55 取不回时漏成 KeyError——非 200 就地停下；
  - 55 与 50 标识可能相撞——带 55；
  - 「约八次」限到 50，重灌后各段复跑一遍；
  - 交付条件的收紧只指快递产品——记进「与票面不同的几处」，种子不改。
- 拣入后复验（2026-11-11）：`37c172945` 叠在 09、10、11 的评审修订之上（09 的修订改了 `FBA-HEADHAUL@1` 的说明文字与发布摘要，收紧了读不回规则包的处理）。
  - 从这一提交重建二进制，`up.sh demo_goods12 --reset` 退 0；`55`、`50`、`10-main-chain.sh` 各退 0。缺项、补充与接受各行与前一轮相同。规则包按标识与版本引用参考配置，摘要变化不影响；日志里没有新的无法判定原因。
  - 读种子数据的测试包在 `feat/goods-details-admin` 上全部重跑 ok，名单同上。
