# 07 ADR 草案的非作者评审、编号与接受：业务参与方读口随行答类别

Category: enhancement
Status: resolved——2026-11-12 进 main（没有做评审，用户当日裁定）
父票：[spec](../spec.md)
Blocked by: 无——与前端各票互不相干

## 要做的

草案原在本目录上一层的 `adr-draft-party-categories-on-the-read.md`，由本 spec 的作者会话起草，没有编号、没有经过评审，放在 `.scratch` 里而不是 `docs/adr/`：那时它还不是规则。（2026-11-12 接受后移进 `docs/adr/`，即 [ADR-0378](../../../docs/adr/0378-business-party-reads-answer-categories-read-from-the-existence-of-rows-and-catalogue-reads-may-carry-facet-counts.md)，草案文件随之删掉；见文末完成记录。）

1. **非作者评审**。重点三处，草案的「越权风险点」各有一条：
   - 它与 [ADR-0372](../../../docs/adr/0372-a-catalogue-status-filter-is-put-out-only-for-a-status-read-directly-from-the-registers-own-rows.md) 的界划得对不对——类别是不是落在「有没有那一行」这一侧。评审若判它属于「领域规则在多行之上求出的结论」，出路是 ADR-0372 决定二·1 的写面落列，草案的候选丙要重估。
   - 草案的起草能力边界里写明没读的那几份（`internal/platform/cataloguepage` 的实现、客户账户册读面全文、读口的 HTTP 载荷、ADR-0077）有没有与它相抵的地方。
   - 分面计数进 ADR-0144 的答复形状，对其他上下文的读口有没有连带。
2. **编号与落地**。评审无阻断后，照 `docs/agents/workflow.md`「进 main 的步骤」：编号由推送方在落地时取，文件移进 `docs/adr/`，被扩充的两篇（ADR-0144、ADR-0372）各加前向指针，写法照 `docs/adr/README.md` 顶部；同一笔把草案「Consequences」里那一句写进 party-commercial `CONTEXT.md`。
3. **接受**。归 party-commercial owner。用户 2026-11-10 对本 spec 的各项决定答「你作为国际小包业务和系统专家，根据你以往的经验帮我决定吧」；这句话是对着页面重构说的，是否涵盖一篇改读契约的 ADR，由评审会话向用户确认一次，不默认涵盖。

## 完成判据

- 评审记录写明钉的 SHA、结论与每条越权风险点的判词。
- ADR 进 `docs/adr/` 并带编号；`go run ./tools/adr-index -check` 退 0。
- `CONTEXT.md` 里有类别的定义句；票 08 的「Blocked by」随之解除。
- 评审否决或大改时：本票写明结论，父票「类型由谁算」一格与票 08、09 照新结论改；管理台停在票 01 的降级形态，那是可以长期停留的形态。

## 完成记录（2026-11-12，直接推 `main`）

- **评审没有做。** 本票「要做的」第 1 步是非作者评审，第 3 步要向用户确认一次「此前的授权是否涵盖一篇改读契约的 ADR」。2026-11-12 会话把这件事单独问了用户（三个选项：接受草案不评审并做完 07→08→09；这条线不做；先让隔离子代理评一遍），用户选第一项，同日并说「不需要评审和验收了，你直接测试通过就行了」。所以完成判据第一条（评审记录、钉的 SHA、每条越权风险点的判词）没有东西可交：三处评审重点原样写进了 ADR 的「越权风险点」，标明没有人评过、待 party-commercial owner 复核。
- **编号与落地。** 取号 0378（落地时 `docs/adr/` 里最大的是 0377）。草案移进 `docs/adr/` 成 [ADR-0378](../../../docs/adr/0378-business-party-reads-answer-categories-read-from-the-existence-of-rows-and-catalogue-reads-may-carry-facet-counts.md)，`.scratch` 里的草案文件删掉，父票与票 08 里指向草案的链接改指正式文件。ADR-0144 与 ADR-0372 各在 `Status` 行的索引注记末尾与 Links 节加了前向指针（「扩充」「延伸适用」，都不是停用），正文没有动。索引由 `go run ./tools/adr-index` 重新生成，`-check` 退 0；索引里这一篇的标签是工具从 `Status` 行派生的「代决·待 party-commercial owner 复核」。
- **`CONTEXT.md`。** party-commercial 的 `CONTEXT.md` 在客户角色那一句之后补了类别的定义句。
- **接受时定稿的几处**（ADR 里有同名一节）：两个新词取 `OUR_COMPANY`、`NO_ROLE`，段数取 `relationshipCounts`（`asHolder`、`asCounterparty` 各带 `live`、`candidate`）；分面答词表里的每一个值，为零的也答；**不补索引**——草案写要补，接受时读了迁移 `0047_catalogue_read_pagination_indexes.sql` 的头注：这几本册的读面先按租户整册折叠到最新修订再连接，按所指参与方的索引没有读者。
- 接受时比起草时多读了什么、仍没读什么，写在 ADR 的 `Status` 行里。
- 票 08 的「Blocked by」已解除。
