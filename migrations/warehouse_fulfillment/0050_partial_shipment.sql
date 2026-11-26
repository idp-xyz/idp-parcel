-- 部分发货：一张出库请求拣不齐时货主愿意怎么发。整单发、少发、拆单发三种，都是货主的意思；
-- 没写明的留空，不填默认。三层各记各的，取哪一层由编排定：
--   货主的默认——登记在缺货处置里，与「部分预占 / 不足拒单」是两件事，各说各的，所以原来必填的 kind 放成可空，
--   一份修订至少说其中一件；
--   请求提交时带的——记在请求上；
--   受理之后货主对这一张请求另外表示的——由运营凭依据代录，一句一行，不改请求提交时的内容。

ALTER TABLE warehouse_fulfillment.shortage_policy
    DROP CONSTRAINT shortage_policy_kind;

ALTER TABLE warehouse_fulfillment.shortage_policy
    ALTER COLUMN kind DROP NOT NULL,
    ADD COLUMN partial_shipment text,
    ADD CONSTRAINT shortage_policy_kind CHECK (kind IS NULL OR kind IN ('PARTIAL', 'REJECT')),
    ADD CONSTRAINT shortage_policy_partial_shipment CHECK (
        partial_shipment IS NULL OR partial_shipment IN ('SHIP_COMPLETE', 'SHORT_SHIP', 'SPLIT_SHIP')
    ),
    ADD CONSTRAINT shortage_policy_says_something CHECK (kind IS NOT NULL OR partial_shipment IS NOT NULL);

ALTER TABLE warehouse_fulfillment.outbound_request
    ADD COLUMN partial_shipment text,
    ADD CONSTRAINT outbound_request_partial_shipment CHECK (
        partial_shipment IS NULL OR partial_shipment IN ('SHIP_COMPLETE', 'SHORT_SHIP', 'SPLIT_SHIP')
    );

CREATE TABLE warehouse_fulfillment.outbound_partial_shipment_consent (
    tenant_id        text        NOT NULL,
    request_id       text        NOT NULL,
    basis            text        NOT NULL,
    partial_shipment text        NOT NULL,
    operator_id      text        NOT NULL,
    recorded_at      timestamptz NOT NULL,

    CONSTRAINT outbound_partial_shipment_consent_pkey PRIMARY KEY (tenant_id, request_id, basis),
    CONSTRAINT outbound_partial_shipment_consent_request_fkey
        FOREIGN KEY (tenant_id, request_id)
        REFERENCES warehouse_fulfillment.outbound_request (tenant_id, request_id),
    CONSTRAINT outbound_partial_shipment_consent_rule CHECK (
        partial_shipment IN ('SHIP_COMPLETE', 'SHORT_SHIP', 'SPLIT_SHIP')
    ),
    CONSTRAINT outbound_partial_shipment_consent_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(request_id) <> '' AND btrim(basis) <> '' AND btrim(operator_id) <> ''
    )
);
