# 13 SA 对账单「已作废 → 已替代」的替代关系在代码里不存在

Category: enhancement
Status: ready-for-human——2026-10-27 冻结到首个租户上线之后（线外，复核见 .scratch/launch-line-freeze-2026-10-27.md 第六节）：SA 对账单「已作废 → 已替代」缺替代关系——整单作废首发少见，作废留痕齐全，财务靠备注对上；解冻条件：首次整单作废后财务对不上时。此前 ready-for-agent——R3（不急）：整单作废首发少见，作废本身留痕齐全；缺的是「新单替代了哪一张」这条关系，财务要靠备注对上。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无
出处：[audit/sa-cr.md](../audit/sa-cr.md) L7（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，现象仍在）。

## 现象与证据

- 文档：SA CONTEXT Lifecycles「对账单」：「已作废 → 已替代：需要重新出具时形成新对账单并关联原单」；Rules：「整单无效时保留原单并建立作废和替代关系」。
- 代码：`internal/settlementaccounting/domain/customer_statement.go` 注释「整单确实无效时作废留痕，替代单用新身份（AT-SA-069），这里没有替代方法」；`adapters/postgres` 的 `CustomerStatements.Replace` 只写作废留痕那几列。

## 三类

机制。无产品策略、无租户取值。

## 规模

S–M：新单形成时可选回指被作废的原单，原单读出「已被某单替代」；迁移加一列。

## 最小的诚实修法

截单发布命令可带「替代的原单」；原单必须已作废、同账户同期间；新单落库同笔记下替代关系，原单不改写。

## 要裁的

无。
