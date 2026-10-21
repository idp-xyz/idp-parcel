# 167 承运答复挡回册没有查阅面：运营分不清订舱不在册的那一答是「不归你答」还是「还没录」

Category: enhancement
Status: ready-for-agent——2026-10-21 票 [145](./145-booking-answers-from-a-non-provider-answer-booking-not-found.md) 的作者子代理立（用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」与常设指令「你作为业务和系统专家自决……解决所有的问题」授权）。出自 [ADR-0239](../../../docs/adr/0239-a-carrier-answer-from-a-non-provider-is-answered-booking-not-found-and-the-basis-is-kept-on-a-bounce-register.md) 决定二末条与越权风险点 1。
Blocked by: 无（文件与票 [48](./48-booking-requests-and-carrier-answers-have-no-read-face.md) 同属 tf-chain-plan「并行车道」车道 A，落地顺序随车道 A 的令牌；与 48 同做或先后都可）
地盘：TF `ports/catalogue_read.go`、`adapters/postgres/review_catalogue.go`、tfhttp `query_transport_fulfillment_records.go`（+ 各自测试）；不碰承运答复口与挡回册的写口。

## 现象

票 145 之后，承运答复口对「订舱在本租户下不在册」与「答复方不是该订舱所属委托的服务提供方」答同一格 `BOOKING_NOT_FOUND`，凭什么挡回只记在承运答复挡回册（`transport_fulfillment.booking_answer_bounce`，ADR-0239 决定二）。承运方拿到这一答，续办是找租户核对；运营要回答它，就得看挡回册——可 `GET /transport-fulfillment-records` 没有这本册的 `registry`，管理台也没有页，今天只能经库查。

## 做什么

- 查阅口加 `registry=booking-answer-bounce`：每行一个（答复方、订舱、答复标识、依据），带首次与最近一次挡回时刻、次数；按最近一次挡回时刻倒序。
- 依据按 `ports.BookingAnswerBounceBasis` 的词透出（`BOOKING_NOT_REGISTERED`、`SOURCE_NOT_SERVICE_PROVIDER`）；库里存的是定宽码，由仓储一处译，读面不另抄一份码表。这是运营读面，不是承运方读面：只挂操作者渠道，不得开给集成客户端族——开给承运方就把 ADR-0239 决定一合并掉的那一位又交了回去。
- 可按订舱、按答复方筛；有了票 48 的订舱查阅之后，订舱那一行能链到它名下的挡回。
- 只做查阅面。词条已由票 145 立在 TF CONTEXT 的 Language 一节（「承运答复挡回」），页面与查阅口的用词照它。

## 完成判据

- 真库：一个不是服务方的答复与一个订舱不在册的答复各落一行，查阅口两行依据不同、他租户读不到；同一份答复重交只见次数变。
- 读口不触发判断、不写任何行（查阅的性质，ADR-0077）。
- 集成客户端令牌调这一口被拒（认证族不对）。

## 三类

机制。规模 S。
