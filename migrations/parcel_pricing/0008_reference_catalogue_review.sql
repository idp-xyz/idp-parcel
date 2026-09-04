-- 计价参考目录版本复核记录（ADR-0109 Decision 二「复核与在用的门照 ADR-0099 给序列立的那一套」）。
--
-- 复核是追加在版本之旁的独立事实：谁、何时、什么结论、凭什么。它不改 reference_catalogue_version
-- 一列——那张表的行只增不改，「在用」不是它上面的状态列，而是从本表按评价基准时点派生的结论
-- （该时刻之前复核通过的最新版本）。
--
-- 同一版本允许多条复核（退回之后再通过），键是（版本三列 + 复核时刻 + 复核责任方）。外键指向
-- reference_catalogue_version：复核一个不在册的版本在库层就不成立。
--
-- **不加**「复核责任方 ≠ 登记责任方」的库层约束，理由与 reference_series_review 同：那条四眼门是跨表
-- 比对，领域构造门（domain.NewCatalogueReview 以登记为入参）已关。本表只守形状。

CREATE TABLE parcel_pricing.reference_catalogue_review (
    tenant_id           text        NOT NULL,
    catalogue_id        text        NOT NULL,
    catalogue_version   text        NOT NULL,

    reviewer            text        NOT NULL,
    reviewed_at         timestamptz NOT NULL,
    decision            text        NOT NULL,
    basis               text        NOT NULL,

    recorded_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT reference_catalogue_review_pkey
        PRIMARY KEY (tenant_id, catalogue_id, catalogue_version, reviewed_at, reviewer),

    CONSTRAINT reference_catalogue_review_version_fkey
        FOREIGN KEY (tenant_id, catalogue_id, catalogue_version)
        REFERENCES parcel_pricing.reference_catalogue_version (tenant_id, catalogue_id, catalogue_version),

    CONSTRAINT reference_catalogue_review_decision_closed
        CHECK (decision IN ('APPROVED', 'RETURNED')),

    CONSTRAINT reference_catalogue_review_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(catalogue_id) <> ''
            AND btrim(catalogue_version) <> ''
            AND btrim(reviewer) <> ''
            AND btrim(basis) <> ''
        )
);

-- 在用解析按（租户、目录）取全部版本与复核，再在领域里选；这个索引服务那一问。
CREATE INDEX reference_catalogue_review_by_catalogue
    ON parcel_pricing.reference_catalogue_review (tenant_id, catalogue_id, reviewed_at);
