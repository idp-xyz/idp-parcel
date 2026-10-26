# 177 订舱当前答复的选法在读面与写面各实现一遍

Category: enhancement
Status: ready-for-agent——2026-10-26 Claude Code 作者子代理按票 [48](./48-booking-requests-and-carrier-answers-have-no-read-face.md) 隔离评审 Spec 非阻断 1 立（会话 `tops-20` 派单；用户 2026-10-20「要我裁定的部分，你作为业务和系统专家，请你自决吧」授权）。分级 R3（不急）：迁移 0029 的两条部分唯一索引兜着，凡能经答复编排落库的答复，两处选出的是同一份，今天没有答错；规则一变（例如 ADR-0202 越权风险点 13 放宽候补的形）两处才会分叉，到那时须与规则改动同笔改，不能先上。
Blocked by: 无
出处：票 48 完成记录「代决与越权风险点」第 5 条；票 48、127、167 隔离评审（钉 `c428ad2fc`）Spec 非阻断 1；[ADR-0202](../../../docs/adr/0202-transport-commission-booking-and-carrier-answer-entry-families-and-supplier-agreement-check.md)「补记：票 24 落地时的补充裁定」里「候补之后可接一份定局」一条的读法。
地盘：TF 领域 `domain/transport_commission.go`（`CarrierAcceptance` 一组）与它的测试、`adapters/postgres/booking.go` 的 `BookingAnswers.FindByKey`、`adapters/postgres/review_catalogue.go` 的 `bookingStanding`（+ 各自测试）。

## 现象

一次订舱名下「当前那一份答复」——有定局取定局，否则取候补——今天写在两处：

- 写面：`BookingAnswers.FindByKey` 用 SQL 排序选（`ORDER BY (outcome <> 'WAITLISTED') DESC, decided_at DESC LIMIT 1`），遇到绕过编排落库的异常行（两份定局、两份候补）静默取最新那一份；运输收费发生项的订舱事实经它读当前答复。
- 读面：票 48 的 `ReviewCatalogue.bookingStanding` 在 Go 里按答复值字符串比 `WAITLISTED` 选，遇到同样的异常行答错误、整页 500。

两处的选法今天等价，靠的是库上 `booking_answer_one_settling`、`booking_answer_one_waitlist` 两条部分唯一索引；一处单一权威写成了两份，异常行上的处置也不同。

## 做什么

- 把选法收进领域层：`CarrierAcceptance` 已有 `Settles()`（候补以外四值都是定局），在旁边加一个从同一订舱名下的答复里选出当前那一份的函数（有定局取定局，否则取候补；两份定局或两份候补答错误），作为唯一定义。
- `BookingAnswers.FindByKey` 改为读回名下全部答复再调它，不再靠 SQL 排序选；`bookingStanding` 也调它，用 `Settles()` 判，不比字符串。
- 两处对异常行的处置随之统一为答错误（不静默挑一份）；写面上这会让收费发生项登记在异常行上答没形成答案而不是按最新那一份登——实施时核一遍调用方的错误代数（ADR-0029），写进完成记录。

## 完成判据

- 领域用例：只有候补、候补加定局、只有定局、无答复、两份定局、两份候补各一格。
- 真库：`FindByKey` 与查阅口订舱册在同一组答复上选出同一份；把异常行直插进库，两处同样答错误。
- 全仓再找不到第二处按答复值字符串或 SQL 排序挑当前答复的写法。

## 三类

机制。规模 S。
