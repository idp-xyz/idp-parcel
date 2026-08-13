-- 接受前财务控制（PN-02）：冻结账本与信用暴露账本，两本账互不借用（ADR-0047）。
--
-- 一行一条控制：键=租户+结算作用域+控制请求标识（幂等按它认领）；内部编号在作用域内
-- 唯一（账本签发的 FRZ-/EXP- 序号，重建后续编号靠它恢复）；状态与金额平铺成列，指纹
-- 列保全幂等判定的依据。`业务限制`从不入账本，所以状态只有两个取值。
-- 只增不删：释放是同键状态推进（HELD→RELEASED / RECORDED→RELEASED），原金额与原
-- 发生时间不动。

CREATE TABLE settlement_accounting.funds_freeze (
    tenant_id          text        NOT NULL,
    legal_entity       text        NOT NULL,
    account_id         text        NOT NULL,
    currency           text        NOT NULL,
    control_request_id text        NOT NULL,

    freeze_id          text        NOT NULL,
    status             smallint    NOT NULL,
    amount_minor       bigint      NOT NULL,
    association        text        NOT NULL,
    request_digest     text        NOT NULL,
    frozen_at          timestamptz NOT NULL,
    released_at        timestamptz,
    saved_at           timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT funds_freeze_pkey
        PRIMARY KEY (tenant_id, legal_entity, account_id, currency, control_request_id),
    CONSTRAINT funds_freeze_id_unique
        UNIQUE (tenant_id, legal_entity, account_id, currency, freeze_id),

    CONSTRAINT funds_freeze_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(legal_entity) <> ''
            AND btrim(account_id) <> ''
            AND btrim(currency) <> ''
            AND btrim(control_request_id) <> ''
            AND btrim(freeze_id) <> ''
            AND btrim(association) <> ''
            AND btrim(request_digest) <> ''
        ),
    -- 零金额控制从不入账本（CONTEXT 禁用它冒充「明确无控制」）。
    CONSTRAINT funds_freeze_amount_positive
        CHECK (amount_minor > 0),
    -- 状态两态：1=HELD 2=RELEASED；`业务限制`(3) 没有行。释放必带时间且不早于冻结。
    CONSTRAINT funds_freeze_status_coherent
        CHECK (
            (status = 1 AND released_at IS NULL)
            OR (status = 2 AND released_at IS NOT NULL AND released_at >= frozen_at)
        )
);

CREATE TABLE settlement_accounting.credit_exposure (
    tenant_id          text        NOT NULL,
    legal_entity       text        NOT NULL,
    account_id         text        NOT NULL,
    currency           text        NOT NULL,
    control_request_id text        NOT NULL,

    exposure_id        text        NOT NULL,
    status             smallint    NOT NULL,
    amount_minor       bigint      NOT NULL,
    association        text        NOT NULL,
    request_digest     text        NOT NULL,
    exposed_at         timestamptz NOT NULL,
    released_at        timestamptz,
    saved_at           timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT credit_exposure_pkey
        PRIMARY KEY (tenant_id, legal_entity, account_id, currency, control_request_id),
    CONSTRAINT credit_exposure_id_unique
        UNIQUE (tenant_id, legal_entity, account_id, currency, exposure_id),

    CONSTRAINT credit_exposure_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(legal_entity) <> ''
            AND btrim(account_id) <> ''
            AND btrim(currency) <> ''
            AND btrim(control_request_id) <> ''
            AND btrim(exposure_id) <> ''
            AND btrim(association) <> ''
            AND btrim(request_digest) <> ''
        ),
    CONSTRAINT credit_exposure_amount_positive
        CHECK (amount_minor > 0),
    -- 1=RECORDED 2=RELEASED；`业务限制`(3) 没有行。
    CONSTRAINT credit_exposure_status_coherent
        CHECK (
            (status = 1 AND released_at IS NULL)
            OR (status = 2 AND released_at IS NOT NULL AND released_at >= exposed_at)
        )
);
