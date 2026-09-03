-- 计价参考序列版本复核记录（ADR-0099 决定二、三；票 pricing-reference-series-operations/02）。
--
-- 复核是追加在版本之旁的独立事实：谁、何时、什么结论、凭什么。它不改 reference_series_version
-- 一列——那张表的行只增不改，「在用」不是它上面的状态列，而是从本表按评价形成时刻派生的结论
-- （该时刻之前复核通过的最新版本）。做成状态列会让「谁在何时凭什么通过」在结构上无处落。
--
-- 同一版本允许多条复核（退回之后再通过；通过之后再来一条退回也不撤销——取值错误以更正版本
-- 处理，CONTEXT 生命周期「计价参考序列版本」）。所以键是（版本三列 + 复核时刻 + 复核责任方），
-- 不是版本三列。
--
-- 外键指向 reference_series_version：复核一个不在册的版本在库层就不成立。
--
-- **不加**「复核责任方 ≠ 登记责任方」的库层约束。那条四眼门是跨表比对，领域构造门
-- （domain.NewSeriesReview 以登记为入参）已关；在这里重复会让同一条规则有两处口径，而
-- CHECK 又写不了跨行子查询，触发器则把一条领域规则藏进库里。本表只守形状。
--
-- 复核不改证据等级：evidence_grade 仍在版本行上，本表没有它——等级是取值凭证的属性，复核
-- 确认的是转录。

CREATE TABLE parcel_pricing.reference_series_review (
    tenant_id       text        NOT NULL,
    series_id       text        NOT NULL,
    series_version  text        NOT NULL,

    reviewer        text        NOT NULL,
    reviewed_at     timestamptz NOT NULL,
    decision        text        NOT NULL,
    basis           text        NOT NULL,

    recorded_at     timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT reference_series_review_pkey
        PRIMARY KEY (tenant_id, series_id, series_version, reviewed_at, reviewer),

    CONSTRAINT reference_series_review_version_fkey
        FOREIGN KEY (tenant_id, series_id, series_version)
        REFERENCES parcel_pricing.reference_series_version (tenant_id, series_id, series_version),

    CONSTRAINT reference_series_review_decision_closed
        CHECK (decision IN ('APPROVED', 'RETURNED')),

    CONSTRAINT reference_series_review_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(series_id) <> ''
            AND btrim(series_version) <> ''
            AND btrim(reviewer) <> ''
            AND btrim(basis) <> ''
        )
);

-- 在用解析按（租户、序列）取全部版本与复核，再在领域里选；这个索引服务那一问。
CREATE INDEX reference_series_review_by_series
    ON parcel_pricing.reference_series_review (tenant_id, series_id, reviewed_at);
