# 178 管理台运输履约查阅页：没有订舱区与承运答复挡回区

Category: enhancement
Status: in-progress——2026-10-26 会话 `tops-20`（Claude Code）认领（用户 2026-10-26「按你的建议开工吧」），分支 `claude-dcg178`（worktree `idp-parcel-dcg178`；前端切片，多会话下改在 worktree 做、落 main 时重放）。此前 ready-for-agent——2026-10-26 Claude Code 作者子代理按票 [48](./48-booking-requests-and-carrier-answers-have-no-read-face.md)、[167](./167-booking-answer-bounce-register-has-no-read-face.md) 隔离评审 Spec 非阻断 2 立（会话 `tops-20` 派单；用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」授权），照票 [18](./18-transport-preparation-consumes-the-initial-route.md) → [45](./45-admin-web-transport-preparation-section.md) 先例。分级 R2（上线后尽快）：两格读口随票 48、167 进 main，可运营人员只用管理台，今天只能拿操作者令牌直调接口；承运方拿到「订舱不在册」后的续办是找租户核对（ADR-0239 决定一），运营在页上查不到挡回依据，这一答就落空。不挡上线：答复口本身照常工作，挡回也照常记进挡回册。
Blocked by: [48](./48-booking-requests-and-carrier-answers-have-no-read-face.md)、[167](./167-booking-answer-bounce-register-has-no-read-face.md)（查阅口 `booking-request`、`booking-answer-bounce` 两格随它们进 main）
地盘：`apps/admin-web/**`，照 [workflow.md「前端切片」](../../../docs/agents/workflow.md#前端切片一人在-main-上直接做)在 `main` 上直接做。
出处：票 48 完成记录「代决与越权风险点」第 8 条、票 167 第 3 条；票 48、127、167 隔离评审（钉 `c428ad2fc`）Spec 非阻断 2；[ADR-0202](../../../docs/adr/0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md) 文末补记越权风险点 15 的管理台那一半（去处见其 Status 行「去处注」）；[ADR-0239](../../../docs/adr/0239-a-carrier-answer-from-a-non-provider-is-answered-booking-not-found-and-the-basis-is-kept-on-a-bounce-register.md) 越权风险点 1。

## 现象

票 48、167 进 main 之后，`GET /transport-fulfillment-records` 有两格新册：

- `registry=booking-request` 答 `BOOKING_REQUESTS_LISTED`，`bookings` 每行带订舱、委托、班次、申请量与单位、申请时刻、取消时刻（可缺）、状态词 `standing`、当前那一份答复 `currentAnswer`（可缺）与名下全部答复 `answers`。
- `registry=booking-answer-bounce` 答 `BOOKING_ANSWER_BOUNCES_LISTED`，`bounces` 每行带答复方、订舱、答复标识、依据词、首末挡回时刻与次数；可加 `booking=`、`source=` 筛。

管理台 transport-fulfillment-review 页没有这两区，运营在页上既看不到订舱是待确认、候补还是已确认，也分不清一份被挡回的答复是「不归它答」还是「订舱没录」。

## 做什么

- `records-api.ts`：册名加 `booking-request`、`booking-answer-bounce`，答案加两种 outcome 与行形；挡回册的两个筛选参数。
- 订舱区：每次订舱一行——订舱、委托、班次、申请量与单位、申请时刻、状态词（原词示文，页面不另算，读法只在读口）、当前答复（答复值、接受量、答复时刻）；取消时刻单列，不并进状态词；名下全部答复可展开，候补与它的后继都列出。
- 挡回区：每行答复方、订舱、答复标识、依据（「订舱不在册」「不是服务提供方」两词照 TF CONTEXT「承运答复挡回」词条示文）、首次与最近一次挡回时刻、次数；可按订舱、按答复方筛。订舱区的一行能链到挡回区按那一单筛出的结果。
- 空册照页上「登记册为空」的分词呈现。

## 完成判据

- 演示走到承运答复那一步之后，订舱区示干线那一单已确认；用末端承运客户端答干线订舱一次，挡回区多一行「不是服务提供方」，从订舱区那一行点过去只见这一行。
- 前端单测覆盖状态词各格、取消时刻单列、挡回两种依据与筛选参数。

## 三类

机制（查阅页如实呈现两本册子）。规模 S。
