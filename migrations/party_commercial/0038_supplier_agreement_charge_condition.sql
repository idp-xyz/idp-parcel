-- 供应商协议正文的收费发生条件（ADR-0209；票 demo-chain-gaps/29）：一行记一版协议对一个发生原因的声明——计费并带
-- 数量形，或明确不计费。
--
-- 与正文表分表、外键挂在正文行上：条件是正文的一部分，只能随正文同一次发布登记；子表让「一个原因一行」由主键守住，
-- 不必在一列 JSON 里另判重复。一版协议没有任何一行即整节没声明——运输履约按这一版登记发生项时答`未配置`；有行而
-- 缺某个原因，那个原因同样是没声明。不计费是一行 charged = false，不是缺行：两者要人做的事相反。
--
-- 原因与数量形各是封闭集，CHECK 守集合；哪个原因能配哪个数量形是发布用例的领域门（ADR-0209 决定二），不在库面再写
-- 一遍——那张表随产品策略加宽时，只改领域一处。行属实例半边：演示租户由种子登 `SYN-` 值。
CREATE TABLE party_commercial.supplier_agreement_charge_condition (
    tenant_id      text        NOT NULL,
    object_kind    smallint    NOT NULL,
    object_id      text        NOT NULL,
    version_label  text        NOT NULL,
    reason         text        NOT NULL,
    charged        boolean     NOT NULL,
    quantity_form  text,
    registered_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT supplier_agreement_charge_condition_pkey
        PRIMARY KEY (tenant_id, object_kind, object_id, version_label, reason),

    CONSTRAINT supplier_agreement_charge_condition_agreement_fkey
        FOREIGN KEY (tenant_id, object_kind, object_id, version_label)
        REFERENCES party_commercial.supplier_agreement
            (tenant_id, object_kind, object_id, version_label),

    CONSTRAINT supplier_agreement_charge_condition_reason_closed
        CHECK (reason IN ('BOOKING', 'CANCELLATION', 'FAILED_ATTEMPT', 'ACTUAL_FULFILLMENT')),

    CONSTRAINT supplier_agreement_charge_condition_form_closed
        CHECK (quantity_form IS NULL OR quantity_form IN ('PER_SEGMENT', 'PER_PIECE', 'PER_ATTEMPT', 'PER_BOOKING')),

    -- 计费必带数量形、不计费必不带：计费而没有数量形，运输履约定不出数量与单位；不计费带着它，读的人分不清哪一格算数。
    CONSTRAINT supplier_agreement_charge_condition_form_iff_charged
        CHECK (charged = (quantity_form IS NOT NULL))
);
