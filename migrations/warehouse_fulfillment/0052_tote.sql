-- 周转箱：拣货时装一张出库请求的货的箱子，拣货车上的格口也按一个箱登记。它是登记过的主数据：
-- 扫到没登记的码不认，才不会把货绑到错的单上。按仓登记，箱号在一个仓里唯一。
-- 停用的箱留着行不删：它过去绑过哪些单要查得到。
--
-- tote_binding 是箱与出库请求的绑定，一次绑定一行。同一时刻一个箱只属于一张请求——还没放开的绑定
-- 每个箱至多一行，由唯一索引守着；放开之后这个箱可以再绑别的请求，旧的那一行留着。

CREATE TABLE warehouse_fulfillment.tote (
    tenant_id      text        NOT NULL,
    warehouse_code text        NOT NULL,
    tote_code      text        NOT NULL,
    registered_by  text        NOT NULL,
    registered_at  timestamptz NOT NULL,
    retired_by     text,
    retired_at     timestamptz,

    CONSTRAINT tote_pkey PRIMARY KEY (tenant_id, warehouse_code, tote_code),
    CONSTRAINT tote_retirement CHECK ((retired_at IS NULL) = (retired_by IS NULL)),
    CONSTRAINT tote_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(warehouse_code) <> '' AND btrim(tote_code) <> '' AND btrim(registered_by) <> ''
    )
);

CREATE TABLE warehouse_fulfillment.tote_binding (
    tenant_id      text        NOT NULL,
    warehouse_code text        NOT NULL,
    tote_code      text        NOT NULL,
    request_id     text        NOT NULL,
    bound_at       timestamptz NOT NULL,
    released_at    timestamptz,

    CONSTRAINT tote_binding_pkey PRIMARY KEY (tenant_id, warehouse_code, tote_code, bound_at),
    CONSTRAINT tote_binding_tote_fkey
        FOREIGN KEY (tenant_id, warehouse_code, tote_code)
        REFERENCES warehouse_fulfillment.tote (tenant_id, warehouse_code, tote_code),
    CONSTRAINT tote_binding_request_fkey
        FOREIGN KEY (tenant_id, request_id)
        REFERENCES warehouse_fulfillment.outbound_request (tenant_id, request_id),
    CONSTRAINT tote_binding_released_after_bound CHECK (released_at IS NULL OR released_at >= bound_at)
);

CREATE UNIQUE INDEX tote_binding_live_idx
    ON warehouse_fulfillment.tote_binding (tenant_id, warehouse_code, tote_code)
    WHERE released_at IS NULL;

CREATE INDEX tote_binding_request_idx
    ON warehouse_fulfillment.tote_binding (tenant_id, request_id);
