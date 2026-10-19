# 30 BUY 成本卡声明金额取整（09 余半）

Category: enhancement
Status: in-progress——完工、评审已修订，待进 main，分支 claude-dcg26（与 [26](./26-settlement-consumes-charge-occurrence-formed-to-request-buy-evaluation.md) 同一分支，本票先落）。此前 ready-for-agent——2026-10-19 Claude Code 会话立（用户「你作为业务和系统专家自决……解决所有的问题」授权）。代决见 [tf-chain-plan](../tf-chain-plan.md)。
Blocked by: 无
地盘：`scripts/demo-seeds/data/pricing/` 成本卡新版本、`seed.sh` 一行。
出处：[09](./09-demo-price-cards-declare-no-amount-rounding.md)；方案「BUY 发起（接 08，收 10）」D10-1，「已不成立或已修」09。

## 做什么

- 演示 BUY 成本卡补声明 `amountRounding`，以新版本重发，`seed.sh` 加一行（方案 D10-1）。已改判为就地改 v1、`seed.sh` 不加行，见完成记录「代决」与方案 D10-1 的改判。
- 进 main 的种子令牌次序见方案「并行车道」。

## 完成判据

- 成本卡以新版本重发并声明取整。（已改判：就地改 v1 声明取整，见完成记录。）
- `seed.sh` 退出码 0。
- 走 ADR-0200 路径时不再落「金额未声明」格。

## 规模

S

## 完成记录（2026-10-19，Claude Code 作者子代理，分支 `claude-dcg26`，提交 `4ec22881`）

### 做了什么

- `scripts/demo-seeds/seedgen` 的 `buyCardSnapshot` 声明金额取整：合计点、`0.01 CNY`、`HALF_UP`，与售价卡同值。重跑生成器，只有 `price-card-cn-sg-cost.json` 变，内容摘要换新。
- `internal/parcelpricing/adapters/pricecardtemplate` 的夹具 `costCard` 补三格，模板导入的演示成本卡与种子仍同摘要（「夹具同摘要」那条用例）。底卡现在三格齐，「三格缺一」那条用例改成删掉一格。
- 规范 `docs/design/pp-price-card-import-template-and-validation-spec.md` 第八节的成本卡样例补三格；`scripts/demo-seeds/README.md` 的 `data/pricing/` 一行补一句。
- 新用例 `internal/settlementaccounting/adapters/parcelpricing/demo_cost_card_test.go`：照种子原样重建演示成本卡，对 3 kg、Z1 真算一份 BUY 评价，经 BUY 读口交回已完成、`CNY 8500` 分。用旧种子跑同一条，落「金额未声明」（实测红）。

### 代决

- **就地改 v1，不另发 v2**。票面与方案 D10-1 都写「以新版本重发」，核过在用解析之后改判：
  - `PriceCards.ResolveInForce` 按（租户、范围、方向、目的、时点）取适用期含该时点的全部版本，多于一行就答适用冲突，不分是不是同一方案身份。v1 适用期无上界，任何 v2 都与它重叠。
  - 价卡版本没有退役或截期口。给 v1 截期，本身就是改 v1。
  - 路由取候选（`LoadApplicable`）遇同一方案两版报歧义，演示线路按段取价那条会断。
  - 与票 09 原「最小的诚实修法」、sell-customer-charge/05 对售价卡的做法一致。
- 所以 `seed.sh` 不加行：原有那一行登的就是这份文件。

### 越权风险点

1. 已经灌过旧种子的库，再跑 `seed.sh` 会在成本卡那一行答内容冲突（退出码 2），只能 `--reset` 或换新库（演示租户 owner；售价卡在 sell-customer-charge/05 时是同一处境）。
2. `0.01 CNY`、`HALF_UP`、合计点是 `SYN` 取值，证据 `S`，不进参数登记册（`PAR-SET-03` owner）。

### 用例

- 演示成本卡评价经 BUY 读口交回已完成（`TestTheDemoCostCardYieldsAnAdoptableBuyEvaluation`）。
- 演示线路按段取价仍是三段各 85 CNY、合计 25500 分（`cmd/parcel-dispatch` 的 `TestADeclaredParcelOnZonedLegsIsPricedThroughProductionWiring`）。
- 模板导入与种子同摘要（`pricecardtemplate` 包全绿）。

### 验证

- `go test` 带真库：`./internal/parcelpricing/...`、`./internal/settlementaccounting/adapters/parcelpricing/`、`./cmd/parcel-pricing-register/`、`./internal/networkrouting/adapters/registrationjson/`，`./cmd/parcel-dispatch/` 中三条路由按段取价用例，全绿。
- 一次性库 `dcg26_seed30` 上 `seed.sh` 退出码 0，两张卡 `RECORDED`，成本卡摘要为新值；用完已删库。
- `go build ./...`、`go vet`、`gofmt -l` 干净。

### 偏离与待裁

- 偏离：「新版本」改为就地 v1，见代决。
- **待裁（交给 [27](./27-transport-charge-occurrences-are-registered-and-handed-off.md) 的端到端，读码，证据 `S`）**：演示成本卡没绑分区目录。PP 按发生项造的快照取成员包裹的申报与邮编路线，不带分区。卡没绑目录、输入也没给分区时，`resolveCatalogues` 答分区解不出，评价不形成。所以 27 的端到端多半停在 BUY 评价这一步，到不了供应商预期成本，与金额取整无关。两条路，认领 27 前要裁：
  - 成本卡绑分区目录。这会让路由按段取价改读目录，要先核 ADR-0189 的段区域折法。
  - PP 按发生项取段计价区域。

## 评审记录（2026-10-19，与票 26 同一次隔离评审）

- 阻断 0 项（本次评审唯一的阻断在票 26 的地盘，见 [26](./26-settlement-consumes-charge-occurrence-formed-to-request-buy-evaluation.md) 评审记录）。
- 非阻断 1 项落在本票，已修：方案 D10-1 仍写「以新版本重发」，偏离只记在本票。D10-1 下追加一条改判，指向本票完成记录；上文「做什么」「完成判据」两处注明已改判。
- 评审核过就地改 v1 有据（在用解析多于一版即适用冲突、路由取候选遇同方案两版报歧义），与 sell-customer-charge/05 先例一致；旧内容摘要仓里没有别处引用。
