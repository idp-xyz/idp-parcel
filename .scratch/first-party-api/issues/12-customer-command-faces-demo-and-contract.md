# 12 五口演示种子收尾与客户契约补章

Category: enhancement
Status: in-progress——2026-10-26 作者完工，待隔离评审与进 main，分支 claude-fpa12（最后一笔 7108ca046）。此前 ready-for-agent——R1（上线前必修，用户 2026-10-26 定首发要这一口）。
Blocked by: 07–11
出处：用户 2026-10-26「都需要」（首个租户上线时客户经首方 API 要能撤回、取消、修订、补充、索赔）、「每个客户系统只代表它自己的账户（直客），只考虑这个吧」、「首发不要客户状态读口」、「首发不做限流」；技术取舍由会话 `tops-20` 代决（用户「你帮我决定吧」）。 计划见 [spec](../spec.md)「五口上线」与 [摸底报告](../customer-faces-scoping-2026-10-26.md)。

## 做什么

五口演示实走一遍、种子收尾；客户系统 JSON 契约（ADR-0140）补五口章节。

## 完成判据

- [ ] 首方客户凭证经 `/v1` 该口提交，演示租户答成功；他账户目标答找不到、不泄露存在；未授予答 403；重放答同一结果（生产装配真库用例）。
- [ ] 新 AT 行与 AT 盘点①表行；Go 门禁；全量用 `scripts/test-full.sh`；种子 `seed.sh` 退 0。

## 规模

S

## 完成记录

起点 `0ac66f1cc`，分支 `claude-fpa12`，提交 `7108ca046`（脚本、README、479 头注）。本票不改产品代码；契约章节五口都已随 07–11 写进 `docs/api/first-party-api-contract.md`，实走未发现契约与行为不符，契约未改。

### 改了什么
- 新 `scripts/demo-seeds/walk-customer-command-faces.sh`：取令牌走 `operator-channel/customer-token.sh`，两份委托现提；委托 A 停在已提交（补充、撤回），委托 B 轮询客户追踪视图到已接受（`IDP_PARCEL_WALK_ACCEPT_TIMEOUT`，缺省 90 秒，超时说明该起 `cmd/parcel-dispatch`）后修订、索赔、取消；两条 404、一条 400 反例。同一 UTC 秒重跑撞提交键、答 200 `EXISTING_RESULT`，脚本因要求 201 失败退出，头注写明。
- `scripts/demo-seeds/README.md`「首方客户渠道」第 10 条改成现状（五口都已开，逐口写靠哪份种子，写明撤回口对外开放以 `docs-consistency/code/102` 进 main 为前提）；「组成」表 `operator-channel/` 一格补脚本。`steps/479-access-customer-command-faces.sh` 头注同改。取值依据均已 `git grep` 核实：`data/access/18`、`19`，`data/governance/53`–`57`，发布批 `SYN-AUTH-SOURCE-AMEND-01`、`SYN-AUTH-CANCEL-01` 与 `sourceDataAmendment` 矩阵，`resolution-key-syn-account-01.json`，`data/visibility/04`、`05`。

### 实走（一次性库 `fpa12_walk`，用完已删）
命令：建库后用该库 DSN 跑 `seed.sh`（退 0）；`docker compose -f scripts/demo-seeds/operator-channel/compose.yaml up -d --wait`（发行方 `127.0.0.1:5558`）；`parcel-api` 监听 `:19212`，带首方三件与 `IDP_PARCEL_GOVERNANCE_SELF_AUTHORITY=SYN-AUTH/idp-parcel-pilot`；`parcel-dispatch` 带七个变量（间隔 2s）同库；`IDP_PARCEL_API_BASE=http://127.0.0.1:19212 scripts/demo-seeds/walk-customer-command-faces.sh`，一次通过：

| 步 | 答复 |
|---|---|
| 提交 A、B | 201 `SUBMITTED` |
| 补充 PSU-1 / 原样重放 | 201 `SUPPLEMENT_RECORDED` / 200 `SUPPLEMENT_ALREADY_HANDLED` |
| 撤回 PWD-1 / 原样重放 | 201 `FORMED` / 200 `DECISION_ALREADY_FORMED`（同一份 `withdrawalId`，脚本断言） |
| B 接受 | 追踪视图 `NO_VISIBLE_RESULT`（dispatch 一拍内接受） |
| 修订 PAM-1（`SENDER_PLACE` × `SUPPLEMENT`） / 重放 | 201 `RECORDED` / 200 `ALREADY_HANDLED` |
| 索赔 VCL-1（`SYN-CONTRACT-01`、`SYN-APPLICANT-01`、`LOSS`） / 重放 | 201 `CLAIM_RECEIVED` / 200 `CLAIM_EXISTING_RESULT` |
| 取消 PCN-1 / 重放 | 201 `PARCEL_CANCELLED` / 200 `EXISTING_RESULT` |
| 撤回、取消指不存在的原提交 | 404 `SHIPMENT_REQUEST_NOT_VISIBLE`（两处） |
| 撤回载荷自报 `customerAccount` | 400 `MALFORMED_REQUEST` |

他账户那一格（与不存在同字节）演示只有一台客户端，由下列生产装配真库用例钉，脚本不造第二个账户。无产品缺陷、无 5xx。收尾：api、dispatch 进程已停，库已删，发行方容器已 `down`。

### 判据对用例
1. 首方凭证经 `/v1` 成功、他账户答找不到、未授予 403、重放同结果、自报账户 400（`cmd/parcel-api` 生产装配真库）：补充 `TestACustomerClientSupplementsThroughTheFirstPartyLineAgainstARealDatabase`；撤回 `TestACustomerClientWithdrawsThroughTheFirstPartyLineAgainstARealDatabase`；修订 `TestACustomerClientAmendsThroughTheFirstPartyLineAgainstARealDatabase`；取消 `TestACustomerClientCancelsThroughTheFirstPartyLineAgainstARealDatabase`；索赔 `TestACustomerClientSubmitsAClaimThroughTheFirstPartyLineAgainstARealDatabase`。
2. AT 号段：补充 `AT-PS-130`–`133`、撤回 `AT-PS-134`–`137`、修订 `AT-PS-140`–`143`、取消 `AT-PS-144`–`149`、索赔 `AT-VE-208`–`212`。本票不加 AT，AT 盘点不动。
3. 全量 `scripts/test-full.sh` 未跑（本票只动脚本与文档，派单限定不跑全量）；种子 `seed.sh` 在改完 README 与 479 之后用 `--reset` 复灌，退 0。

### 已知边界
- 管理台撤回页、取消页（`apps/admin-web/src/pages/shipment-request/api.ts` 的 `withdrawShipmentRequest`、`cancelParcel`）已发向 `/v1/...` 路径，但草稿形状仍是旧的（`originalRequestKey`、`requesterReference`、`reasonReference`，取消还缺 `requestedAt`），与首方载荷（`originalAccessRequestId`、`reason`、`accessRequestId` 等，封闭解码）不符；两页带的是操作者令牌，首方族在 `/v1` 上对操作者令牌答 401。按现状这两页对首方口是走不通的，接首方载荷与首方凭证属后续票，本票不动 `apps/`。
- 撤回口对外开放以 `docs-consistency/code/102` 进 main 为前提（见 README 第 10 条）。
- 演示只有一台客户端，他账户 404 同字节不在走查脚本里演。

### 自验
`bash -n` 新脚本退 0；本机无 `shellcheck`。实走如上。`go run ./tools/at-coverage-counts -check` 退 0（无新报）。未跑 Go 测试（无 Go 改动）。
