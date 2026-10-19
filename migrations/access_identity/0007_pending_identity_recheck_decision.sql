-- 「运营决定」能力面加一种决定：待识别重核（ADR-0151 决定一；ADR-0199 决定三第二个触发，票 no-reception-source/03）。复核页
-- 上由人指名一件待识别实物、按存下的号再问一次 PS，是管理台上作判断的口。与 accessidentity.DecisionPendingIdentityRecheck 同笔
-- 放开（0001 头注那条约定）。本迁移不种任何行：谁被授予这一种决定是租户取值。

ALTER TABLE access_identity.operator_grant
    DROP CONSTRAINT operator_grant_decision_kind_decided;

ALTER TABLE access_identity.operator_grant
    ADD CONSTRAINT operator_grant_decision_kind_decided
        CHECK (decision_kind IS NULL OR decision_kind IN (
            'MANUAL_REVIEW_COMPLETION', 'ACTIVE_REJECTION', 'AUTHORIZED_DISPOSITION',
            'CONTROLLED_CLOSURE', 'CONTROLLED_REOPENING',
            'SEGMENT_CLOSURE', 'DISPATCH_TASK_REGISTRATION', 'LOAD_ASSIGNMENT', 'PARTICIPATION_TERMINATION',
            'EFFECTIVE_TIME_JUDGMENT', 'CARRIER_FIRST_EFFECTIVE_PICKUP_JUDGMENT',
            'PENDING_IDENTITY_RECHECK'
        ));
