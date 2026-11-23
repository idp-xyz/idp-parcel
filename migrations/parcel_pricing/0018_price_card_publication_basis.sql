-- 价卡发布可以记下一条依据。依据是调用方交来的不透明引用，计价不解释它。
--
-- 一行至多一条，不改已记下的不同依据。没记下就是这次发布没有依据。不回填。

CREATE TABLE parcel_pricing.price_card_publication_basis (
    tenant_id     text NOT NULL,
    plan_id       text NOT NULL,
    plan_version  text NOT NULL,
    basis         text NOT NULL,

    CONSTRAINT price_card_publication_basis_pkey
        PRIMARY KEY (tenant_id, plan_id, plan_version),

    CONSTRAINT price_card_publication_basis_version_fkey
        FOREIGN KEY (tenant_id, plan_id, plan_version)
        REFERENCES parcel_pricing.price_card_version (tenant_id, plan_id, plan_version),

    CONSTRAINT price_card_publication_basis_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(plan_id) <> ''
            AND btrim(plan_version) <> ''
            AND btrim(basis) <> ''
            AND basis = btrim(basis)
        )
);
