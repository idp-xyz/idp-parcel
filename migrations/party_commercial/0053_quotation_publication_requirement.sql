-- 有发出记录的价卡草稿，发布要不要以一条有效接受为前提。
--
-- 一户一行。没登记不设这道门，读口答未配置。不回填，不插默认行。

CREATE TABLE party_commercial.quotation_publication_requirement (
    tenant_id  text    NOT NULL,
    requires   boolean NOT NULL,

    CONSTRAINT quotation_publication_requirement_pkey
        PRIMARY KEY (tenant_id),

    CONSTRAINT quotation_publication_requirement_tenant_not_blank
        CHECK (btrim(tenant_id) <> '' AND tenant_id = btrim(tenant_id))
);
