-- 客户面能力加一个词：客户追踪视图查阅（UC-VE-008；ADR-0139 决定四；票 demo-chain-gaps/96）。照首方客户族「一口一词」（票
-- first-party-api/00 完成记录「代决」一节），与 accessidentity.CustomerCapabilityTrackingViewQuery 同笔放开（0010 里这条 CHECK 的注释
-- 「两处同笔改」）。本迁移不种任何行：哪个客户端被授予这一格是租户取值（参数登记册 PAR-INT-13）。查阅口不判准入范围（ADR-0139
-- 决定八），准入范围对照那张表不动。
--
-- 只重建客户面能力这一条 CHECK；决定种类、外部事实类型与准入范围对照各条都不动。全集以 0016 那一份为底、再加本份一个词。迁移不按
-- 编号先后施加，本份可能晚于编号更大的一份施加，只列本份基线上的全集会把后来的词删掉、让它们的授予登不进。之后谁再重建这条
-- CHECK，都取此前各份的并集。

ALTER TABLE access_identity.customer_client_grant
    DROP CONSTRAINT customer_client_grant_capability_decided;

ALTER TABLE access_identity.customer_client_grant
    ADD CONSTRAINT customer_client_grant_capability_decided
        CHECK (capability IN (
            'SHIPMENT_REQUEST_SUBMISSION',
            'DELIVERY_PRE_ADVICE_SUBMISSION', 'DELIVERY_PRE_ADVICE_WITHDRAWAL',
            'DELIVERY_HANDOVER_CONFIRMATION',
            'CUSTOMER_TRACKING_VIEW_QUERY'
        ));
