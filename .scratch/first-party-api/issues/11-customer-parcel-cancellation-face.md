# 11 取消口经首方渠道接通

Category: enhancement
Status: in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 claude-fpa11（代码最后一笔 4e1841b50）。此前 in-progress——2026-10-26 会话 `tops-20` 认领（前置 code/101、code/98 已进 main），分支 `claude-fpa11`；再此前 ready-for-agent——R1（上线前必修，用户 2026-10-26 定首发要这一口）。
Blocked by: 无（06、docs-consistency/code/101 取消生命周期闸门、code/98 均已进 main）
出处：用户 2026-10-26「都需要」（首个租户上线时客户经首方 API 要能撤回、取消、修订、补充、索赔）、「每个客户系统只代表它自己的账户（直客），只考虑这个吧」、「首发不要客户状态读口」、「首发不做限流」；技术取舍由会话 `tops-20` 代决（用户「你帮我决定吧」）。 计划见 [spec](../spec.md)「五口上线」与 [摸底报告](../customer-faces-scoping-2026-10-26.md)。

## 做什么

首方 `IntakeCancellation`、取消命令带账户（目标须属信封账户）、授权链采用版本源装配、演示租户登「客户系统可在收寄前取消」参考配置（取舍 C）、`endpoints_cancellation.go` 换真 Intake。

## 完成判据

- [x] 首方客户凭证经 `/v1` 该口提交，演示租户答成功；他账户目标答找不到、不泄露存在；未授予答 403；重放答同一结果（生产装配真库用例）。
- [x] 新 AT 行与 AT 盘点①表行；Go 门禁；全量用 `scripts/test-full.sh`；种子 `seed.sh` 退 0。

## 规模

M

## 完成记录

起点 `29ef8aa1e`；提交 `4e1841b50`（单笔）。

**改了什么**：首方 `CustomerCancellationIntake`（`internal/parcelshipment/adapters/http/customer_cancellation_intake.go`）、`CustomerCancellationAuthenticator`（`MintParcelCancellation`）、`buildCustomerCancellationIntake`；`endpoints_cancellation.go` 迁 `/v1/shipment-requests/parcel-cancellations`、挂 `PARCEL_CANCELLATION`、换真 Intake；旧路径进 `TestTheUnversionedFirstPartyPathsAreGone`，`customerFaceCommandPatterns` 清空删除；授权缝 `buildCancellationAuthority`（`ScopedCancellationContent` + `CustomerSelfCancellationParty` + 既有 `ServiceStageRulesAdapter`）；PC 域只读方法 `CommercialRegistry.EffectiveVersionsAt`；PS 域 `NewCustomerAccountCancellationRequester`；管理台取消页改发 `/v1`；ADR-0313；契约一章；UC-PS-006 与 AT 盘点 `AT-PS-144` 至 `149`。无迁移、无种子改动、不碰 PS 应用层。

**本口形状（`PCN-1`）**：`originalAccessRequestId`、`shipmentRequestId`（必填）、`parcelId`、`reason`、`requestedAt`（RFC 3339，必填）进摘要；`accessRequestId` 铸信封（`parcel-cancellation:` 前缀）；租户、账户、取消者取自信封，载荷自报一律 400。

**代决与越权风险点**（详见 ADR-0313 文末）：
1. 取消授权不走接受时闭包：同范围授权规则天然多版（演示租户 `SYN-SCOPE-01` 接受时点有四版：价格 / 成本方向授权、资料修订授予载体、取消目录），列进必需依据会让接受答适用冲突，且定价的命名引用要求同范围、挪不出去。改为按接受时点、范围内「登记了取消目录」的唯一一版读（零版未配置、多版 `AUTHORITY_UNAVAILABLE`）。这是对 PC 读口缺口的处理：加了一个只读领域方法，未动 PC 端口与适配器；PC owner 复核，重启条件是闭包按用途分开采用授权规则。
2. 取消授权没有法人、等级、范围坐标（目录只按请求方格取），不适用资料修订的坐标映射；请求方格映射只认「客户账户本人」。
3. `requestedAt` 由载荷声明：取消库内容摘要含请求时刻，取服务端时钟则重放必冲突；契约写明重放须逐字节同载荷。
4. PCN-1 无处入库：`CancelParcelCommand` 没有摘要槽，判重由取消库的内容摘要承担；PCN-1 在 Intake 里只当载荷立得住的一道门。要入库须 tops-26 给命令加摘要字段。
5. `REQUEST_AWAITING_DECISION`、`REQUEST_ENDED` 答 200，不译 409（与首方族其它口业务答复一致）。
6. 取消的幂等键是（租户、原接入请求标识、包裹），取消请求自己的接入请求标识不参与幂等：换标识同内容重放读回同一份取消。
7. 演示租户参考配置沿用发布批已有的 `SYN-AUTH-CANCEL-01`（目录 `CUSTOMER` 与 `OPERATIONS` 两行，范围 `SYN-SCOPE-01`），无需改种子或解析键；用种子灌入的一次性库验过 `EffectiveVersionsAt` + 目录读取，范围内恰一版带目录。

**判据对用例**：成功 / 他账户 404 同字节 / 未授予 403 / 重放同结果 / 自报账户 400 / 改原因冲突 → `TestACustomerClientCancelsThroughTheFirstPartyLineAgainstARealDatabase`（`AT-PS-147`）；已撤回、已拒绝 `REQUEST_ENDED`、已提交 `REQUEST_AWAITING_DECISION` → `TestACustomerCancellationAgainstANonAcceptedRequestIsAnsweredByTheLifecycleGate`（`AT-PS-148`）；没登取消规则答未配置、真拒绝、两版并存 → `TestTheCustomerCancellationIsTheTenantsToConfigureAndSaysSoWhenItIsNot`（`AT-PS-149`）。单元：`AT-PS-144`（Intake）、`145`（`PCN-1`）、`146`（请求方格）。

**自验**（含 PG）：`go build ./...`、`go vet`、`gofmt`；带 DSN `go test -count=1 -p 1` 跑 `./internal/parcelshipment/...`、`./internal/partycommercial/...`、`./internal/accessidentity/...`、`./cmd/parcel-api/...`、`./internal/architecture/...`、`./migrations/...` 全绿；`seed.sh` 在一次性库退 0（种子未改，仅验参考配置）；`go run ./tools/at-coverage-counts -check` 只报生成物过期。未跑全量 `scripts/test-full.sh`（本票单口，按派单只跑受影响上下文）。
