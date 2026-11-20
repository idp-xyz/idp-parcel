-- 提交版本带上成版时读到的成员实测毛重。
--
-- 申报资料快照带不了节点称出来的毛重。成版时按成员读当前有效实测，有则记下数值、单位、来源身份与发生时间；
-- 没有则这一件不出现。已有行成版时没读过实测，默认空数组，读回就是没有。

ALTER TABLE customs_compliance.declaration_submission
    ADD COLUMN member_weights jsonb NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE customs_compliance.declaration_submission
    ADD CONSTRAINT declaration_submission_member_weights_array
        CHECK (jsonb_typeof(member_weights) = 'array');
