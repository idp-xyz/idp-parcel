# 09 演示租户把冲突信号规则登成 `CUSTOMS_HOLD`：一次交接更正分叉在演示动线上变成「关务扣留」并可对客披露

Category: bug
Status: ready-for-agent——R2（上线后尽快）：错在随产品发布的参考配置（`SYN-TENANT-01` 种子），不是机制；真实租户自己登冲突信号规则，但照抄参考配置的会继承同一个错。演示动线上一次替代链分叉就会对客说成关务扣留——对客答错，限于演示租户与照抄者。绕法：重登一条中性类型的冲突信号规则。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/ve.md](../audit/ve.md) VE-16（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，种子未改）。

## 现象

冲突信号只由投影派生在替代链分叉裁不了时形成，来源可以是运输交接、交付等任何事实，不是关务事实；种子却把它的信号类型登成 `CUSTOMS_HOLD`。同一类型在分诊、异常披露两份种子里另有去处，于是这一格在演示上会挂到既有关务案件上、并对 `SYN-ACCOUNT-01` 可披露为关务扣留。

## 证据

- 文档：VE CONTEXT「关务相关异常只能消费 `customs-compliance` 已经接受且范围明确的事实或判断……查验、扣留、税费或内部限制等正常程序事实也只有命中已登记异常或客户影响规则时才形成异常信号。」
- 配置与代码：
  - `scripts/demo-seeds/data/visibility/09-conflict-signal-rule-v1.json`：`"signalKind": "CUSTOMS_HOLD"`。
  - 冲突信号的唯一形成处：`application.DeriveProjectionHandler` 的 `raiseConflict`。
  - `02-triage-rules-v1.json` 里 `CUSTOMS_HOLD` 走 `ATTACH_TO_EXISTING`；`08-exception-disclosure-rules-v1.json` 对 `SYN-ACCOUNT-01` 可披露（内容 `SYN-VE-CONTENT-CUSTOMS-HOLD-V1`）。
  - `scripts/demo-seeds/README.md` 与票 seed-completeness/02 的组成表把这一行写成了「冲突信号规则一租户一条，钉在 `CUSTOMS_HOLD`」。

## 三类

产品策略（随产品发布的参考配置）。机制不动；租户取值不动（真实租户自登）。

## 规模

S：改种子三份与 README 一段；若信号类型是封闭词表，加一个中性类型词属机制，一并做。

## 最小的诚实修法

冲突信号登一个中性的来源冲突类类型；分诊规则给它一条去处（例如新立案件或交运营复核），异常披露规则对它默认不披露；同步 `scripts/demo-seeds/README.md`「对外怎么说」一段。

## 要裁的

- 信号类型词：VE 现有封闭集里有没有可用的中性类型；没有就加一格（与票 [demo-chain-gaps/124](../../demo-chain-gaps/issues/124-customs-layer-conflict-relation-and-tracking-signal.md) 要立的「关务进展待确认」信号区分开）。作者可代决。
