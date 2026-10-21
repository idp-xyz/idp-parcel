-- 派送重问间隔（ADR-0230 决定五；票 demo-chain-gaps/70）：一个租户的派送待形成册上，一件的最近一次答复过去多久才算到期、该再问一次
-- 所有者。租户取值，登记册配置写：本迁移不种任何行，产品不给默认——没登记的租户，到期重问如实答未配置。
--
-- 一版一行，登记方指名版本，同版本只能登一次（同正文是重放、异正文是冲突，由编排分）；最近登记的那一版生效。间隔按整分钟存。
CREATE TABLE transport_fulfillment.delivery_dispatch_reask_cadence (
    tenant_id        text        NOT NULL,
    version          text        NOT NULL,
    interval_minutes integer     NOT NULL,
    recorded_at      timestamptz NOT NULL,

    CONSTRAINT delivery_dispatch_reask_cadence_pkey
        PRIMARY KEY (tenant_id, version),

    CONSTRAINT delivery_dispatch_reask_cadence_scope_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(version) <> ''),

    CONSTRAINT delivery_dispatch_reask_cadence_interval_positive
        CHECK (interval_minutes >= 1)
);

-- 取一个租户此刻生效的那一版：最近登记的在前，同刻按版本稳定取一个。
CREATE INDEX delivery_dispatch_reask_cadence_current
    ON transport_fulfillment.delivery_dispatch_reask_cadence (tenant_id, recorded_at DESC, version DESC);
