# 参与方与商业区管理台的后端缺口（party-commercial-uiux 第三段）

Category: enhancement
Status: resolved——01、02 已进 main（`51e154be0`）；用户 2026-10-26 答「全做」后 03、04、05 也已进 main（`e9742f271`、`44b3fbc20`、`914473999`）

出处：[party-commercial-uiux spec](../party-commercial-uiux/spec.md)「后续」第三段。2026-10-26 前端第二段（07–21）全部进 main 之后，用户答「需要」开第三段。第三段列的是管理台想做、却缺后端读面或后端契约的几项；前端只能如实写「读面还没有」，不在前端票里补。

用量约束：用户 2026-10-26 说过逐票走作者子代理加隔离评审「太消费 token」，之后只接 R1/R2、评审用 Sonnet、简报写到文件与函数。本批是用户明确点了要做的 R3 级读面缺口，所以先做两张小的，不派作者子代理，只一份隔离评审；03–05 较大，做完 01、02 在自然断点先问用户。

## 子票

| # | 标题 | Blocked by | Status |
|---|---|---|---|
| 01 | [客户合同目录行带出货主客户账户](issues/01-customer-contract-catalogue-carries-the-customer-account.md) | — | resolved（`51e154be0`） |
| 02 | [产品—渠道映射的历史修订读口](issues/02-product-channel-mapping-revision-history-read.md) | — | resolved（`51e154be0`） |
| 03 | [待批准发布稿的列表与读回口，批准、发布带期望内容摘要](issues/03-pending-publication-drafts-read-back-and-digest-bound-decisions.md) | ADR-0314（会话代决） | resolved（`e9742f271`） |
| 04 | [授予自查读口，管理台按操作者授予收起登记与发布入口](issues/04-operator-grants-self-read-and-write-entries-hidden.md) | ADR-0315（会话代决） | resolved（`44b3fbc20`） |
| 05 | [逐格问题改回问题码加参数，管理台按码查中文](issues/05-field-problems-carry-a-stable-code-and-params.md) | ADR-0316（会话代决） | resolved（`914473999`） |

03–05 是用户点了要做的三张 R3 级票，各带一份会话代决的 ADR（0314、0315、0316），待各自 owner 复核；要确认什么、不认可时改哪里见 [owner 复核清单](owner-review.md)。
