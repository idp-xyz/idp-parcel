-- 分拣：合并拣货的波次，货合在一起拿回来之后，一件一件按出库请求放进各自的周转箱。一次投放一行：
-- 这个波次里、这张请求的这种货、几件、放进了哪个箱。同一个作业事实号只记一次。
--
-- 分拣不动库存：货在拣货时已经记进待出库库位，投进箱里并没有换库位。它只回答「这张请求拣出来的货分好了没有」——
-- 一张请求在合并拣货的波次里拣出来的件数，减去它名下投放的件数，就是还没分的；分完才能复核。
-- 数量按基本单位，与拣货任务同口径。

CREATE TABLE warehouse_fulfillment.sort_put (
    tenant_id      text        NOT NULL,
    fact_id        text        NOT NULL,
    wave_id        text        NOT NULL,
    request_id     text        NOT NULL,
    goods_code     text        NOT NULL,
    quantity       bigint      NOT NULL,
    warehouse_code text        NOT NULL,
    tote_code      text        NOT NULL,
    recorded_at    timestamptz NOT NULL,

    CONSTRAINT sort_put_pkey PRIMARY KEY (tenant_id, fact_id),
    CONSTRAINT sort_put_wave_request_fkey
        FOREIGN KEY (tenant_id, wave_id, request_id)
        REFERENCES warehouse_fulfillment.wave_request (tenant_id, wave_id, request_id),
    CONSTRAINT sort_put_tote_fkey
        FOREIGN KEY (tenant_id, warehouse_code, tote_code)
        REFERENCES warehouse_fulfillment.tote (tenant_id, warehouse_code, tote_code),
    CONSTRAINT sort_put_quantity_positive CHECK (quantity > 0),
    CONSTRAINT sort_put_not_blank CHECK (btrim(fact_id) <> '' AND btrim(goods_code) <> '')
);

CREATE INDEX sort_put_request_idx
    ON warehouse_fulfillment.sort_put (tenant_id, request_id, wave_id);
