-- 来源连接器绑定（ADR-0099 决定六；票 pricing-reference-series-operations/06）。
--
-- 这是实例半边的格：租户对某一条计价参考序列声明由哪种连接器、从哪个定位符抓、按哪个口径、
-- 以谁的名义登记、多久抓一次、免不免人工复核。机制只建格与约束，**出厂零行**——任何一行都由
-- 租户经受控口（cmd/parcel-pricing-feed）登进来，本迁移不种任何真实来源。
--
-- 键 =（租户 + 序列 + 绑定版本）。行只增不改：改声明就登新版本；「当前在用的绑定」不是一列，
-- 是按登记时刻派生的结论（最近登记的一版），与在用序列版本的派生同一形状——存一列就有第二个
-- 来源，某次登记忘了改列，两处便各说各话。
--
-- review_exemption 三格封闭且 NOT NULL 无默认：「未声明 = 需人工复核」是租户显式写下的取值，
-- 不是列默认出来的（CONTEXT：产品不设默认）。
--
-- 口径引用三列成对（含 digest）：绑定没有快照，引用要能原样重建，digest 只能落列；汇率必带口径
-- （CONTEXT：不接受未声明口径的裸汇率）。
--
-- 抓取原文本体不进本表也不进任何业务表（ADR-0092 决定二）：凭证（摘要、抓取时刻、地址）随
-- 序列版本的期次进登记快照，本体走出向端口。

CREATE TABLE parcel_pricing.source_connector_binding (
    tenant_id           text        NOT NULL,
    series_id           text        NOT NULL,
    binding_version     text        NOT NULL,

    connector_kind      text        NOT NULL,
    source_identifier   text        NOT NULL,
    source_locator      text        NOT NULL,
    series_kind         text        NOT NULL,

    quote_basis_id      text,
    quote_basis_version text,
    quote_basis_digest  text,

    registrant          text        NOT NULL,
    cadence             text,
    review_exemption    text        NOT NULL,

    registered_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT source_connector_binding_pkey
        PRIMARY KEY (tenant_id, series_id, binding_version),

    CONSTRAINT source_connector_binding_series_kind_closed
        CHECK (series_kind IN ('FUEL_RATE', 'EXCHANGE_RATE')),

    CONSTRAINT source_connector_binding_review_exemption_closed
        CHECK (review_exemption IN ('EXEMPT', 'NOT_EXEMPT', 'UNDECLARED')),

    CONSTRAINT source_connector_binding_quote_basis_paired
        CHECK (
            (quote_basis_id IS NULL AND quote_basis_version IS NULL AND quote_basis_digest IS NULL)
            OR (quote_basis_id IS NOT NULL AND btrim(quote_basis_id) <> ''
                AND quote_basis_version IS NOT NULL AND btrim(quote_basis_version) <> ''
                AND quote_basis_digest IS NOT NULL AND btrim(quote_basis_digest) <> '')
        ),

    CONSTRAINT source_connector_binding_fx_demands_quote_basis
        CHECK (series_kind <> 'EXCHANGE_RATE' OR quote_basis_id IS NOT NULL),

    CONSTRAINT source_connector_binding_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(series_id) <> ''
            AND btrim(binding_version) <> ''
            AND btrim(connector_kind) <> ''
            AND btrim(source_identifier) <> ''
            AND btrim(source_locator) <> ''
            AND btrim(registrant) <> ''
            AND (cadence IS NULL OR btrim(cadence) <> '')
        )
);
