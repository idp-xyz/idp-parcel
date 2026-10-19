-- 「运营决定」能力面加三种决定：履约段采用运输委托、取消运输委托、取消订舱（ADR-0202 决定一、ADR-0204；票
-- demo-chain-gaps/25）。与 accessidentity.DecisionFulfillmentSegmentCommissionAdoption、DecisionTransportCommissionCancellation、
-- DecisionBookingRequestCancellation 同笔放开（0001 头注那条约定）。本迁移不种任何行：谁被授予哪一种决定是租户取值。
--
-- 三种分开授予：能提交委托的不因此能取消它，能取消委托的也不因此能把它挂到一个段上——授予的粒度等于权力的粒度
-- （ADR-0202 决定一，先例是受控关闭与重开分成两种）。
--
-- 决定种类的全集取此前各份重建的并集（0011 头注那条约定），以 0013 那一份为底再加本份三种：迁移不按编号先后施加，本份
-- 可能晚于编号更大的一份施加，只列本份基线上的全集会把后来的种类删掉、让它们的授予登不进。0014 重建的是客户面能力那条
-- CHECK，不碰决定种类，所以这里不必并它。之后谁再重建这条 CHECK，都取此前各份的并集。

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
            'FULFILLMENT_SEGMENT_COMMISSION_ADOPTION', 'TRANSPORT_COMMISSION_CANCELLATION', 'BOOKING_REQUEST_CANCELLATION'
        ));
