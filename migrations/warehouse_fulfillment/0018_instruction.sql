-- 加工与商品换标指令，以及一次执行（UC-WF-008）。
-- 组成在受理时固定。执行整笔入账；账上不够的不落执行事实。
-- 本迁移只建结构、不种任何行。

CREATE TABLE warehouse_fulfillment.instruction (
    tenant_id          text        NOT NULL,
    instruction_id     text        NOT NULL,
    owner_account_id   text        NOT NULL,
    warehouse_code     text        NOT NULL,
    kind               text        NOT NULL,
    status             text        NOT NULL,
    origin             text        NOT NULL,
    goods_code         text        NOT NULL,
    unit_name          text,
    quantity           bigint      NOT NULL,
    from_alias         text,
    to_alias           text,
    batch_number       text,
    expiry_date        date,
    reference_text     text,
    request_key        text,
    decision_id        text,
    operator_id        text,
    reason             text,
    basis_reference    text,
    named_revision     integer     NOT NULL,
    fixed_revision     integer     NOT NULL,
    done_quantity      bigint      NOT NULL,
    execution_fact_id  text,
    void_origin        text,
    void_reason        text,
    void_operator_id   text,
    withdrawal_key     text,
    voided_at          timestamptz,
    received_at        timestamptz NOT NULL,

    CONSTRAINT instruction_pkey PRIMARY KEY (tenant_id, instruction_id),
    CONSTRAINT instruction_kind CHECK (kind IN ('PROCESS', 'DISASSEMBLE', 'RELABEL')),
    CONSTRAINT instruction_status CHECK (status IN ('PENDING', 'DONE', 'VOID')),
    CONSTRAINT instruction_origin CHECK (origin IN ('CUSTOMER', 'OPERATOR')),
    CONSTRAINT instruction_quantity CHECK (quantity > 0 AND done_quantity >= 0 AND fixed_revision >= 1 AND named_revision >= 0),
    CONSTRAINT instruction_request_key UNIQUE (tenant_id, request_key),
    CONSTRAINT instruction_decision_key UNIQUE (tenant_id, decision_id),
    CONSTRAINT instruction_withdrawal_key UNIQUE (tenant_id, withdrawal_key),
    CONSTRAINT instruction_not_blank CHECK (
        btrim(tenant_id) <> '' AND btrim(instruction_id) <> '' AND btrim(owner_account_id) <> ''
        AND btrim(warehouse_code) <> '' AND btrim(goods_code) <> ''
        AND (unit_name IS NULL OR btrim(unit_name) <> '')
        AND (from_alias IS NULL OR btrim(from_alias) <> '')
        AND (to_alias IS NULL OR btrim(to_alias) <> '')
        AND (batch_number IS NULL OR btrim(batch_number) <> '')
        AND (request_key IS NULL OR btrim(request_key) <> '')
        AND (decision_id IS NULL OR btrim(decision_id) <> '')
        AND (execution_fact_id IS NULL OR btrim(execution_fact_id) <> '')
        AND (void_origin IS NULL OR void_origin IN ('CUSTOMER', 'OPERATOR'))
    )
);

CREATE TABLE warehouse_fulfillment.instruction_component (
    tenant_id       text    NOT NULL,
    instruction_id  text    NOT NULL,
    position        integer NOT NULL,
    goods_code      text    NOT NULL,
    quantity        bigint  NOT NULL,

    CONSTRAINT instruction_component_pkey PRIMARY KEY (tenant_id, instruction_id, position),
    CONSTRAINT instruction_component_instruction_fkey
        FOREIGN KEY (tenant_id, instruction_id)
        REFERENCES warehouse_fulfillment.instruction (tenant_id, instruction_id),
    CONSTRAINT instruction_component_quantity CHECK (quantity > 0 AND position >= 0 AND btrim(goods_code) <> '')
);

CREATE TABLE warehouse_fulfillment.instruction_execution (
    tenant_id         text        NOT NULL,
    fact_id           text        NOT NULL,
    instruction_id    text        NOT NULL,
    device_id         text        NOT NULL,
    operator_id       text        NOT NULL,
    occurred_at       timestamptz NOT NULL,
    device_sequence   bigint      NOT NULL,
    received_at       timestamptz NOT NULL,
    done_quantity     bigint      NOT NULL,
    movement_id       text        NOT NULL,

    CONSTRAINT instruction_execution_pkey PRIMARY KEY (tenant_id, fact_id),
    CONSTRAINT instruction_execution_instruction_fkey
        FOREIGN KEY (tenant_id, instruction_id)
        REFERENCES warehouse_fulfillment.instruction (tenant_id, instruction_id),
    CONSTRAINT instruction_execution_quantity CHECK (done_quantity > 0 AND device_sequence >= 1)
);

CREATE TABLE warehouse_fulfillment.instruction_execution_line (
    tenant_id        text    NOT NULL,
    fact_id          text    NOT NULL,
    position         integer NOT NULL,
    direction        text    NOT NULL,
    location_code    text    NOT NULL,
    goods_code       text    NOT NULL,
    unit_name        text    NOT NULL,
    stock_state      text    NOT NULL,
    label_alias      text,
    batch_number     text,
    expiry_date      date,
    inbound_batch_id text,
    quantity         bigint  NOT NULL,

    CONSTRAINT instruction_execution_line_pkey PRIMARY KEY (tenant_id, fact_id, position),
    CONSTRAINT instruction_execution_line_fact_fkey
        FOREIGN KEY (tenant_id, fact_id)
        REFERENCES warehouse_fulfillment.instruction_execution (tenant_id, fact_id),
    CONSTRAINT instruction_execution_line_shape CHECK (
        direction IN ('IN', 'OUT') AND quantity > 0 AND position >= 0
        AND (label_alias IS NULL OR btrim(label_alias) <> '')
        AND (batch_number IS NULL OR btrim(batch_number) <> '')
        AND (inbound_batch_id IS NULL OR btrim(inbound_batch_id) <> '')
    )
);
