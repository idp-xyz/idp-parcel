# 07 ADR 草案的非作者评审、编号与接受：业务参与方读口随行答类别

Category: enhancement
Status: ready-for-agent
父票：[spec](../spec.md)
Blocked by: 无——与前端各票互不相干

## 要做的

草案在 [adr-draft-party-categories-on-the-read.md](../adr-draft-party-categories-on-the-read.md)，由本 spec 的作者会话起草，没有编号、没有经过评审，放在 `.scratch` 里而不是 `docs/adr/`：它还不是规则。

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
