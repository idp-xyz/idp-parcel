-- 外部资金事实采用与监管凭证登记两种运营决定（ADR-0236 决定一；票 demo-chain-gaps/140）：EXTERNAL_FUNDS_FACT_ADOPTION（首版与更正
-- 同一种）、REGULATORY_CREDENTIAL_REGISTRATION。与 accessidentity.DecisionExternalFundsFactAdoption、
-- DecisionRegulatoryCredentialRegistration 同笔放开（0001 头注那条约定）。本迁移不种任何行：谁被授予哪一种决定是租户取值。
--
-- 只重建决定种类这一条 CHECK；客户面能力、外部事实类型与准入范围对照各条都不动——集成客户端族照旧按 EXTERNAL_FUNDS_FACT、
-- REGULATORY_CREDENTIAL 两类事实授予，两族并存。全集取此前各份重建的并集（以 0019 那一份为底），再加本份两个词。迁移不按编号先后
-- 施加，本份可能晚于编号更大的一份施加，只列本份基线上的全集会把后来的词删掉、让它们的授予登不进；之后谁再重建这条 CHECK，都取此前
-- 各份的并集。

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
            'DELIVERY_DISPATCH_REASK',
            'EXTERNAL_FUNDS_FACT_ADOPTION', 'REGULATORY_CREDENTIAL_REGISTRATION'
        ));
