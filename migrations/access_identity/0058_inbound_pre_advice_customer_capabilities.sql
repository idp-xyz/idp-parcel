-- 客户面能力加四个词：入库预报提交、告知更新、撤回，以及仓配查阅（ADR-0384 决定一；UC-WF-003；票 pn-09-build/08）。
-- 照首方客户族「一口一词」。WAREHOUSE_READ 是读口共用的词，本份只让入库预报的读用它。与 accessidentity.CustomerCapability 同笔放开
-- （0010 里这条 CHECK 的注释「两处同笔改」）。本迁移不种任何行：哪个客户端被授予这几格是租户取值（参数登记册 PAR-INT-13）。
-- 三个写口的准入词不走 CUSTOMER_COMMAND，对照册的 CHECK 不在本份。查阅不判准入。
--
-- 只重建客户面能力这一条 CHECK。全集取此前各份（0010、0014、0016、0017、0024、0038）的并集、再加本份四个词：迁移不按编号先后施加，
-- 本份可能晚于编号更大的一份施加，只列本份基线上的全集会把后来的词删掉、让它们的授予登不进。之后谁再重建这条 CHECK，都取此前各份的并集。

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
            'SHIPMENT_REQUEST_SUPPLEMENT', 'CLAIM_SUBMISSION',
            'CUSTOMER_SHIPMENT_TRACKING_QUERY',
            'INBOUND_PRE_ADVICE_SUBMISSION', 'INBOUND_PRE_ADVICE_ARRIVAL_NOTICE',
            'INBOUND_PRE_ADVICE_WITHDRAWAL', 'WAREHOUSE_READ'
        ));
