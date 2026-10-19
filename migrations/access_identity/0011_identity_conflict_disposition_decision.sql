-- 「运营决定」能力面加一种决定：身份冲突处置（ADR-0151 决定一；ADR-0210 决定一，票 no-reception-identity/06）。复核页上对一件判
-- 身份冲突的作业实物指定一件候选或确认都不是，是管理台上作判断的口。与 accessidentity.DecisionIdentityConflictDisposition 同笔
-- 放开（0001 头注那条约定）。本迁移不种任何行：谁被授予这一种决定是租户取值。

-- 全集里预先列上 TRANSPORT_COMMISSION_SUBMISSION 与 TRANSPORT_CHARGE_OCCURRENCE_REGISTRATION：它们由另一组票的迁移
-- 0008、0009 加入，那两份编号在本份之前、却可能晚于本份进 main。新库按编号施加时本份排在它们之后，重建这条 CHECK 若
-- 只取本份基线上的全集，就会把那两种删掉、让它们的授予登不进。多列两个此刻 Go 侧还没定义的词无害：授予册只经领域
-- 词表写入，库面这条 CHECK 只守「不是封闭集外的串」。之后谁再重建这条 CHECK，都取此前各份的并集。
ALTER TABLE access_identity.operator_grant
    DROP CONSTRAINT operator_grant_decision_kind_decided;

ALTER TABLE access_identity.operator_grant
    ADD CONSTRAINT operator_grant_decision_kind_decided
        CHECK (decision_kind IS NULL OR decision_kind IN (
            'MANUAL_REVIEW_COMPLETION', 'ACTIVE_REJECTION', 'AUTHORIZED_DISPOSITION',
            'CONTROLLED_CLOSURE', 'CONTROLLED_REOPENING',
            'SEGMENT_CLOSURE', 'DISPATCH_TASK_REGISTRATION', 'LOAD_ASSIGNMENT', 'PARTICIPATION_TERMINATION',
            'EFFECTIVE_TIME_JUDGMENT', 'CARRIER_FIRST_EFFECTIVE_PICKUP_JUDGMENT',
            'PENDING_IDENTITY_RECHECK',
            'TRANSPORT_COMMISSION_SUBMISSION', 'TRANSPORT_CHARGE_OCCURRENCE_REGISTRATION',
            'IDENTITY_CONFLICT_DISPOSITION'
        ));
