-- 价卡换版时收尾旧版的两张册（ADR-0234，票 demo-chain-gaps/150）。
--
-- 价卡版本行只增不改（0002 头注：发布后的定价方案不可原地覆盖），按范围选卡的口在同一计价范围、方向与目的上见到两版
-- 同时适用就答`适用冲突`。旧版要离开新的选择，只能在册上追加收尾事实、装载时施加到选择上；版本行与它的快照一字不动，
-- 既有评价与重放照旧按版本引用读回原来那一版。
--
-- 一、截止册。一行记一次「这一版的选用截到哪一刻、让位给同一方案的哪一版、凭什么」。截止点取后继的起点，由服务端判出
-- 写下，不由登记方给。只增：同一版可以先后被截多次（每一次都要落在当时的选用之内），选用按登记顺序取最后一条——
-- registration_id 是内部序轴（形照 ADR-0056 决定二），写入前先锁被截那一版的版本行，成功接纳的顺序于是与串行化顺序
-- 一致。同一后继、同一依据的那一次只有一行：重放撞唯一键，适配器读回答`已登记`。
--
-- 二、退役册。一行记一次「这一版让位给同一方案的哪一版、凭什么、何时起不再参与任何新的选择」。一版只退一次，主键即版本
-- 三元组。退役时刻取登记时刻。
--
-- 两张册的后继都以外键指回版本册，且与被收尾的那一版同租户、同方案：跨方案的「后继」在这两张表里存不下。

CREATE TABLE parcel_pricing.price_card_version_cut (
    registration_id   bigserial   NOT NULL,

    tenant_id         text        NOT NULL,
    plan_id           text        NOT NULL,
    plan_version      text        NOT NULL,

    successor_version text        NOT NULL,
    selection_ends_at timestamptz NOT NULL,
    closure_reference text        NOT NULL,
    registered_at     timestamptz NOT NULL,

    CONSTRAINT price_card_version_cut_pkey
        PRIMARY KEY (registration_id),

    CONSTRAINT price_card_version_cut_once_per_decision
        UNIQUE (tenant_id, plan_id, plan_version, successor_version, closure_reference),

    CONSTRAINT price_card_version_cut_version_fkey
        FOREIGN KEY (tenant_id, plan_id, plan_version)
        REFERENCES parcel_pricing.price_card_version (tenant_id, plan_id, plan_version),

    CONSTRAINT price_card_version_cut_successor_fkey
        FOREIGN KEY (tenant_id, plan_id, successor_version)
        REFERENCES parcel_pricing.price_card_version (tenant_id, plan_id, plan_version),

    CONSTRAINT price_card_version_cut_successor_is_another_version
        CHECK (successor_version <> plan_version),

    CONSTRAINT price_card_version_cut_not_blank
        CHECK (btrim(successor_version) <> '' AND btrim(closure_reference) <> '')
);

-- 选卡按版本取最新一条截止：索引按版本键与序轴排，取最后一条不扫全册。
CREATE INDEX price_card_version_cut_latest
    ON parcel_pricing.price_card_version_cut (tenant_id, plan_id, plan_version, registration_id);

CREATE TABLE parcel_pricing.price_card_version_retirement (
    tenant_id         text        NOT NULL,
    plan_id           text        NOT NULL,
    plan_version      text        NOT NULL,

    successor_version text        NOT NULL,
    closure_reference text        NOT NULL,
    retired_at        timestamptz NOT NULL,

    CONSTRAINT price_card_version_retirement_pkey
        PRIMARY KEY (tenant_id, plan_id, plan_version),

    CONSTRAINT price_card_version_retirement_version_fkey
        FOREIGN KEY (tenant_id, plan_id, plan_version)
        REFERENCES parcel_pricing.price_card_version (tenant_id, plan_id, plan_version),

    CONSTRAINT price_card_version_retirement_successor_fkey
        FOREIGN KEY (tenant_id, plan_id, successor_version)
        REFERENCES parcel_pricing.price_card_version (tenant_id, plan_id, plan_version),

    CONSTRAINT price_card_version_retirement_successor_is_another_version
        CHECK (successor_version <> plan_version),

    CONSTRAINT price_card_version_retirement_not_blank
        CHECK (btrim(successor_version) <> '' AND btrim(closure_reference) <> '')
);
