-- 商品登记册（UC-WF-002 的商品修订；仓配履约 CONTEXT「商品」「商品别名」「组合商品」「包装层级」「商品申报资料」与「主数据登记」）。
-- 身份是（租户，货主客户账户，商品编码）。每笔修订是一份完整资料，不是增量；修订从 1 起只追加，货主口与代录口写同一条序列。绝不 UPDATE。
--
-- 「连续」「本租户已登记过仓」「不可改的格」「组成成立」「目录码与数值为正」由应用层的登记用例判。库上守三件事：
-- 同一修订号至多一笔（主键）；同一个判重位置（租户，提交途径，来源，键）至多一笔（唯一约束）；数值、折合数、数量为正，提交形状自洽。
-- 识别码的区间交叠不是唯一约束能表达的（起含、止不含，没给的一端无界，而且只比别的商品的当前修订），所以不在库上做排除约束。
-- 写入侧对同一货主取事务级咨询锁，锁内再判判重、修订号与识别码。绕过写入侧的直插不在这把锁的范围内。
--
-- 本表不对仓修订表建外键：商品不指名仓，「一个仓都没登记就答未配置」是应用层的答法。
-- 本迁移只建结构、不种任何行。仓配开关关着的部署也施加本迁移，表是空的。
--
-- 商品编码、别名、名称、单位名与申报各格逐字保存。四个可空的文本组（依据引用、出库方式、申报资料、声明的尺寸或重量）在 CHECK 里写明
-- 「整组都空」或「整组都齐」，避开 SQL 三值逻辑把 NULL 判成通过。

CREATE TABLE warehouse_fulfillment.goods_revision (
    tenant_id            text        NOT NULL,
    owner_account_id     text        NOT NULL,
    goods_code           text        NOT NULL,
    revision             integer     NOT NULL,

    goods_name           text        NOT NULL,
    base_unit            text        NOT NULL,
    track_batch          boolean     NOT NULL,
    track_expiry         boolean     NOT NULL,
    composite            boolean     NOT NULL,
    outbound_mode        text,

    declared_name           text,
    declared_hs_code        text,
    declared_origin_country text,
    declared_unit_price     numeric(15,6),
    declared_currency       text,

    submission_path    text        NOT NULL,
    submission_source  text        NOT NULL,
    request_key        text        NOT NULL,
    evidence_reference text,

    registrant    text        NOT NULL,
    registered_at timestamptz NOT NULL,

    CONSTRAINT goods_revision_pkey
        PRIMARY KEY (tenant_id, owner_account_id, goods_code, revision),

    CONSTRAINT goods_revision_submission_key
        UNIQUE (tenant_id, submission_path, submission_source, request_key),

    CONSTRAINT goods_revision_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(owner_account_id) <> ''
            AND btrim(goods_code) <> ''
            AND btrim(goods_name) <> ''
            AND btrim(base_unit) <> ''
            AND btrim(request_key) <> ''
            AND btrim(registrant) <> ''
        ),

    CONSTRAINT goods_revision_revision_positive
        CHECK (revision >= 1),

    CONSTRAINT goods_revision_submission_shape
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

    CONSTRAINT goods_revision_outbound_shape
        CHECK (
            (composite = false AND outbound_mode IS NULL)
            OR (composite = true AND outbound_mode IN ('BY_COMPONENT', 'AS_FINISHED_GOODS'))
        ),

    CONSTRAINT goods_revision_declared_text_blank
        CHECK (
            (declared_name IS NULL OR btrim(declared_name) <> '')
            AND (declared_hs_code IS NULL OR btrim(declared_hs_code) <> '')
            AND (declared_origin_country IS NULL OR btrim(declared_origin_country) <> '')
        ),

    CONSTRAINT goods_revision_price_pair
        CHECK (
            (declared_unit_price IS NULL AND declared_currency IS NULL)
            OR (
                declared_unit_price IS NOT NULL
                AND declared_unit_price > 0
                AND declared_currency IS NOT NULL
                AND btrim(declared_currency) <> ''
            )
        )
);

CREATE TABLE warehouse_fulfillment.goods_packaging_level (
    tenant_id        text    NOT NULL,
    owner_account_id text    NOT NULL,
    goods_code       text    NOT NULL,
    revision         integer NOT NULL,
    position         integer NOT NULL,
    unit_name        text    NOT NULL,
    factor           bigint  NOT NULL,

    CONSTRAINT goods_packaging_level_pkey
        PRIMARY KEY (tenant_id, owner_account_id, goods_code, revision, position),

    CONSTRAINT goods_packaging_level_name_key
        UNIQUE (tenant_id, owner_account_id, goods_code, revision, unit_name),

    CONSTRAINT goods_packaging_level_revision_fkey
        FOREIGN KEY (tenant_id, owner_account_id, goods_code, revision)
        REFERENCES warehouse_fulfillment.goods_revision (tenant_id, owner_account_id, goods_code, revision),

    CONSTRAINT goods_packaging_level_shape
        CHECK (position >= 0 AND factor > 0 AND btrim(unit_name) <> '')
);

CREATE TABLE warehouse_fulfillment.goods_alias (
    tenant_id        text        NOT NULL,
    owner_account_id text        NOT NULL,
    goods_code       text        NOT NULL,
    revision         integer     NOT NULL,
    position         integer     NOT NULL,
    alias_code       text        NOT NULL,
    alias_type       text        NOT NULL,
    platform_name    text,
    valid_from       timestamptz,
    valid_to         timestamptz,

    CONSTRAINT goods_alias_pkey
        PRIMARY KEY (tenant_id, owner_account_id, goods_code, revision, position),

    CONSTRAINT goods_alias_revision_fkey
        FOREIGN KEY (tenant_id, owner_account_id, goods_code, revision)
        REFERENCES warehouse_fulfillment.goods_revision (tenant_id, owner_account_id, goods_code, revision),

    CONSTRAINT goods_alias_shape
        CHECK (
            position >= 0
            AND btrim(alias_code) <> ''
            AND (
                (alias_type = 'BARCODE' AND platform_name IS NULL)
                OR (alias_type = 'PLATFORM_CODE' AND (platform_name IS NULL OR btrim(platform_name) <> ''))
            )
            AND (valid_from IS NULL OR valid_to IS NULL OR valid_to > valid_from)
        )
);

CREATE INDEX goods_alias_by_code
    ON warehouse_fulfillment.goods_alias (tenant_id, owner_account_id, alias_code);

CREATE TABLE warehouse_fulfillment.goods_component (
    tenant_id        text    NOT NULL,
    owner_account_id text    NOT NULL,
    goods_code       text    NOT NULL,
    revision         integer NOT NULL,
    position         integer NOT NULL,
    component_code   text    NOT NULL,
    quantity         bigint  NOT NULL,

    CONSTRAINT goods_component_pkey
        PRIMARY KEY (tenant_id, owner_account_id, goods_code, revision, position),

    CONSTRAINT goods_component_code_key
        UNIQUE (tenant_id, owner_account_id, goods_code, revision, component_code),

    CONSTRAINT goods_component_revision_fkey
        FOREIGN KEY (tenant_id, owner_account_id, goods_code, revision)
        REFERENCES warehouse_fulfillment.goods_revision (tenant_id, owner_account_id, goods_code, revision),

    CONSTRAINT goods_component_shape
        CHECK (position >= 0 AND quantity > 0 AND btrim(component_code) <> '')
);

CREATE TABLE warehouse_fulfillment.goods_declared_measurement (
    tenant_id        text          NOT NULL,
    owner_account_id text          NOT NULL,
    goods_code       text          NOT NULL,
    revision         integer       NOT NULL,
    position         integer       NOT NULL,
    level_name       text          NOT NULL,
    length_value     numeric(15,6),
    width_value      numeric(15,6),
    height_value     numeric(15,6),
    dimensions_unit  text,
    weight_value     numeric(15,6),
    weight_unit      text,

    CONSTRAINT goods_declared_measurement_pkey
        PRIMARY KEY (tenant_id, owner_account_id, goods_code, revision, position),

    CONSTRAINT goods_declared_measurement_revision_fkey
        FOREIGN KEY (tenant_id, owner_account_id, goods_code, revision)
        REFERENCES warehouse_fulfillment.goods_revision (tenant_id, owner_account_id, goods_code, revision),

    CONSTRAINT goods_declared_measurement_shape
        CHECK (
            position >= 0
            AND btrim(level_name) <> ''
            AND (
                (
                    length_value IS NULL AND width_value IS NULL AND height_value IS NULL AND dimensions_unit IS NULL
                )
                OR (
                    length_value IS NOT NULL AND length_value > 0
                    AND width_value IS NOT NULL AND width_value > 0
                    AND height_value IS NOT NULL AND height_value > 0
                    AND dimensions_unit IS NOT NULL AND btrim(dimensions_unit) <> ''
                )
            )
            AND (
                (weight_value IS NULL AND weight_unit IS NULL)
                OR (
                    weight_value IS NOT NULL AND weight_value > 0
                    AND weight_unit IS NOT NULL AND btrim(weight_unit) <> ''
                )
            )
            AND NOT (
                length_value IS NULL AND width_value IS NULL AND height_value IS NULL AND dimensions_unit IS NULL
                AND weight_value IS NULL AND weight_unit IS NULL
            )
        )
);
