-- 放不出的例外：放波次（由人选了「放行能放的」，或按规则成波）时这张已预占的请求分配不出——货不在可拣的库位，
-- 或少拣的库位还没盘过——它被跳过、留在已预占，这里记下最近的那一次。一张请求一行，再跳过一次就换成新的那一次。
-- 请求之后放进了波次，这一行删掉；请求离开已预占（取消、拆单）之后这一行留着不管，读的时候只认仍是已预占的请求。

CREATE TABLE warehouse_fulfillment.outbound_allocation_exception (
    tenant_id   text        NOT NULL,
    request_id  text        NOT NULL,
    -- WAVE 是由人放的那个波次，RULE 是按规则成波的那一次执行；reference 是波次号或执行号。
    source      text        NOT NULL,
    reference   text        NOT NULL,
    recorded_at timestamptz NOT NULL,

    CONSTRAINT outbound_allocation_exception_pkey PRIMARY KEY (tenant_id, request_id),
    CONSTRAINT outbound_allocation_exception_request_fkey
        FOREIGN KEY (tenant_id, request_id)
        REFERENCES warehouse_fulfillment.outbound_request (tenant_id, request_id),
    CONSTRAINT outbound_allocation_exception_source CHECK (source IN ('WAVE', 'RULE')),
    CONSTRAINT outbound_allocation_exception_not_blank CHECK (btrim(reference) <> '')
);
