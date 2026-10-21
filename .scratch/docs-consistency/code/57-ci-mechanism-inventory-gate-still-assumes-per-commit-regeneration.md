# 57 CI 的机制清点门仍按各笔自己重生成的旧规矩挂在 PR 上，与生成物的现行规矩相反

Category: chore
Status: ready-for-agent——R3（不急）：今天不经 PR 落 main，这道门只在 main 推送上起作用，结论照旧对；一旦走 parallel-sessions「落 main 的下一形态」开 PR，每张新增文件或迁移的票都会按设计在这一门上红。2026-10-21 文档自洽 10（10b）立（用户「你作为业务和系统专家自决」授权），分支 claude-dc10b。
Blocked by: 无
出处：[audit/cross.md](../audit/cross.md) AG·M1（归 B，10b 代决）。取证于 main `4e7874ba0`。

## 现象

- `.github/workflows/ci.yml` 的 `Mechanism inventory is current` 一步没有 `if:`，PR 与 main 推送都跑；它下面那段注释写「ADR 索引与 AT 盘点计数也是生成物，但约定与上一步相反」，接着说那两件由推送方在 main 的 tip 上重生成、单独成笔。
- 10b 照各票派单「进 main 时统一做」的实际做法裁定三件生成物同一条规矩：票不在自己的笔上重生成，推送方在 `main` 的 tip 上重生成、各单独成笔（[parallel-sessions「生成物不占号」](../../../docs/agents/parallel-sessions.md#生成物不占号但只在干净检出上生成)、[workflow「进 main 的步骤」](../../../docs/agents/workflow.md#进-main-的步骤)）。按这条规矩，机制清点那一门挂在 PR 上就会按设计红，红就不再是信号——与 ADR 索引那一门的注释说的是同一个理由。

## 三类

机制（CI 配置与注释）。无产品策略、无租户取值。

## 规模

S。

## 最小的诚实修法

1. `Mechanism inventory is current` 一步加与下一步相同的 `if: github.event_name == 'push' && github.ref == 'refs/heads/main'`。
2. 下一步的注释「约定与上一步相反」改为「与上一步同一条规矩」，指向 parallel-sessions「生成物不占号」。
3. 上一步自己的注释（「清点报告因此不可能……落后于代码」）补一句：它守的是推送方漏了重生成那一步。

## 要裁的

无（规矩已由 10b 裁定，见票 `../issues/10-cross-cutting-docs.md`「10b 完成记录」）。
