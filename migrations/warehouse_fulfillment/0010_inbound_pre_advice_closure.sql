-- 入库预报完结与少收。少收是完结时按整份预报算出来的，不是某一次收货事实的差异。
-- receipt_discrepancy 的外键指着收货事实，少收不进那张表。
--
-- 完结在（租户，预报）上至多一行。少收行只追加，数量是基本单位，只留下大于 0 的。
-- 本表不对预报、商品的修订表建外键。少收行只指向本迁移自己的完结头。
--
-- 本迁移只建结构、不种任何行。仓配开关关着的部署也施加本迁移，表是空的。

CREATE TABLE warehouse_fulfillment.inbound_pre_advice_closure (
    tenant_id     text        NOT NULL,
    pre_advice_id text        NOT NULL,
    operator_id   text        NOT NULL,
    note          text,
    closed_at     timestamptz NOT NULL,

    CONSTRAINT inbound_pre_advice_closure_pkey
        PRIMARY KEY (tenant_id, pre_advice_id),

    CONSTRAINT inbound_pre_advice_closure_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(pre_advice_id) <> ''
            AND btrim(operator_id) <> ''
        ),

    CONSTRAINT inbound_pre_advice_closure_note
        CHECK (note IS NULL OR btrim(note) <> '')
);

CREATE TABLE warehouse_fulfillment.inbound_pre_advice_shortage (
    tenant_id     text   NOT NULL,
    pre_advice_id text   NOT NULL,
    line_index    integer NOT NULL,
    goods_code    text   NOT NULL,
    kind          text   NOT NULL,
    quantity      bigint NOT NULL,

    CONSTRAINT inbound_pre_advice_shortage_pkey
        PRIMARY KEY (tenant_id, pre_advice_id, line_index),

    CONSTRAINT inbound_pre_advice_shortage_closure_fkey
        FOREIGN KEY (tenant_id, pre_advice_id)
        REFERENCES warehouse_fulfillment.inbound_pre_advice_closure (tenant_id, pre_advice_id),

    CONSTRAINT inbound_pre_advice_shortage_goods_key
        UNIQUE (tenant_id, pre_advice_id, goods_code),

    CONSTRAINT inbound_pre_advice_shortage_not_blank
        CHECK (btrim(goods_code) <> '' AND line_index >= 0),

    CONSTRAINT inbound_pre_advice_shortage_quantity
        CHECK (quantity > 0),

    CONSTRAINT inbound_pre_advice_shortage_kind
        CHECK (kind = 'SHORTAGE')
);
