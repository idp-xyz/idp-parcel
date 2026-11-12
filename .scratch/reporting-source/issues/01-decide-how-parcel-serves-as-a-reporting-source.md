# 01 写 ADR-0384：本产品怎样做报表来源

Category: docs
Status: in-progress——草案已出（分支 `cursor/reporting-source-adr`），等产品 owner 读完作答

依据：

- 产品 owner 2026-11-11 在报表平台一侧的决定：IDP Parcel 是报表平台 IDP Reporting 的第一个真实来源；Parcel 一侧在本仓做，先出本仓自己的 ADR 给 owner 看，再实现；报表查询里的「公司」是责任法人。出处在 `idp-xyz/idp-reporting` 仓的 `docs/ADR_0027_AFTER_THE_DESIGN_REVIEW.md`，那个仓不是本仓的权威。
- [AGENTS.md](../../../AGENTS.md)「改文档」：难逆转的取舍走新 ADR。让另一个系统替某个人来读，是本仓各族接入渠道都没有的一种信任。

## 要做的

写 [ADR-0384](../../../docs/adr/0384-reporting-source-is-an-optional-capability-a-reporting-platform-client-names-an-operator-and-reads-frozen-operating-summaries-judged-by-the-operator-register.md)，`Status` 为 Proposed。

只加三份文件：ADR 本身、本目录的 `spec.md` 与本票。不动 `internal/**`，不动任何已接受的 ADR、开发主线、CONTEXT 与参数登记册——那些改动在 ADR 被接受的那一笔做（ADR-0384「接受时同笔改动的文件」），另立票。

## 完成判据

- ADR-0384 写明：谁在调，信什么、不信什么，凭据落到别人手里的后果；权限按哪本册、哪一格、哪一维裁；冻结落在哪、什么时候拒绝；对已接受记录的作用逐句引用并写明适用场景；候选与否决理由；越权风险点逐条指到 owner。
- 引别的文件的句子逐字；跨文件不用行号、计数、序数与位置词。
- 证据层级如实：全部取证只记 `S`；报表平台一侧的文档只作出处，不当本仓权威。
- owner 的答复写进本票 Comments；按答复修订 ADR 之后，再走 [workflow.md](../../../docs/agents/workflow.md)「进 main 的步骤」——非作者评审，推送方在 main 的 tip 上重生成 ADR 索引。草案期间只推分支，不进 main。

## 要 owner 答的

各条在 ADR-0384「越权风险点」里以同样的名字点了名。

1. **点名的信任**：接受「报表平台说是谁就查谁，他能看什么由本产品按操作者册现判」这一层信任吗？不接受，整份草案不成立。
2. **发行方是不是同一个**：报表平台验登录用的发行方，与本部署操作者族的发行方，是不是同一个？草案按同一个写；不是，就要另立一本主体对应册。
3. **法人范围**：「经营报表查阅」的授予按责任法人范围登记，从第一天起就带——同意吗？
4. **服务凭据**：报表平台持客户端凭据令牌出示，前提是那一侧的通用适配器先补上取与续令牌——同意两边一起这样做吗？
5. **部署与证据**：预发环境与第一个真实租户何时有？在那之前证据只到 `S`，真实对账做不了。

## Comments

- 2026-11-13 作者（Cursor 会话）：ADR 编号取 0384。取证于 `9bf6e8603`：main 上最大的号是 0383；本机各分支与 `origin` 各分支上，main 没有的 ADR 文件里没有更大的号。进 main 之前若被别的票先占，重编号并改文件名与各处链接。
- 2026-11-13 作者：起草时读过与没读的，列在 ADR 的 `Status` 行「起草能力边界」里。各上下文 owner 复核时先看越权风险点里讲线格式、数据集口径、证书绑定令牌、冻结读取的位置与公司目录的各条。
