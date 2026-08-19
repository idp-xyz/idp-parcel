-- 追踪投影版本只增不改写（ADR-0065）：重新派生追加新版本行，原版本连同其条目、
-- 所用映射版本和派生时间一并留存，并可按版本读回。当前版由 tracking_projection_current
-- 的显式标记指名——读当前版走标记直取，不扫描历史。
--
-- 0007 头注称「重派生换版本改同一行，历史由 prior_version 指回承担」——那句与它
-- 自己造成的效果不符：被指回的行已被同一句 DO UPDATE 覆盖，指针在，靶子不在
-- （ADR-0065 Context）。已施加迁移是不可变资产（migrations 包自述，校验和漂移即告警），
-- 同义句不改写历史文件，由本迁移在此更正。
--
-- 条目仍进 jsonb：只引用源事实与映射版本，不复制源事实内容。第 k 版存 k 条条目，
-- 保留期限与归档属实例半边（参数登记册），本迁移不设默认值。

CREATE TABLE visibility_exception.tracking_projection_version (
    tenant_id     text        NOT NULL,
    parcel_ref    text        NOT NULL,
    version_id    text        NOT NULL,
    derived_at    timestamptz NOT NULL,
    prior_version text,
    entries       jsonb       NOT NULL,

    CONSTRAINT tracking_projection_version_pkey PRIMARY KEY (tenant_id, version_id),

    -- 供当前标记的三维外键引用：两维只保证版本存在，保证不了标记指的是同一包裹
    -- 名下的版本。
    CONSTRAINT tracking_projection_version_parcel_unique
        UNIQUE (tenant_id, parcel_ref, version_id),

    CONSTRAINT tracking_projection_version_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(parcel_ref) <> ''
            AND btrim(version_id) <> ''
        ),
    CONSTRAINT tracking_projection_version_prior_not_self
        CHECK (
            prior_version IS NULL
            OR (btrim(prior_version) <> '' AND prior_version <> version_id)
        ),
    CONSTRAINT tracking_projection_version_entries_present
        CHECK (jsonb_typeof(entries) = 'array' AND jsonb_array_length(entries) >= 1)
);

-- prior_version 留裸文本不设外键：存量行的前身已被旧 UPSERT 覆盖、无从追回，指针
-- 如实留存；自本迁移起版本不再消失，新链条的前身都读得回。

CREATE TABLE visibility_exception.tracking_projection_current (
    tenant_id  text NOT NULL,
    parcel_ref text NOT NULL,
    version_id text NOT NULL,

    CONSTRAINT tracking_projection_current_pkey PRIMARY KEY (tenant_id, parcel_ref),

    -- 非空形状由外键传递保证：三列必须整体命中一条版本行，而版本行已拦空白。
    CONSTRAINT tracking_projection_current_names_stored_version
        FOREIGN KEY (tenant_id, parcel_ref, version_id)
        REFERENCES visibility_exception.tracking_projection_version (tenant_id, parcel_ref, version_id)
);

-- 存量行是各包裹现存的当前版：迁入版本表并立当前标记，先版本后标记以满足外键。

INSERT INTO visibility_exception.tracking_projection_version
    (tenant_id, parcel_ref, version_id, derived_at, prior_version, entries)
SELECT tenant_id, parcel_ref, version_id, derived_at, prior_version, entries
  FROM visibility_exception.tracking_projection;

INSERT INTO visibility_exception.tracking_projection_current
    (tenant_id, parcel_ref, version_id)
SELECT tenant_id, parcel_ref, version_id
  FROM visibility_exception.tracking_projection;

DROP TABLE visibility_exception.tracking_projection;
