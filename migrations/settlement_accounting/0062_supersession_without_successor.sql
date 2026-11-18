-- 重估为零额也替代前一笔未确认的预估：新一代评价合计为零时没有新预估，前一笔费用照样被替代、不再确认，
-- 所以替代关系可以没有后继。之后一代算出非零金额、形成新预估时，再把它补成这条替代关系的后继。
-- 只放开 superseded_by_charge_id 的非空约束；后继在场时，指向客户费用的外键与「不能替代自己」两道约束照旧
-- （两道约束遇到空值都不判错）。被替代的费用行不改数字。不改已有迁移，本迁移不种任何租户的行。

ALTER TABLE settlement_accounting.customer_charge_supersession
    ALTER COLUMN superseded_by_charge_id DROP NOT NULL;
