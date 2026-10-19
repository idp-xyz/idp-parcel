-- 送站交接确认的两处词（ADR-0212 决定三；票 ps-external-mark-relations/12）：「运营决定」能力面加一种决定 DELIVERY_HANDOVER_CONFIRMATION
-- （持授予的人依授权交付关系确认送站交接单对一个客户账户的绑定），首方客户面能力加一个词 DELIVERY_HANDOVER_CONFIRMATION（客户账户
-- 对声称了自己的已关闭送站交接单确认或否认，照「一口一词」）。与 accessidentity.DecisionDeliveryHandoverConfirmation、
-- CustomerCapabilityDeliveryHandoverConfirmation 同笔放开（0001 与 0010 头注那两条约定）。本迁移不种任何行：谁被授予这一种决定、
-- 哪个客户端被授予这一项能力，都是租户取值（参数登记册 PAR-INT-08、PAR-INT-13）。
--
-- 作业事实登记面的交接单开立、归入、关闭三种准入事实类型不在这里：准入范围对照那张表没有封闭词表的 CHECK（0005 头注），词表由
-- 领域构造门守。
--
-- 两条 CHECK 各自的全集都取此前各份重建的并集，再加本份一个词：决定种类以 0015 那一份为底，客户面能力以 0014 那一份为底。迁移不按
-- 编号先后施加，本份可能晚于编号更大的一份施加，只列本份基线上的全集会把后来的词删掉、让它们的授予登不进。之后谁再重建这两条
-- CHECK 之一，都取此前各份的并集。

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
            'DELIVERY_HANDOVER_CONFIRMATION'
        ));

ALTER TABLE access_identity.customer_client_grant
    DROP CONSTRAINT customer_client_grant_capability_decided;

ALTER TABLE access_identity.customer_client_grant
    ADD CONSTRAINT customer_client_grant_capability_decided
        CHECK (capability IN (
            'SHIPMENT_REQUEST_SUBMISSION',
            'DELIVERY_PRE_ADVICE_SUBMISSION', 'DELIVERY_PRE_ADVICE_WITHDRAWAL',
            'DELIVERY_HANDOVER_CONFIRMATION'
        ));
