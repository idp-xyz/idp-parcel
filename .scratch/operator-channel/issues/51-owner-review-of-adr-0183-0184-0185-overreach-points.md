# 51 ADR-0183、0184、0185 越权风险点的 owner 复核（代决）

Category: task
Status: resolved——2026-10-19 进 main（纯文档），见「进 main 记录」。此前 in-progress——2026-10-19 子代理完工，待评审。此前 in-progress——2026-10-19 会话 `idp-parcel-9b` 认领（分支 `claude-oc51`）。此前 ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 立并代决（票 26 研判完整性批评点出）
Blocked by: 无
父票：[26](./26-non-author-review-backlog-for-claude-code-landings.md)
出处：研判轮完整性批评：三份 ADR 的越权风险点至今没有 owner 复核行
地盘：
- docs/adr/0183-*.md（只在越权风险点旁加复核行）
- docs/adr/0184-*.md（同上）
- docs/adr/0185-*.md（同上）
- .scratch/owner-review-queue.md（生成物，按其脚本重生成）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」。

## 做什么

照 `.scratch/owner-review-queue.md` 头行的流程，对三份 ADR 末尾的越权风险点逐条复核：认可就在原文旁写「owner 复核 2026-10-xx 认可（用户 2026-10-16 授权「你作为业务和系统专家你来自决」，会话 idp-parcel-9b 代裁，逐条）：1. …」，每条一句理由；不认可的走 supersede（新 ADR 部分停用），不改写原文。

与在途票的关系：ADR-0184 风险点 2（逐字比对）的事实更正与范围归 ps-external-mark-relations/04；「唯一命中算不算可靠识别」归 ps-external-mark-relations/05；ADR-0183 决定五被 ADR-0187（operator-channel/38）部分停用；ADR-0183 风险点 1、2 的复核要与 38 的证据落册裁定一致。

## 完成判据

- [ ] 三份 ADR 的每一条越权风险点都有复核行或 supersede 指针。
- [ ] 复核理由各自引原文依据，不复述别处权威。
- [ ] owner 复核队列重生成后这三份显示「已有 owner 复核记录」。

## 完成记录（2026-10-19，会话 idp-parcel-9b 的子代理，分支 claude-oc51）

复核行都写在各 ADR「越权风险点」列表之后，以「owner 复核 2026-10-19 认可」起头；Decision 与其余各节一字未改。

- **ADR-0183**：风险点 1 至 4 全部认可。1 请求方只取委托客户账户，与 pc-gaps/13 裁决 ③ 同向，「代」的可查性由票 38 落请求证据补足；2 责任主体只进审计，与票 38 对两份证据的待遇同一格；3 种类段与`关闭责任来源`同形，册上无旧写法；4 复核记录已补读两票原文。附记：决定五首句不认可，走 supersede，由票 38 出 ADR-0187（待落）。
- **ADR-0184**：风险点 1 至 6 全部认可。1 接受时形成合 PS CONTEXT「只在被标识对象成为正式对象时形成」，唯一误关联归票 ps-external-mark-relations/05 与 ADR-0195；2 维持逐字，范围按票 04 裁定（去两端空白只在接单入口，号码字符不动，核对读口不折算）；3 号码标识成员不是版本内容；4 领域不加门，Intake 同草案不重复不升格为产品策略；5 两写入方写不同类型的行，合决定三；6 客户参考号无消费方。附记：决定六按票 04 维持。
- **ADR-0185**：风险点 1 认可：合 PS CONTEXT「形成新的关联，不复活旧的」，并进旧关联会让后一笔作废选不中。
- **队列**：本机有 pwsh，但脚本按 Windows 路径写，原样跑会 `cannot change to '\home\…'`；把路径分隔换成 `/` 的副本放在会话草稿区跑，脚本本身不改。重生成钉 `980c16d5`，三份都显示「已有 owner 复核记录」。旧版钉在 `37f8050a`，这次的 diff 还带进此后全部 ADR 与票面的变化。
- **能力边界**：读过三份 ADR 全文；票 operator-channel/38「裁定」节，ps-external-mark-relations/04 的「为什么」「裁定」「做什么」与条目草案，ps-external-mark-relations/05 的「要裁的一问」与「裁定」；PS CONTEXT 两句引文；`FindParcelsByExternalIdentifier` 只核了它在哪里定义。没读：pc-gaps/13、label-channel/30 票面原文（依 ADR-0183 复核记录的转述）、UC-NO-002、ADR-0151、ADR-0060，各处代码实现。
- **没做**：ADR README 里 ADR-0184、ADR-0185 两行的「越权风险点……待 PS owner 复核」摘要已变旧，不在本票地盘；ADR-0184 那一行由票 ps-external-mark-relations/04 按其「做什么」第 5 条改写，ADR-0185 那一行留给分诊。
- 评审：待会话读 diff 审。

## Comments

- **评审 ← 会话 idp-parcel-9b 读 diff 审（不算非作者评审）· 钉 `4d321eb8`（基线 `9d9ac2d4`）· 2026-10-19**：复核由本会话起的子代理代裁、会话读 diff 审，如实记下。三份 ADR 只在越权风险点后加一行复核，Decision 与其余各节一字未动；11 条风险点全部认可，各引原文依据，与 ps-external-mark-relations/04、05 与 operator-channel/38 的裁定一致；ADR-0183 决定五首句的不认可指向票 38 的 ADR-0187（尚未落）。会话补改 `docs/adr/README.md` 里 0184、0185 两行的「待 PS owner 复核」（`4d321eb8`）。owner 复核队列用改了路径分隔符的脚本副本重生成，钉在 `980c16d5`，因旧队列钉在 9 月，差里带进了此后全部 ADR 与票面变化。

### 进 main 记录（2026-10-19，会话 idp-parcel-9b）

- 纯文档（三份 ADR 的复核行、ADR README 两行、owner 复核队列、本票），不涉 `.go` / `.sql`，未跑构建与全量；推送前核过 `origin/main..<推送 SHA>` 只有本票各笔。
