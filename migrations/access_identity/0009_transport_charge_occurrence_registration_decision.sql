-- 「运营决定」能力面加一种决定：运输收费发生项登记（票 demo-chain-gaps/27，ADR-0207 决定六）。与
-- accessidentity.DecisionTransportChargeOccurrenceRegistration 同笔放开（0001 头注那条约定）。
--
-- 重建 0002 立的决定种类 CHECK，列**截至本迁移号为止的全集**：0007 加的待重核身份、0008 加的运输委托提交都在内。
-- 只追加本笔那一种而漏列前两者，施加本迁移的那一刻就把已授的那两种授予判成违约；同一条约束在几份迁移里各重建
-- 一次，每一份都必须是完整清单，不是增量。本迁移不种任何行。

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
            'TRANSPORT_COMMISSION_SUBMISSION',
            'TRANSPORT_CHARGE_OCCURRENCE_REGISTRATION'
        ));
