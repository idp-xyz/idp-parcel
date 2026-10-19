-- 「运营决定」能力面加一种决定：运输委托提交（ADR-0151 决定一；ADR-0202 决定一，票 demo-chain-gaps/23）。运营人员向外部
-- 服务提供方提出明确的运输范围与条件，是管理台上作决定的口。与 accessidentity.DecisionTransportCommissionSubmission 同笔放开
-- （0001 头注那条约定）。本迁移不种任何行：谁被授予这一种决定是租户取值。
--
-- 约束整条重建，列出截至本迁移为止的全集：CHECK 不能只追加一个值，漏抄前面任何一种，已登的那一类授予就再也写不进来。

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
            'TRANSPORT_COMMISSION_SUBMISSION'
        ));
