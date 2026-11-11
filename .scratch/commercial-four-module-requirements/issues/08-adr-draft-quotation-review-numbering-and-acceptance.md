# 08 ADR 草案「对客报价」的非作者评审、编号与接受

Category: enhancement
Status: draft
父票：[spec](../spec.md)
Blocked by: 用户决定 Q5（见 [spec](../spec.md)「待用户确认」）。Q1 已由会话按行业做法代答、待用户认可

对应要求：Quotation 一行的前置。

## 要做的

草案在 [adr-draft-quotation-offers-a-set-of-prices.md](../adr-draft-quotation-offers-a-set-of-prices.md)，由本 spec 的作者会话起草，没有编号，没有经过评审，放在 `.scratch` 里而不是 `docs/adr/`：它还不是规则。

1. **先认可 Q1 的答案。** 草案的全部决定都建立在「报的是一套价」上。这一条没有可问的客户，由会话参考行业头部软件的公开做法代答，依据在[行业做法笔记](../industry-practice-notes.md)；评审时把那份笔记一并复核，尤其是标着摘录与未取到的几条。用户不认可这个答案，或日后有客户要的是逐票报价并锁价，草案不进评审，本票改为重写。
2. **非作者评审。** 重点看草案「越权风险点」里的这几条：
   - 报价对象归 `party-commercial`、价格内容留在 `parcel-pricing`，这条界划得对不对；新增的那条 `party-commercial` 读 `parcel-pricing` 的依赖有没有别的连带。
   - 价格生效申请不设自己的批准，与客户要求里的「申请」对不对得上。
   - 运营代录接受借 ADR-0116 的分法，适不适用。
   - 草案「起草能力边界」里写明没读的那几份，有没有与它相抵的地方。
3. **编号与落地。** 评审无阻断后，照 `docs/agents/workflow.md`「进 main 的步骤」：编号由推送方在落地时取，文件移进 `docs/adr/`；同一笔改草案「对已接受记录与现行文档的作用」里列的被改句子——`PILOT-SCOPE.md` 讲不对客户形成价格承诺的那一句、计价 `CONTEXT.md` Rules 里讲报价不进首发的那一句与「计算目的」词条的末句、`docs/application/README.md` 要补的那一句——并给 ADR-0012 挂来向指针，在冻结清单「记录」追加一行。新的领域语言与用例不在这一笔，归票 [09](./09-quotation-domain-language-and-use-case.md)。
4. **接受。** 范围取舍归用户；上下文边界归 `party-commercial` 与 `parcel-pricing` 的 owner。用户 2026-11-11 在会话里答的「都要做」，指的是起草这份草案，不是接受它；接受要用户另说。

## 完成判据

- 评审记录写明钉的 SHA、结论与每条越权风险点的判词。
- ADR 进 `docs/adr/` 并带编号；`go run ./tools/adr-index -check` 退 0。
- 被改的句子在同一笔里改成现行；票 09 的「Blocked by」随之解除。
- 评审否决，或 Q1 的答案被推翻时：本票写明结论，父票「两处读法」与票 09 至 13 照新结论改。
