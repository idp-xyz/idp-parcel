-- 派送重问（ADR-0230 决定四、七；票 demo-chain-gaps/70）：「运营决定」能力面加一种决定 DELIVERY_DISPATCH_REASK——按派送待形成册批量
-- 重问与对单个（段，对象）补拍共用这一种授予。与 accessidentity.DecisionDeliveryDispatchReask 同笔放开（0001 头注那条约定）。本迁移
-- 不种任何行：谁被授予这一种决定是租户取值。
--
-- 只重建决定种类这一条 CHECK。全集取此前各份重建的并集（以 0018 那一份为底），再加本份一个词。迁移不按编号先后施加，本份可能晚于
-- 编号更大的一份施加，只列本份基线上的全集会把后来的词删掉、让它们的授予登不进；之后谁再重建这条 CHECK，都取此前各份的并集。

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
            'IDENTITY_CONFLICT_DISPOSITION',
            'BOOKING_REQUEST_SUBMISSION',
            'CAPACITY_RESERVATION', 'CAPACITY_RELEASE',
            'FULFILLMENT_SEGMENT_COMMISSION_ADOPTION', 'TRANSPORT_COMMISSION_CANCELLATION', 'BOOKING_REQUEST_CANCELLATION',
            'DELIVERY_HANDOVER_CONFIRMATION',
            'CUSTOMS_CASE_ESTABLISHMENT', 'CUSTOMS_SUBMISSION_AUTHORIZATION', 'CUSTOMS_DECLARATION_SUBMISSION',
            'DELIVERY_DISPATCH_REASK'
        ));
