# 管理台前后端接线审查的后续实施

Category: enhancement
Status: resolved——六张子票全 resolved；批 Owner 于 2026-09-04 判 04 的余下上下文不算本批余项（理由见文末「收口」）
Owner: MCP-2（用户 2026-09-03 授权「自决」）

## 出处

2026-09-03 在 `0d492b8` 上做的一次全面审查：前后端接线、页面完整性、后端 API 完整性。审查本身
不落文档（结论有保质期），只把**可实施**的余项落成票；结论里凡引用当时的数（75 端点、66 被页面
消费、30 条棘轮基线）都只对 `0d492b8` 成立，票面按需自己重取。

## 审查里落成票的四件

| 发现 | 票 |
|---|---|
| 前端零测试文件，四态判读、词表、行转写全无自动化保护；而加依赖被 `pnpm-lock.yaml` 的宿主问题拦着（见 `docs/agents/workflow.md` 本机环境） | [01](./issues/01-frontend-test-harness-without-lockfile-change.md) |
| `/commercial-policies?kind=CREDIT_POLICY` 后端已供（`0020_credit_policy.sql`、`serveCreditPolicies`），前端 `CommercialPolicyKind` 六格无它，页面文案仍称「信用政策无独立正文册」 | [02](./issues/02-credit-policy-register-on-commercial-policies-page.md) |
| 前端发出的 URL 与后端端点表今天逐条对得上（0 悬空），但没有任何东西守它；下一次漂移仍要靠人工审查撞见 | [03](./issues/03-admin-web-endpoint-consumers-gate.md) |
| 前端 TS 响应类型全部手写，后端 JSON 形状由 Go 单测钉住，两侧之间无共享夹具——票 02 那条漂移正是从这条缝长出来的 | [04](./issues/04-contract-fixtures-shared-with-backend-tests.md) |
| `pages/governance/` 下混放结算两页、VE 分诊页与 PS 受理复核页，与 `moduleInfoById` 声明的主责上下文不一致 | [05](./issues/05-relocate-pages-under-owning-context-folders.md) |
| TF 交接范围汇总四层俱在，`cmd/parcel-api/endpoints.go` 无它的行 | [06](./issues/06-mount-tf-handover-scope-summary-endpoint.md) |

## 审查里**不**落票的

- 六个无页面消费的端点（NO 收寄、TF 交付与更正、客户追踪视图、索赔受理、关务外部结果）——
  一线作业端 / 渠道 / 外部集成入口（ADR-0021），设计上不在管理台。
- `/commercial-publications` 登记签——已有票 `admin-write-faces/03`，阻在两套封闭集对不齐，不另立。
- 渠道账号使用授权两个写口无页面——**先要后端读面**（今天没有 GET），且在 MCP-5 地盘
  `internal/partycommercial/`；本批不动，记在此处供其票面承接。
- 真 Intake / 前端带 `Authorization`——实例半边（PAR-INT-01），不因开发方努力而闭合。

## 子票

01 → 02（02 的 TDD 依赖 01 的运行器）；03、05、06 相互独立；04 依赖 01 且其后端半边落在
MCP-5 地盘，本批只落前端半边或留票。

六张票的票面均已转 resolved（06 于 2026-09-03 由 MCP-1 收口）。**批状态留给批 Owner 判**：
04 是首切片收口，pricing 与 party-commercial 的同形夹具仍未做，那算不算本批的余项由 MCP-2 定。

## 收口（2026-09-04 · MCP-2）

判 **不算本批余项**，批转 resolved。理由：本批的出处是一次审查，票面只承诺「至少一个上下文的列表
响应有共享夹具，改任一侧字段名两侧都红」，04 已在 `networkrouting` 上兑现并做了反证；余下两个上下文
当时不做的原因都是**地盘**（`parcelpricing` 有另一会话在途、`partycommercial` 等 MCP-5 释号），
不是本批的工作没做完。夹具是逐上下文的形状，补哪个由该上下文的认领人在自己的票里带——04 的
Comments 已写明照做的形状与入口（`contract_fixture_test.go` + `*.contract.test.ts`），此处不另立票，
免得一张无人认领的伞票替别人排期。
