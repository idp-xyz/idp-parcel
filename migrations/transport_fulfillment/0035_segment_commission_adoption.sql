-- 履约段采用运输委托（ADR-0204；票 demo-chain-gaps/25）。
--
-- 一行是一次采用声明：这一段在这份委托下、由这一家履约，以及这一段在委托下开始的依据与时刻（段的首个当前参与）。
-- 主键只到段：一段至多采用一份委托，同段第二份撞主键，编排读回按摘要分已有与冲突。同一份委托可以被多段采用，
-- 所以委托一列不唯一，另建索引供按委托回查。
--
-- 只插不改：采用之后不改、不解除（ADR-0204 越权风险点「没有反向解除采用的面」），没有任何列会被 UPDATE。段与委托都
-- 只增不删，两条外键钉「段须在册、委托须已提交」——编排已在落行之前核过，这里是第二道（0001 立的纪律）。
--
-- 服务方与委托上的服务提供方在采用时核过相等；这里另存一份是登记方声明的原样，不是委托快照的副本——服务方、协议与法人
-- 的权威仍是委托那一行，发生项登记按委托取（tf-chain-plan D07-2）。
CREATE TABLE transport_fulfillment.segment_commission_adoption (
    tenant_id      text        NOT NULL,
    segment_ref    text        NOT NULL,

    commission_id  text        NOT NULL,
    provider_ref   text        NOT NULL,
    start_basis    text        NOT NULL,
    started_at     timestamptz NOT NULL,
    content_digest text        NOT NULL,
    recorded_at    timestamptz NOT NULL,
    inserted_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT segment_commission_adoption_pkey
        PRIMARY KEY (tenant_id, segment_ref),

    CONSTRAINT segment_commission_adoption_segment_fkey
        FOREIGN KEY (tenant_id, segment_ref)
        REFERENCES transport_fulfillment.actual_fulfillment_segment (tenant_id, segment_ref),

    CONSTRAINT segment_commission_adoption_commission_fkey
        FOREIGN KEY (tenant_id, commission_id)
        REFERENCES transport_fulfillment.transport_commission (tenant_id, commission_id),

    CONSTRAINT segment_commission_adoption_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(segment_ref) <> ''
            AND btrim(commission_id) <> ''
            AND btrim(provider_ref) <> ''
            AND btrim(start_basis) <> ''
            AND btrim(content_digest) <> ''
        )
);

CREATE INDEX segment_commission_adoption_commission
    ON transport_fulfillment.segment_commission_adoption (tenant_id, commission_id);
