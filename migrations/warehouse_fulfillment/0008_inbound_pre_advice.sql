-- 入库预报（UC-WF-003 的受理、告知与撤回；仓配履约 CONTEXT「入库预报」）。
-- 身份是（租户，预报标识）。货主、仓、到仓方式、入库方式与明细在受理时固定；告知另成版本链，从 1 起只追加。
-- 预报不是库存：本迁移不建、不改库存账。
--
-- 「仓在不在、是不是自营、商品有没有登记、计量单位是不是包装层级、跟踪属性允不允许带批号效期」由应用层在受理时判。
-- 库上守闭集、判重位置、数量为正，以及可空文本是 NULL 或非空白。
-- 判重位置分三处，不挤在同一个唯一约束里：提交在预报头一行，每版告知一行，撤回一行。
-- 同一个键可以先提交、再更新告知，那是两笔。撤回的键在租户内唯一，已落册的键不能再撤另一份。
-- 单据号非空时（租户，单据号）唯一。
--
-- 本表不对仓修订表、商品修订表建外键：受理之后仓被停用、商品追加修订，都不回改已受理的预报。
-- 本迁移只建结构、不种任何行。仓配开关关着的部署也施加本迁移，表是空的。

CREATE TABLE warehouse_fulfillment.inbound_pre_advice (
    tenant_id            text        NOT NULL,
    pre_advice_id        text        NOT NULL,
    owner_account_id     text        NOT NULL,
    warehouse_code       text        NOT NULL,
    arrival_mode         text        NOT NULL,
    inbound_mode         text        NOT NULL,
    status               text        NOT NULL,
    document_number      text,
    numbering_absence_reason text,

    submission_path      text        NOT NULL,
    submission_source    text        NOT NULL,
    request_key          text        NOT NULL,
    evidence_reference   text,

    registrant           text        NOT NULL,
    accepted_at          timestamptz NOT NULL,

    CONSTRAINT inbound_pre_advice_pkey
        PRIMARY KEY (tenant_id, pre_advice_id),

    CONSTRAINT inbound_pre_advice_submission_key
        UNIQUE (tenant_id, submission_path, submission_source, request_key),

    CONSTRAINT inbound_pre_advice_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(pre_advice_id) <> ''
            AND btrim(owner_account_id) <> ''
            AND btrim(warehouse_code) <> ''
            AND btrim(request_key) <> ''
            AND btrim(registrant) <> ''
        ),

    CONSTRAINT inbound_pre_advice_arrival_mode
        CHECK (arrival_mode IN ('FULL_CONTAINER', 'PALLET', 'LOOSE_CARTON')),

    CONSTRAINT inbound_pre_advice_inbound_mode
        CHECK (inbound_mode IN ('BY_GOODS', 'BY_CARTON')),

    CONSTRAINT inbound_pre_advice_status
        CHECK (status IN ('ADVISED', 'RECEIVING', 'CLOSED', 'CANCELLED')),

    CONSTRAINT inbound_pre_advice_submission_shape
        CHECK (
            (
                submission_path = 'OWNER'
                AND btrim(submission_source) <> ''
                AND evidence_reference IS NULL
            )
            OR (
                submission_path = 'ON_BEHALF'
                AND submission_source = ''
                AND evidence_reference IS NOT NULL
                AND btrim(evidence_reference) <> ''
            )
        ),

    CONSTRAINT inbound_pre_advice_number_shape
        CHECK (
            (
                document_number IS NOT NULL
                AND btrim(document_number) <> ''
                AND numbering_absence_reason IS NULL
            )
            OR (
                document_number IS NULL
                AND numbering_absence_reason = 'NUMBERING_TIME_ZONE_NOT_CONFIGURED'
            )
        ),

    CONSTRAINT inbound_pre_advice_evidence_text
        CHECK (evidence_reference IS NULL OR btrim(evidence_reference) <> '')
);

CREATE UNIQUE INDEX inbound_pre_advice_document_number_key
    ON warehouse_fulfillment.inbound_pre_advice (tenant_id, document_number)
    WHERE document_number IS NOT NULL;

CREATE TABLE warehouse_fulfillment.inbound_pre_advice_goods_line (
    tenant_id     text    NOT NULL,
    pre_advice_id text    NOT NULL,
    line_index    integer NOT NULL,
    goods_code    text    NOT NULL,
    unit_name     text    NOT NULL,
    quantity      bigint  NOT NULL,
    batch_number  text,
    expiry_date   date,
    carton_count  bigint,

    CONSTRAINT inbound_pre_advice_goods_line_pkey
        PRIMARY KEY (tenant_id, pre_advice_id, line_index),

    CONSTRAINT inbound_pre_advice_goods_line_pre_advice_fkey
        FOREIGN KEY (tenant_id, pre_advice_id)
        REFERENCES warehouse_fulfillment.inbound_pre_advice (tenant_id, pre_advice_id),

    CONSTRAINT inbound_pre_advice_goods_line_not_blank
        CHECK (
            btrim(goods_code) <> ''
            AND btrim(unit_name) <> ''
            AND line_index >= 0
        ),

    CONSTRAINT inbound_pre_advice_goods_line_quantity
        CHECK (quantity > 0),

    CONSTRAINT inbound_pre_advice_goods_line_carton_count
        CHECK (carton_count IS NULL OR carton_count > 0),

    CONSTRAINT inbound_pre_advice_goods_line_batch
        CHECK (batch_number IS NULL OR btrim(batch_number) <> '')
);

CREATE TABLE warehouse_fulfillment.inbound_pre_advice_carton (
    tenant_id     text    NOT NULL,
    pre_advice_id text    NOT NULL,
    carton_index  integer NOT NULL,
    carton_number text    NOT NULL,

    CONSTRAINT inbound_pre_advice_carton_pkey
        PRIMARY KEY (tenant_id, pre_advice_id, carton_index),

    CONSTRAINT inbound_pre_advice_carton_pre_advice_fkey
        FOREIGN KEY (tenant_id, pre_advice_id)
        REFERENCES warehouse_fulfillment.inbound_pre_advice (tenant_id, pre_advice_id),

    CONSTRAINT inbound_pre_advice_carton_number_key
        UNIQUE (tenant_id, pre_advice_id, carton_number),

    CONSTRAINT inbound_pre_advice_carton_not_blank
        CHECK (btrim(carton_number) <> '' AND carton_index >= 0)
);

CREATE TABLE warehouse_fulfillment.inbound_pre_advice_carton_line (
    tenant_id     text    NOT NULL,
    pre_advice_id text    NOT NULL,
    carton_index  integer NOT NULL,
    line_index    integer NOT NULL,
    goods_code    text    NOT NULL,
    quantity      bigint  NOT NULL,

    CONSTRAINT inbound_pre_advice_carton_line_pkey
        PRIMARY KEY (tenant_id, pre_advice_id, carton_index, line_index),

    CONSTRAINT inbound_pre_advice_carton_line_carton_fkey
        FOREIGN KEY (tenant_id, pre_advice_id, carton_index)
        REFERENCES warehouse_fulfillment.inbound_pre_advice_carton (tenant_id, pre_advice_id, carton_index),

    CONSTRAINT inbound_pre_advice_carton_line_not_blank
        CHECK (btrim(goods_code) <> '' AND line_index >= 0),

    CONSTRAINT inbound_pre_advice_carton_line_quantity
        CHECK (quantity > 0)
);

CREATE TABLE warehouse_fulfillment.inbound_pre_advice_notice (
    tenant_id       text        NOT NULL,
    pre_advice_id   text        NOT NULL,
    version         integer     NOT NULL,
    expected_at     timestamptz NOT NULL,
    arrival_clue    text,
    owner_reference text,

    submission_path    text        NOT NULL,
    submission_source  text        NOT NULL,
    request_key        text        NOT NULL,
    evidence_reference text,

    registrant  text        NOT NULL,
    recorded_at timestamptz NOT NULL,

    CONSTRAINT inbound_pre_advice_notice_pkey
        PRIMARY KEY (tenant_id, pre_advice_id, version),

    CONSTRAINT inbound_pre_advice_notice_submission_key
        UNIQUE (tenant_id, submission_path, submission_source, request_key),

    CONSTRAINT inbound_pre_advice_notice_pre_advice_fkey
        FOREIGN KEY (tenant_id, pre_advice_id)
        REFERENCES warehouse_fulfillment.inbound_pre_advice (tenant_id, pre_advice_id),

    CONSTRAINT inbound_pre_advice_notice_version
        CHECK (version >= 1),

    CONSTRAINT inbound_pre_advice_notice_not_blank
        CHECK (btrim(request_key) <> '' AND btrim(registrant) <> ''),

    CONSTRAINT inbound_pre_advice_notice_submission_shape
        CHECK (
            (
                submission_path = 'OWNER'
                AND btrim(submission_source) <> ''
                AND evidence_reference IS NULL
            )
            OR (
                submission_path = 'ON_BEHALF'
                AND submission_source = ''
                AND evidence_reference IS NOT NULL
                AND btrim(evidence_reference) <> ''
            )
        ),

    CONSTRAINT inbound_pre_advice_notice_clue
        CHECK (arrival_clue IS NULL OR btrim(arrival_clue) <> ''),

    CONSTRAINT inbound_pre_advice_notice_reference
        CHECK (owner_reference IS NULL OR btrim(owner_reference) <> ''),

    CONSTRAINT inbound_pre_advice_notice_evidence
        CHECK (evidence_reference IS NULL OR btrim(evidence_reference) <> '')
);

CREATE TABLE warehouse_fulfillment.inbound_pre_advice_withdrawal (
    tenant_id     text        NOT NULL,
    pre_advice_id text        NOT NULL,

    submission_path    text        NOT NULL,
    submission_source  text        NOT NULL,
    request_key        text        NOT NULL,
    evidence_reference text,

    registrant    text        NOT NULL,
    withdrawn_at  timestamptz NOT NULL,

    CONSTRAINT inbound_pre_advice_withdrawal_pkey
        PRIMARY KEY (tenant_id, pre_advice_id),

    CONSTRAINT inbound_pre_advice_withdrawal_submission_key
        UNIQUE (tenant_id, submission_path, submission_source, request_key),

    CONSTRAINT inbound_pre_advice_withdrawal_pre_advice_fkey
        FOREIGN KEY (tenant_id, pre_advice_id)
        REFERENCES warehouse_fulfillment.inbound_pre_advice (tenant_id, pre_advice_id),

    CONSTRAINT inbound_pre_advice_withdrawal_not_blank
        CHECK (btrim(request_key) <> '' AND btrim(registrant) <> ''),

    CONSTRAINT inbound_pre_advice_withdrawal_submission_shape
        CHECK (
            (
                submission_path = 'OWNER'
                AND btrim(submission_source) <> ''
                AND evidence_reference IS NULL
            )
            OR (
                submission_path = 'ON_BEHALF'
                AND submission_source = ''
                AND evidence_reference IS NOT NULL
                AND btrim(evidence_reference) <> ''
            )
        ),

    CONSTRAINT inbound_pre_advice_withdrawal_evidence
        CHECK (evidence_reference IS NULL OR btrim(evidence_reference) <> '')
);
