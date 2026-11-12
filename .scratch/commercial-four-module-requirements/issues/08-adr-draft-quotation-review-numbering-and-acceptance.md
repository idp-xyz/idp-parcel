# 08 ADR 草案「对客报价」的非作者评审、编号与接受

Category: enhancement
Status: ready-for-agent
父票：[spec](../spec.md)
Blocked by: 无。原来等票 [15](./15-one-authority-for-which-price-a-customer-uses-and-the-customer-price-structure.md)；它的结论是 [ADR-0380](../../../docs/adr/0380-the-customer-contract-version-declares-ordered-sell-price-sources-and-settlement-hands-them-to-pricing-from-the-fixed-resolution.md)，草案「等票 15 定的」一节现在可以补。Q1 与 Q5 用户 2026-11-11 已答（见 [spec](../spec.md)「用户的决定」）：报的是一套价；报价进首发试点范围

对应要求：Quotation 一行的前置。

## 要做的

草案在 [adr-draft-quotation-offers-a-set-of-prices.md](../adr-draft-quotation-offers-a-set-of-prices.md)，由本 spec 的作者会话起草，是第二稿，没有编号，没有经过评审，放在 `.scratch` 里而不是 `docs/adr/`：它还不是规则。

1. **形态与范围已由用户定。** 用户 2026-11-11 认可「报的是一套价」（Q1），并定报价进首发试点范围（Q5）。Q1 没有可问的客户，由会话参考行业头部软件的公开做法代答，依据在[行业做法笔记](../industry-practice-notes.md)；评审时把那份笔记一并复核，尤其是标着摘录与未取到的几条。日后有客户要的是逐票报价并锁价，草案不进评审，本票改为重写。
2. **照票 15 的结论补完草案。** 草案「等票 15 定的」一节列着：报的价格内容是什么形状、生效申请与那道门落在哪一处、所引价格内容的计价范围与客户的关系。补完之前不进评审。
3. **非作者评审。** 重点看草案「越权风险点」里的这几条：
   - 「报价审批」读作价格内容的批准，不另立审批职责规则，与要求对不对得上。
   - 报价在发出时才成立，批准之前没有可以流转的报价单，够不够。
   - 发出与答复归 `party-commercial`、价格内容留在 `parcel-pricing`，这条界划得对不对；新增的那条 `party-commercial` 读 `parcel-pricing` 的依赖有没有别的连带。
   - 价卡草稿加一格`已撤下`，接上的是 ADR-0101 的哪一句。
   - 运营代录接受借 ADR-0116 的分法，适不适用。
   - 草案「起草能力边界」里写明没读的那几份，有没有与它相抵的地方。
4. **编号与落地。** 评审无阻断后，照 `docs/agents/workflow.md`「进 main 的步骤」：编号由推送方在落地时取，文件移进 `docs/adr/`；同一笔改草案「对已接受记录与现行文档的作用」里列的被改句子，并给 ADR-0012 与 ADR-0101 挂指针，在冻结清单「记录」追加一行。被改的句子是：
   - `PILOT-SCOPE.md`「明确不包含」一节讲不对客户形成价格承诺的那一句。照 Q5 的答案，它收窄到逐票价格承诺与锁价；对客报一套价的发出、答复与生效写进「已确认目标」一节。
   - 计价 `CONTEXT.md` Rules 里讲报价不进首发的那一句，与「计算目的」词条的末句。
   - `docs/application/README.md` 要补的那一句。

   新的领域语言与用例不在这一笔，归票 [09](./09-quotation-domain-language-and-use-case.md)。
5. **接受。** 上下文边界归 `party-commercial` 与 `parcel-pricing` 的 owner。用户 2026-11-11 的答复定的是形态与范围，不是草案的文本；接受要用户另说。

## 完成判据

- 评审记录写明钉的 SHA、结论与每条越权风险点的判词。
- ADR 进 `docs/adr/` 并带编号；`go run ./tools/adr-index -check` 退 0。
- 被改的句子在同一笔里改成现行；票 09 的「Blocked by」随之解除。
- 评审否决，或 Q1 的答案被推翻时：本票写明结论，父票「两处读法」与票 09 至 13 照新结论改。

## Comments

- 2026-11-11：用户认可 Q1、定 Q5 为进首发试点范围，并答「q6 全部激活」。本票不再等 Q5，转 `ready-for-agent`，仍等票 15。
- 2026-11-11：草案同日改成第二稿，报价不再有自己的状态、审批职责规则与价格生效申请对象。本票加一道阻塞（票 15）与一步（照票 15 的结论补完草案），评审重点随之换过。
- 2026-11-12：票 15 的结论是 ADR-0380，本票解除对它的阻塞。「要做的」第 2 步要补的三项，ADR-0380 给出的落点是（见它的 Consequences）：报的价格内容仍是销售价卡草稿，发到客户自己的计价范围里；所引价格内容的计价范围与客户的关系，是客户合同版本的销售价格来源指名这个范围；「价格生效申请」是两个现有发布之一——合同已经指名这个范围时是价卡的发布，还没有指名时是新一版合同的发布。「发布以接受为前提」的门设在哪一个发布上，ADR-0380 没有定，仍归本票的评审。另有一条限制要写进草案：给一位已登记的客户换来源列表，在 `product-strategy-boundary/06` 解冻之前走不通（ADR-0380 越权风险点 2）。
