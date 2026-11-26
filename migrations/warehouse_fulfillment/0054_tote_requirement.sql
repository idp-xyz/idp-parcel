-- 「这个仓拣货必须用周转箱」是租户按仓定的规则：有的仓每张单都进箱，有的仓整托整箱出、用不着箱。
-- 没登记的仓这一条未配置，拣货带不带箱都收。修订只追加，生效的是最新一版；required 为假是明说不要求。

CREATE TABLE warehouse_fulfillment.tote_requirement (
    tenant_id      text        NOT NULL,
    warehouse_code text        NOT NULL,
    revision       integer     NOT NULL,
    required       boolean     NOT NULL,
    operator_id    text        NOT NULL,
    registered_at  timestamptz NOT NULL,

    CONSTRAINT tote_requirement_pkey PRIMARY KEY (tenant_id, warehouse_code, revision),
    CONSTRAINT tote_requirement_revision CHECK (revision >= 1),
    CONSTRAINT tote_requirement_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(warehouse_code) <> '' AND btrim(operator_id) <> ''
    )
);
