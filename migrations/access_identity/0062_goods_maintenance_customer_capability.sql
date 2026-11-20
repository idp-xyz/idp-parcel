-- 客户面能力加「商品维护」（ADR-0384 决定一；UC-WF-002；票 pn-09-build/04 第 3 段）。
-- 与 accessidentity.CustomerCapabilityGoodsMaintenance 同笔放开。货主口的准入不走 CUSTOMER_COMMAND，
-- 两格都是 GOODS_MAINTENANCE；对照册的 CHECK 不在本份。
--
-- 本迁移不种任何行：哪个客户端被授予是租户取值（参数登记册 PAR-INT-13）。
-- 全集取此前各份重建（含 0061）的并集再并上本词：迁移不按编号先后施加，只列本份基线上的全集会删掉后来的词。

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
            'GOODS_MAINTENANCE',
            'INBOUND_PRE_ADVICE_SUBMISSION', 'INBOUND_PRE_ADVICE_ARRIVAL_NOTICE',
            'INBOUND_PRE_ADVICE_WITHDRAWAL', 'WAREHOUSE_READ',
            'RECEIPT_DISCREPANCY_CONCLUSION'
        ));
