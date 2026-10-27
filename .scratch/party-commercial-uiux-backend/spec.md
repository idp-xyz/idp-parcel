# 参与方与商业区管理台的后端缺口（party-commercial-uiux 第三段）

Category: enhancement
Status: in-progress——01、02 已做完待进 main；03–05 是大票（要 ADR 级取舍），尚未拆票，等用户决定做不做

出处：[party-commercial-uiux spec](../party-commercial-uiux/spec.md)「后续」第三段。2026-10-26 前端第二段（07–21）全部进 main 之后，用户答「需要」开第三段。第三段列的是管理台想做、却缺后端读面或后端契约的几项；前端只能如实写「读面还没有」，不在前端票里补。

用量约束：用户 2026-10-26 说过逐票走作者子代理加隔离评审「太消费 token」，之后只接 R1/R2、评审用 Sonnet、简报写到文件与函数。本批是用户明确点了要做的 R3 级读面缺口，所以先做两张小的，不派作者子代理，只一份隔离评审；03–05 较大，做完 01、02 在自然断点先问用户。

## 子票

| # | 标题 | Blocked by | Status |
|---|---|---|---|
| 01 | [客户合同目录行带出货主客户账户](issues/01-customer-contract-catalogue-carries-the-customer-account.md) | — | resolved（待进 main） |
| 02 | [产品—渠道映射的历史修订读口](issues/02-product-channel-mapping-revision-history-read.md) | — | resolved（待进 main） |
| 03 | 待批准发布稿的列表与读回口，批准、发布带期望内容摘要 | 要 ADR（并发与摘要绑定的取舍） | 未拆票 |
| 04 | 管理台按操作者授予隐藏写入口（读面交出本操作者持有哪几族授予） | 要 ADR（授予面向管理台暴露哪些、谁能问） | 未拆票 |
| 05 | 预览与发布的逐格问题改回问题码加参数，前端按码查中文 | 要 ADR（问题码词表与稳定性承诺） | 未拆票 |

03–05 的现状与前端先行的过渡见 party-commercial-uiux 的 03、11、19 号票与 spec 第三段。
