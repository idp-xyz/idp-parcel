-- 信用暴露行记下它据以判额度的信用政策版本（ADR-0127 Consequences 点名留给 SA 迁移的那一格；
-- CONTEXT「每项信用暴露保存实际采用的政策与范围」的信用政策那一份）。
--
-- 一列不是两列：settlement-accounting 拿到的政策引用是商业侧交来的一枚不透明标识
-- （CreditPolicyReference，与结算政策、控制策略的采用引用同形），本上下文只回指不拆解——
-- 拆成「对象 + 版本」两格就得让本上下文知道那枚标识里哪一段是什么，那是提供方的形。
--
-- 可空只为存量行：本迁移之前入库的暴露没有这一格，读回如实为空；新写入非空由应用层保证
-- （授权额度进不了没有出处的 WithAuthorizedLimit，暴露入册时带着它）。CHECK 不追溯存量行，
-- 只拒空白——一枚空字符串的「出处」比 NULL 更坏，它看起来像填过了。

ALTER TABLE settlement_accounting.credit_exposure
    ADD COLUMN credit_policy_ref text,
    ADD CONSTRAINT credit_exposure_policy_ref_not_blank
        CHECK (credit_policy_ref IS NULL OR btrim(credit_policy_ref) <> '');
