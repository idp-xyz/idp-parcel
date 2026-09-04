-- 评价问题项子表（ADR-0105 Decision 三；票 pricing-reference-series-operations/05 第 2 项）。
--
-- 一次评价带的是一组问题项不是一个，所以不给评价表加列、不落数组：一行一条问题项，外键回
-- parcel_pricing.evaluation。与父表同一事务写入、同父表一样追加不改。
--
-- 它是**检索列面**，不是权威内容：评价的权威内容仍在 evaluation.snapshot，读回评价对象只经快照装载与
-- 整图重验，不从本表重建问题项。本表只为「按待判断且原因码属于哪两格、按序列种类分组计数」那一问而立
-- （ADR-0105 Decision 五），读面只读本表与父表的列，不扫 JSONB。
--
-- 序列种类可空——只有 REFERENCE_SERIES_UNRESOLVED 与 EXCHANGE_RATE_UNRESOLVED 两种问题项带「涉及序列」主体
-- （Decision 一、六）；非空时钉在 domain.ReferenceSeriesKind 的封闭集内，与 0003 / 0006 对序列版本表种类列
-- 的做法同款。序列标识可空——汇率那一格只有种类没有标识。
--
-- 不回填（Decision 四）：本仓尚无租户、生产库里没有评价；迁移之前落册的隔离评价没有子表行，读面文案如实
-- 说计数覆盖的是子表有行的评价。

CREATE TABLE parcel_pricing.evaluation_issue (
    evaluation_id   text        NOT NULL,
    ordinal         integer     NOT NULL,

    code            text        NOT NULL,
    series_kind     text,
    series_id       text,

    CONSTRAINT evaluation_issue_pkey
        PRIMARY KEY (evaluation_id, ordinal),

    CONSTRAINT evaluation_issue_evaluation_fkey
        FOREIGN KEY (evaluation_id)
        REFERENCES parcel_pricing.evaluation (evaluation_id),

    CONSTRAINT evaluation_issue_ordinal_from_zero
        CHECK (ordinal >= 0),

    CONSTRAINT evaluation_issue_series_kind_closed
        CHECK (series_kind IS NULL OR series_kind IN ('FUEL_RATE', 'EXCHANGE_RATE', 'PUBLISHED_AMOUNT')),

    -- 标识只在有种类时才可能有：没有主体的问题项两格都空。
    CONSTRAINT evaluation_issue_series_id_needs_kind
        CHECK (series_id IS NULL OR series_kind IS NOT NULL),

    CONSTRAINT evaluation_issue_not_blank
        CHECK (
            btrim(evaluation_id) <> ''
            AND btrim(code) <> ''
            AND (series_kind IS NULL OR btrim(series_kind) <> '')
            AND (series_id IS NULL OR btrim(series_id) <> '')
        )
);

-- 分组计数按（原因码、序列种类）读，父表按租户与状态筛；这个索引服务那一问。
CREATE INDEX evaluation_issue_by_code_kind
    ON parcel_pricing.evaluation_issue (code, series_kind);
