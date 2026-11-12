# 17 演示：FBA 产品用自己的售价卡（逐箱计价）

Category: enhancement
Status: resolved——2026-11-12 进 main
Blocked by: 14、15（均已提交，未进 main；同在分支 `feat/goods-details-admin`）

依据：[ADR-0382](../../../docs/adr/0382-a-sell-price-card-may-name-the-service-products-it-prices-and-an-unnamed-card-prices-the-rest.md)；票 12 完成记录「价卡没另登」一条。

## 要做的

- **种子**：演示租户在 `SYN-SCOPE-01` 登一张指名 `SYN-PROD-CN-SG-FBA` 的 SELL·客户费用卡。
  - 接受前估价与正式 `SELL` 评价都逐件评价（ADR-0382 越权风险点 8），这张卡逐箱计价：合成取值，用与快递卡不同的价表（例如按公斤计价、不收首重），让两张卡算出的价看得出差别。
  - 规范化版本 `PPC-7`，演示的部署开关已开。
  - 快递那张 `SYN-PLAN-CN-SG-01` 不动，照旧接其余产品。
- **演示段 `55`**：断言 FBA 货件的接受前估价与正式 SELL 评价用的是 FBA 卡，同一趟的快递小包用的是 `SYN-PLAN-CN-SG-01`。
- **README**：删掉「FBA 沿用快递卡」那几处，改写为两张卡各计各的价。

## 完成判据

`up.sh <库> --reset` 退 0；`55`、`50`、`10-main-chain.sh` 各退 0；读种子数据的测试包通过；证据记 `S`，参数登记册不动。

## 完成记录（作者一侧，2026-11-11）

- 提交：`feat/goods-details-admin` 上的 `4178889d0`（作者分支 `feat/demo-fba-own-card` 的 `e5e7abaf9` 拣过去的那一笔）与评审修订 `a73304a3e`（写在 `4178889d0` 之上，快进合入）；未推送、未进 main。
- 落点：
  - 种子：`seedgen/fba_card.go` 经领域构造生成 `data/pricing/price-card-cn-sg-fba.json`，即售价卡 `SYN-PLAN-CN-SG-FBA-01` v1。重跑 `seedgen` 时既有各份逐字节不变。
    - 范围 `SYN-SCOPE-01`，SELL·客户费用，适用服务产品 `SYN-PROD-CN-SG-FBA`，规范化版本 `PPC-7`，内容摘要 `c1e528ac137ecb3b994e053aab91bd23514cafec1936e7c4ebeafbbd73a3152f`。
    - 价表与快递卡刻意不同：按公斤单价 Z1 24、Z2 21 CNY，不收首重；体积重按 6000 折、计费重整公斤进位；没有固定附加费，也不绑燃油序列。
    - 绑 150 一节已复核的分区目录 `SYN-CAT-ZONE-CN-SG`，金额取整同快递卡，费用代码沿用 `BASE_FREIGHT`：结算的费目映射按费用代码配，不必另登。
  - 种子步骤 `170-pricing-fba-card.sh`：照 165 一节，只对这一次 CLI 调用设 `IDP_PARCEL_PRICING_REGISTER_NEWER_CANONICALIZATION=true`（ADR-0352）。`up.sh` 给它起的 API 与 dispatch 进程同样设了这个开关（已核）。
  - 快递卡 `SYN-PLAN-CN-SG-01` 一字未动，照旧接其余产品。
  - 演示段 `55`：接受之后加一节「售价」。
    - 价卡目录：按方案标识 `q` 各取一次。核两张卡同在 `SYN-SCOPE-01`；FBA 卡只指名 FBA 产品、规范化 `PPC-7`，快递卡不声明产品；取两张卡的内容摘要。
    - 试算 `/pricing-estimates`：同一只 12.5 kg、60×40×40 cm 的箱子按两个产品各算一次。核选中的卡、评价 `COMPLETED`、答复上的 `serviceProduct`，以及解释首行「price card selected for declared service product …」。
    - 正式 SELL 评价：演示账户的计费时点是网络收寄已采用，所以 FBA 第 1 箱与快递小包各送站预报、收寄一次，先等三段运输准备，逐一收。
      - 认费用：收寄前记下时刻，费用册按 `formedAt` 倒序取此刻之后形成的恰一笔。估价阶段的费用行还没有包裹号，来源事实与确认依据两格到确认才落，按包裹号 `q` 检索不到。
      - 对卡：经评价号 `q` 在 `/pricing-evaluations` 取 `planContentDigest`，对上各自价卡的内容摘要。
  - Go 测试 `cmd/parcel-pricing-register/demo_fba_card_test.go`：
    - 种子卡重建为 `PPC-7`，内容摘要钉住，只指名 FBA 产品；
    - 演示那只箱子 Z1 手算 384、Z2 手算 336，只有 `BASE_FREIGHT` 一行。
  - README 与注释：
    - 演示 README 的 FBA 动线、分段表 `55` 一行、管理台三页（价格表、计价试算、客户费用）都改写为两张卡各计各的产品；「管理台仍读不到」补一条接受前估价用了哪张卡。
    - 种子 README 加 FBA 卡一行，那一行与 `fba_card.go` 注释记下方向授权的风险（见「与票面不同」）。FBA 产品那行同改；完整价卡三那行的「其余仍是 PPC-5」改成指 FBA 卡写 PPC-7。
    - 种子 README 配价一节：FBA 卡那句放在分区目录分句之后，材积除数按卡说。
    - 部署开关的说法改成「只在登记较新规范化版本价卡的步骤上、只对那一次调用设」：种子 README、步骤 165 注释与 `up.sh` 同改。
    - `50` 头注、步骤 160 与 500 的注释同改：160 原说快递卡是动线上唯一的一张，500 原说估价选卡不看产品。
- 判据（评审修正后在 `a73304a3e` 上重跑）：
  - `up.sh demo_goods12 --reset`（`:19380`）退 0，步骤 170 答 `price-card: RECORDED` ✅。
  - 依序跑 `55`、`50`、`10-main-chain.sh`，退出码 0、0、0（37 s、3 s、83 s）✅。`55` 的售价几行：
    - `价卡目录 → SYN-PLAN-CN-SG-FBA-01 指名 SYN-PROD-CN-SG-FBA（PPC-7）；SYN-PLAN-CN-SG-01 不声明产品，同在 SYN-SCOPE-01`
    - `试算 SYN-PROD-CN-SG-FBA → plan=SYN-PLAN-CN-SG-FBA-01 serviceProduct=SYN-PROD-CN-SG-FBA total=384 CNY（解释首行：price card selected for declared service product SYN-PROD-CN-SG-FBA）`
    - `试算 SYN-PROD-CN-SG-EXPRESS → plan=SYN-PLAN-CN-SG-01 serviceProduct=SYN-PROD-CN-SG-EXPRESS total=744 CNY（解释首行：price card selected for declared service product SYN-PROD-CN-SG-EXPRESS）`
    - `正式 SELL 评价 FBA 第 1 箱 → charge=SELLCC-SELLREQ-FXQ6HVJENRTIOUSXZOBMBENB7M evaluation=EVAL-K4NZ3CRHHYQJGNYT5RW62UXWHY plan=SYN-PLAN-CN-SG-FBA-01（内容摘要对上，PPC-7） 384.00 CNY`
    - `正式 SELL 评价 快递小包 → charge=SELLCC-SELLREQ-WFNDZ3SDHDMIF4M2BRVS2DXSTE evaluation=EVAL-A65YF5IFRL36LH53PAX2HBBBCU plan=SYN-PLAN-CN-SG-01（内容摘要对上，PPC-5） 78.00 CNY`
  - 缺项、补充与接受几行同票 12，没变 ✅。`10` 的 SELL 预估客户费用照旧 `9600` ✅。
  - 接受前估价用的是 FBA 卡 ✅，但**不是脚本断言的**：同一轮之后只读查库取证。`settlement_accounting.pre_acceptance_estimate_trace` 的 `price_card` 列：
    - `55` 那票 FBA 的两个提交版本都是 `pricing-plan:SYN-PLAN-CN-SG-FBA-01@v1`，四箱合计 153600；
    - `55` 的快递小包、`50` 的两票（申报快递产品）与 `10` 的那件都是 `pricing-plan:SYN-PLAN-CN-SG-01@v1`。
  - 快照与解释写着产品 ✅：试算答复上看得见（上面两行）。正式评价那两份同样记着，同一轮只读查库取证：`parcel_pricing.evaluation.snapshot` 的 `input.serviceProduct` 分别是 FBA 与快递产品。
  - 读种子数据的测试包全部 ok，带 PG（`IDP_PARCEL_POSTGRES_DSN` 指 55432，测试库由模板克隆），在 `a73304a3e` 上跑：
    - 票 12 那一串：`./cmd/parcel-commercial/`、`./referenceconfig/...`、`./cmd/parcel-api/`、`./cmd/parcel-dispatch/`、`./cmd/parcel-pricing-register/`（含新测试）、`./cmd/parcel-customs-register/`、`./cmd/parcel-document-numbering-register/`、`./cmd/parcel-node-operations-register/`、`./cmd/parcel-transport-register/`、`./cmd/parcel-ve-register/`、`./internal/settlementaccounting/adapters/parcelpricing/`、`./internal/parcelpricing/adapters/pricecardtemplate/`、`./internal/transportfulfillment/adapters/partycommercial/`、`./internal/parcelshipment/adapters/partycommercial/`；
    - 另加读全部演示价卡的 `./internal/parcelpricing/adapters/postgres/`。

    `gofmt`、`go vet ./cmd/parcel-pricing-register/ ./scripts/demo-seeds/seedgen/` 通过 ✅。
  - 证据记 `S`，参数登记册一字未动 ✅。`down.sh` 已收，库 `demo_goods12` 保留。
- 与票面不同的几处：
  - **接受前估价没有在段里断言**：它用了哪张卡今天没有读口——结算的估价留痕（`PreAcceptanceEstimateTraces`）只写不读，计价一侧不留估价。段里断言的是试算与正式 SELL 评价两条路；接受前估价靠上面那次只读查库。补读口要动 Go，不在本票地盘。
  - **正式评价只收寄了 FBA 第 1 箱**：一箱足以对上卡；四箱都收寄只多三轮等待。收寄也让这两件进了网络，后续段不受影响（`10` 照旧走完）。
  - **方向授权**：FBA 卡照演示其余售价卡（含 `SCOPE-02/03/04` 那三张）引 `SYN-AUTH-PRICE-DIR-01`。计价只引授权、从不解析，登记只核它是一份商业授权引用。那份授权挂的价格规则 `SYN-PRICE-RULE-CN-SG` 正文指名的是 `SYN-PLAN-CN-SG-01` 与快递产品，所以 FBA 卡的引用背后没有一份指名它的价格规则——引得进来不等于授权覆盖了它；这一句也记在 `fba_card.go` 与种子 README 那一行。授权对象没重新登：另登一份，目前没有任何一方读它（口径读口 `PricePolicyCaliberView` 没有装配消费方）。
  - **售价仍逐件评价**：FBA 卡逐箱计价，整票汇总计费重的卖法表达不了（ADR-0382 越权风险点 8），README 与 50、55 头注照实写了。
- 评审：非作者评审一轮，无阻断项；修正在 `a73304a3e`，全改：
  - 种子 README 配价一段的 FBA 句子读成绑燃油序列——挪到分区目录分句之后；材积除数按卡说。
  - 「别的步骤不设」已不成立——README、步骤 165 与 `up.sh` 改成只在登记较新规范化版本的步骤、只对那一次调用设。
  - 55 认费用靠对整册做差，整册按费用号排、每页有上限——改成按形成时刻认、评价按评价号 `q` 检索。评审建议的按包裹号 `q` 检索费用没照做：估价阶段的费用行来源事实与确认依据两格都是空的，`q=<包裹号>` 检索不到（只读查库核过，只有 `10` 那笔已确认的费用带包裹号）；`chain.sh` 的 `ch_intake` 在 `4178889d0` 上也还是对整册做差，不是按包裹号检索。
  - 方向授权的风险随卡记下——`fba_card.go` 与种子 README 那一行各一句。
  - 小毛病：
    - 加了钉住演示 FBA 卡的测试；
    - 去掉多余的 `sleep 1`，收寄脚本的来源按纳秒起；
    - 快递小包的包裹号取不到就地停。
## 进 main 记录

- 2026-11-12 随 declared-goods-lines 链进 main，main 尖 `c5001b3de`；重放、改号（分支 0378、0380 落地为 ADR-0382、ADR-0383）、生成物与门禁的完整记录见 [25 号票](./25-an-incomplete-declaration-for-the-estimate-waits-for-customer-supplement.md)「进 main 记录」。
