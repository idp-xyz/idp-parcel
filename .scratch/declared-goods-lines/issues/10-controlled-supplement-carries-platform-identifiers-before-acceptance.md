# 10 受控补充收平台号：受理前补报或更正平台货件号、平台参考号与平台箱标号

Category: enhancement
Status: ready-for-agent
Blocked by: 无

依据：[ADR-0376](../../../docs/adr/0376-fba-submission-conditions-are-judged-as-required-documents-platform-identifiers-may-be-supplied-before-acceptance-and-rule-packages-follow-the-service-product.md) 决定二（部分停用 ADR-0374 决定四一句）；PS CONTEXT「外部标识」词条。

## 要做的

- 首方受控补充口的载荷照提交口同形，收平台货件段（`platformShipment`）与逐件 `platformBoxLabel`；去掉「箱标号只在首次提交声明」的拒收。
- 补充重报整份内容：带了平台段，即这一版起委托的平台声明（整段替换）；没带，即这一版不报。形状规则与提交口同：平台与货件号同在同缺，箱标号要有平台货件，委托内不重号。
- 规范化 `PSU-1` 与提交共用编码（ADR-0373 决定八），平台段进补充摘要；旧的不带平台段的补充摘要逐字节不变。
- 领域：`FormNewSubmissionVersion` 收平台声明并替换委托上的现行声明，对着新成员集合核。接受后不变（补充本来只在受理前）。
- 被补充撤掉的箱标号不再算歧义信号（读快照即随之更新），并照样响声明撤除门铃，与拒绝、撤回同一条。
- 首方契约文档补充一节写明：平台段随补充整份重报，没带即清空。

## 完成判据

- 补充补报箱标号后，按号核对不再答歧义；补充撤掉的箱标号响撤除门铃；带平台段的补充重放答同一结果、改了平台段答来源冲突。
- 旧补充摘要的常量不变。

## 不做的

接受后改平台号（ADR-0376 决定二：撤回重下或关联新委托）。
