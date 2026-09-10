# 33 首次有效收寄判断的两格语义修正：待确认 → 已形成前补问首次判据；轨迹来源自动派生的回指在链上无版本时视作首次判断

Category: bug
Status: resolved——2026-09-10 20:2x（本机时钟）通道 1 推送方重放进 main（非作者评审 ← 通道 4 两轴 0 阻断；作者通道 6 在写完成记录前 crash，完成记录由推送方按分支与验证结果代写，见 Comments；main 上 SHA 对照见「进 main 记录」）。此前 in-progress——2026-09-10 20:0x 通道 6 按通道 1 派单 task-ea5ac39e 自立自做；分支 `mcp6-lc33` 基远端 main `b40b1804`（fetch + ls-remote 核），隔离树 `D:/tops/idp-parcel-mcp6-lc33`；按 /implement 含 /tdd，两格各先见红
Blocked by: 无（[`31`](./31-carrier-first-effective-pickup-fact-registry-and-handoff.md) 已于 2026-09-10 进 main `03474d87`，本票改的是它落下的编排）

## 缺口

出处是票 [`31`](./31-carrier-first-effective-pickup-fact-registry-and-handoff.md) Comments「评审 ← 通道 2」那一条的 Spec 非阻断两格，推送方对着 `internal/transportfulfillment/application/judge_carrier_first_effective_pickup.go` 的 `Judge` / `resolveBasis` 核过属实；此处只对号，不复制第二套：

1. **待确认 → 已形成那一步没有再问首次判据。** `Judge` 只在链上无版本或链尾已失效时问 `objectUnderControl`；链尾为待确认时，`reconsiderPending`（同一依据、身份此刻在册）与 `formOrHold` 的 `found` 分支（另一依据、在册主体）都直接 `Supersede` 成已形成并交意图。对象在待确认期间凭场外揽收或`已交接`进段、之后身份才登记，就会落一版已形成、进段被拒只留 refusal——违 [ADR-0135](../../../docs/adr/0135-carrier-first-effective-pickup-is-a-judged-control-fact-with-its-own-registry-and-enters-the-segment.md) 决定四「非首次……不形成版本」与 TF CONTEXT「已处于运输控制中时新到承运证据不构成首次」。更正那条路（`rederive`）在链尾待确认时同样能 `Supersede` 成已形成，是同一格的第三个入口。
2. **轨迹来源自动派生的回指在链上无版本时把链锁死。** `resolveBasis` 对轨迹事实在命令未指名时把 `corrects` 填成该代的 `Supersedes()`，而 `Judge` 对 `corrects != "" && !found` 一律答 `BASIS_NOT_CURRENT`，调用方无法清空——对象上第一条被判的轨迹版本若本身是更正代（v1 未判、v2 更正 v1 才表达收寄），这条链永远形成不了收寄。既有用例只盖「链存在但依据不同」那格。

两格都是保守失败（拒绝而非误形成），所以不阻断 `31` 合入、另立本票。

## 做法

1. **首次判据收成一处，凡链尾不是已形成而本次要形成新版本的入口都先问它**：`Judge` 主路把「链上无版本或链尾已失效」放宽成「链上无版本或链尾不是已形成」（待确认也算）；`reconsiderPending` 在读法为收寄之后、解承运主体之前问；`rederive` 在链尾不是已形成且读法仍为收寄时问。在控 → 答 `NOT_FIRST`、不落版本、不发意图。**待确认链尾留原样，不失效**——理由两条：决定四原句「非首次……不形成版本」，失效也是一版；领域 `Void` 只从已形成长出（`ErrCarrierPickupNotFormed`），待确认本就不提供、不进段，留着无害且保住「判过了但不够」的历史。
2. **自动派生的回指与命令显式指名的分开**：`pickupBasisInput` 多一格「回指是否自动派生」；`Judge` 在 `corrects != ""` 时——链尾恰以被更正那一代为依据 → 照旧走 `rederive`；链上有版本但依据不同、或回指是显式指名的 → 照旧 `BASIS_NOT_CURRENT`；**回指是自动派生的且链上无版本 → 视作首次判断照正路走**（读法不为收寄 → `NOT_A_PICKUP`；在控 → `NOT_FIRST`；否则 `formOrHold` 首登，首版不回指）。被更正的那一代从未被判过，链上没有任何东西可被它替代或失效，这条证据就是这条链的第一次判断。
3. 顺手：`rederive` 的 `object` 形参因做法 1 被真正使用，`_ = object` 去掉。

## 要裁的

无。两格都在编排层且不碰 ADR-0135 决定句：做法 1 是把决定四已有的判据补到漏掉的入口，做法 2 是把决定六「被更正的那一代必须恰是链尾的依据」限定在链上有东西可比的情形。

## 红线

- 不动 `domain/**`、`adapters/postgres/**`、`adapters/http/**`、`cmd/**`；不加任何列、不改任何结果词。
- 不在编排里复述段那一侧的口径：首次判据仍只问 `objectUnderControl`（`FindActiveSegments` 非空即在控），与 `enterFulfillmentSegment` 同一判据。
- 显式指名的回指语义不变；链上有版本时自动派生回指的语义不变（既有用例「a correction of a version that is not the current basis」原样绿）。

## 完成判据

1. 链尾待确认、对象已在控：同一依据再来且身份已在册 → `NOT_FIRST`，版本行数不变、意图不发、链尾仍是那一版待确认；另一依据指名在册主体 → 同；更正代回指待确认的依据且读法仍为收寄 → 同。三格各有用例。
2. 轨迹来源、命令未指名回指、链上无版本、事实回指从未被判的前代：读法为收寄且主体在册 → `PICKUP_FORMED`，首版不回指、进段、发意图；读法不为收寄 → `NOT_A_PICKUP` 不落版本。显式指名回指而链上无版本 → 仍 `BASIS_NOT_CURRENT`。各有用例。
3. `rederive` 不再有 `_ = object`。
4. `gofmt -l` 空、`go build ./...` / `go vet ./...` 全仓退 0；`go test -count=1` TF application + `cmd/parcel-api`（带 DSN）+ `./internal/architecture/...` 绿；机制清点若变在干净检出重生成（本票不增删文件，预期不变）。
5. 完成记录逐笔 SHA、验证强度（PASS / SKIP）。

## 地盘

`internal/transportfulfillment/application/judge_carrier_first_effective_pickup.go` 与其测试；本票面；lc spec 子票表一行。**不动**其它任何文件。同时在途：通道 3 lc/28（PS + `cmd/parcel-api`）、通道 4 pc-gaps/13（PC）、通道 5 sa-cc/03（CC + SA 只读口 + dispatch）——零重叠。

## 参照

票 `31` Comments「评审 ← 通道 2」；ADR-0135 决定四（「非首次……不形成版本」）、决定六（「被更正的那一代必须恰是链尾的依据」）；TF CONTEXT「实际承运商首次有效收寄」生命周期与「已处于运输控制中时新到承运证据不构成首次」；`judge_carrier_first_effective_pickup.go`（`Judge` / `reconsiderPending` / `rederive` / `formOrHold` / `resolveBasis` / `objectUnderControl`）。

## Comments

- 2026-09-10 20:4x · 通道 6（task-ea5ac39e，分支 `mcp6-lc33` 基 `b40b1804`）：立票并认领，Status 直接 in-progress（作者自立自做）。要裁的为零。
- **完成记录（推送方代写，2026-09-10 20:2x 本机时钟；作者通道 6 在带 DSN 作者验与完成记录之前 crash，用户告知「工作完成了」）**：分支 `mcp6-lc33@2ed45954`（= origin，代码 tip 亦 `2ed45954`），三笔：`cca9dca1` 立票（20:06）→ `a55dbedc` 做法 1（待确认 → 已形成前补问首次判据：`Judge` 主路放宽成「链上无版本或链尾不是已形成」、`reconsiderPending` 在读法为收寄之后解主体之前问、`rederive` 在链尾不是已形成且读法仍为收寄时问；在控答 `NOT_FIRST`、不落版本、不发意图、待确认链尾留原样；`_ = object` 去掉）→ `2ed45954` 做法 2（`pickupBasisInput.correctsDerived`：自动派生且链上无版本 → 正路首登不回指；显式指名或链上有版本但依据不同 → 仍 `BASIS_NOT_CURRENT`）。**完成判据**：1 ✓ 三格用例共用 `assertNotFirstAndUntouched`（`CarrierPickupNotFirst`、行数 1、意图 0、链尾仍 `CarrierPickupPending`）；2 ✓ 三格用例断言到 `CarrierPickupFormedOutcome`（首版 `Supersedes()` 空、业务时间取 v2 有效时间、意图 1、已进段）/ `CarrierPickupNotAPickup` / `CarrierPickupBasisNotCurrent`；3 ✓；4 ✓ 见下验证（作者只来得及跑不带 DSN 的 TF 全部 + architecture，带 DSN 那一半由推送方全量兑）；5 本条。**红线核**：只动 `judge_carrier_first_effective_pickup*.go` 两文件 + 票面 + spec 一行；ADR-0135 / TF CONTEXT 零改动；夹具全合成。**能力边界**：/tdd 的红未单独可见——两笔各把测试与实现同笔提交（评审 Spec ④ 指出）；作者自己的判断题没来得及写。
- **评审 ← 通道 4 · 钉 `2ed45954`（基线 `b40b1804`，三笔，4 文件）· 2026-09-10 20:2x（评审自标 21:4x）**（task-dbb45a16，非作者，隔离只读检出 `%TEMP%\idp-review-lc33`；由推送方代落）。评审侧验证（无 DSN）：`go test -count=1` TF application + architecture 两包 ok；gofmt 空；vet 0。
  - **Standards**：**阻断 无**。**非阻断 2**：(1) `Judge` 里 `if found && current.Pickup.Formed() { return NotFirst }` 之后紧接 `if !found || !current.Pickup.Formed() { answerIfNotFirst }`——条件恒真（前一句已排掉另一半），可无条件调用，头上那句注释值得留；可读性判断。(2) **行为位移未在票面点名**：`reconsiderPending` 把 `answerIfNotFirst` 放在 `resolveSubject` 之前，于是「链尾待确认 + 对象在控 + 身份仍未在册」从此答 `NOT_FIRST` 而非旧的 `PICKUP_PENDING`（两者都不落版本、链尾不动）；按 ADR-0135 决定四是真话，但完成判据 1 只写了「身份已在册」那格——本条完成记录据此补记。**无发现**：注释全中文、无行号、无跨文件计数；ADR-0135 与 TF CONTEXT 零改动；夹具全合成（EXTF-1 / EXTV-2 / party/carrier-y / PCL-1）；不动 domain / postgres / http / cmd；首次判据仍只问 `objectUnderControl`；`_ = object` 已删且 `object` 真被用上。
  - **Spec**：**阻断 无**。**非阻断 1**：完成判据 5 缺（作者 crash，本条代补）。**已核实**：① 做法 1 三个入口都补了，在控答 `NOT_FIRST` 不落版本不发意图、待确认链尾不失效（理由：失效也是一版，领域 `Void` 只从已形成长出）——与决定四相容、决定五不受影响；② 做法 2 的区分在字段上成立（`correctsDerived` 只在 `resolveBasis` 且命令未指名、轨迹事实 `Supersedes()` 有值时置 true），判断只在 `Judge` 一处 switch，「链上无版本」判据 = `FindCurrentByObject` 的 `!found`；正路首登与领域构造门一致；③ ADR-0135 决定六原句未动，链上有版本时自动派生回指语义不变，既有用例原样绿；④ 用例断言到具名结果词，对旧代码红可推但与实现同笔、红未单独可见；⑤ `_ = object` 已删；⑥ 无真实状态码 / 渠道。另核：显式指名与自动派生同时在场时显式优先——「显式指名语义不变」红线成立。
  - **结论**：两轴 0 阻断（Standards 2 / Spec 1 非阻断），可重放。
- **进 main 记录（通道 1 推送方，2026-09-10 20:2x 本机时钟）**：隔离 detached 树 `%TEMP%\idp-replay-lc33` 在 main `0e575072` 上 pick 三笔全干净（`b40b1804..0e575072` 与本票文件零重叠）：`cca9dca1→27695ab6`、`a55dbedc→2e3405ab`、`2ed45954→1d1d405c`；机制清点重跑零差（本票不增删文件）。**验证钉 `1d1d405c`**（= 三笔重放 tip）：`gofmt -l` 空；`go build ./...` / `go vet ./...` 退 0；带 DSN `go test -p 1 -count=1 ./...` **105 ok / 0 FAIL / 15 无测试 / 0 cached**（20:18:37→20:20:42）；TF application `-v` PASS 408 / SKIP 0。分支 `mcp6-lc33@2ed45954` 作封存出处、改名 `merged/`；树 `D:/tops/idp-parcel-mcp6-lc33` 干净、通道 6 无会话 → 推送方比内容后拆。评审非阻断随票记：Standards (1) 一处恒真条件可随下笔简化；(2) 已补进完成记录。
