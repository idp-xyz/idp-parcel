# 93 价卡批准与商业发布批准两门先比主体，与参考复核门先判授予顺序相反

Category: enhancement
Status: needs-triage——建议 R3（不急）：三道门放行与拒绝的集合相同，差的只是两格都不满足时先报哪一格、给操作者的续办提示不同；没有放错一个人、也没有拦错一个人。2026-10-26 协调方据 [code/41](./41-reference-review-gate-reads-a-tenant-review-duty-rule.md) 隔离评审 N2 立。
Blocked by: 无
上下文：PP（价卡草稿批准）与 PC（商业发布载体批准），领域层批准门。**PC 那边另有会话在改管理台（party-commercial-uiux），动 PC 前须先与协调方对齐。**
出处：code/41 隔离评审 N2。

## 现象（读码，只记 `S`）

三道「录入者以外的第二个人」门由租户各登一条职责规则判，规则形状相同（须否不同主体、须持哪一格授予），两格的判断顺序却不一样：

- 价卡发布审批：`internal/parcelpricing/domain/price_card_draft_approval.go` 的 `PriceCardDraft.Approve` 先比主体（`ErrDraftApproverIsSubmitter`，`NEEDS_ANOTHER_APPROVER`），再判授予（`ErrDraftApproverLacksRequiredGrant`，`APPROVER_NOT_QUALIFIED`）。
- 商业发布审批：`internal/partycommercial/domain/publication_draft.go` 的 `PublicationDraft.Approve` 同为先比主体（`ErrApproverIsSubmitter`），再判授予（`ErrApproverLacksRequiredLevel`）。
- 参考复核（code/41）：`internal/parcelpricing/domain/reference_review_duty_rule.go` 的 `ReferenceReviewDutyRule.admit` 先判授予（`ErrReferenceReviewerLacksRequiredGrant`，`REVIEWER_NOT_QUALIFIED`），再比主体。理由：一道共用领域门、不按渠道区分时，只有这个顺序满足 UC-PP-001 `AT-PP-015`「经受控 CLI 复核而规则要求某一格授予时，不论复核者是谁，一律答复核者不合格」。

三门在两格都满足时放行、任一格不满足即拒，放行与拒绝的集合相同；只在两格都不满足（批准者就是录入者、又没有规则要求的那一格）时，两道批准门答须换人，参考复核门答不合格。须换人的提示让人只去找「另一个人」，而另一个人没有那一格照样过不去。

顺带：PC `ErrApproverIsSubmitter` 的注释「判据同 parcel-pricing 的四眼门」在 code/41 之后已过时——PP 的序列与目录复核不再写死四眼门，改按租户的参考复核职责规则判；价卡批准门也从来不是写死的四眼门。

## 三类

机制：门内判断顺序。产品策略：两格都不满足时报哪一格（续办提示）。没有租户取值。

## 规模

S：两道门各互换两段判断，各补一条两格都不满足的用例；PC 一处注释。

## 最小的诚实修法

二选一，裁后两门同做：

1. **对齐到先判授予**：`PriceCardDraft.Approve` 与 `PublicationDraft.Approve` 互换两段判断，各补一条「批准者就是录入者、且没有要求的那一格」答不合格的领域与编排用例；查两门既有用例里有没有钉住旧顺序的断言。
2. **一次报出两格**：三门都在两格都不满足时同时报两格（结果代数多一格或带原因集合），三处同改，代价大于 1。

两种都要顺手改掉 PC `ErrApproverIsSubmitter` 注释里指向 parcel-pricing 四眼门的那半句。

## 要裁的

选 1 还是 2（建议 1：改动最小，三门顺序一致即可，续办信息不丢——要找的人本就得既是另一个人、又持有那一格）。
