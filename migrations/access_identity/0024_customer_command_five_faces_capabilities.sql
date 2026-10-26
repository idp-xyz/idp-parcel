-- 客户面能力一次加五个词：委托撤回 SHIPMENT_REQUEST_WITHDRAWAL、取消包裹 PARCEL_CANCELLATION、客户资料修订 SOURCE_DATA_AMENDMENT、
-- 委托补充 SHIPMENT_REQUEST_SUPPLEMENT、索赔提交 CLAIM_SUBMISSION（首方客户命令五口；票 first-party-api/06；ADR-0310；ADR-0139
-- 决定二、八）。与 accessidentity.CustomerCapabilityShipmentRequestWithdrawal 等五个常量同笔放开（0010 里这条 CHECK 的注释
-- 「两处同笔改」）。五词一份迁移登完，是因为五口共用同一层铸造底座，分五份只会让中间态里有的词登得进、有的登不进。本迁移不种任何行：
-- 哪个客户端被授予哪一格是租户取值（参数登记册 PAR-INT-13）；五口都是生产写，准入范围对照那张表没有封闭词表的 CHECK（0005 头注），
-- 词表由领域构造门守，所以对照表不动。
--
-- 只重建客户面能力这一条 CHECK。全集以 0017 那一份为底、再加本份五个词。迁移不按编号先后施加，本份可能晚于编号更大的一份施加，
-- 只列本份基线上的全集会把后来的词删掉、让它们的授予登不进。之后谁再重建这条 CHECK，都取此前各份的并集。

ALTER TABLE access_identity.customer_client_grant
    DROP CONSTRAINT customer_client_grant_capability_decided;

ALTER TABLE access_identity.customer_client_grant
    ADD CONSTRAINT customer_client_grant_capability_decided
        CHECK (capability IN (
            'SHIPMENT_REQUEST_SUBMISSION',
            'DELIVERY_PRE_ADVICE_SUBMISSION', 'DELIVERY_PRE_ADVICE_WITHDRAWAL',
            'DELIVERY_HANDOVER_CONFIRMATION',
            'CUSTOMER_TRACKING_VIEW_QUERY',
            'SHIPMENT_REQUEST_WITHDRAWAL', 'PARCEL_CANCELLATION', 'SOURCE_DATA_AMENDMENT',
            'SHIPMENT_REQUEST_SUPPLEMENT', 'CLAIM_SUBMISSION'
        ));
