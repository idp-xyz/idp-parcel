-- 内部 ETA 预测只增不改写，且两路来源各是一条预测线（ADR-0065 的追加版本原则；CONTEXT「承运商提供的预测与
-- 运营企业派生的预测分别保留来源和口径。新的 ETA 形成新版本，不覆盖历史预测」）：每个版本一行，写入后
-- 不改动；一条线的键是（租户，包裹，里程碑，来源），线上的当前版由 eta_prediction_current 的显式标记指名，
-- 标记是唯一的可变处。刷新追加新版本行并把标记移过去，被接替的版本连同输入、模型、区间、可信度一并留存。
--
-- 0006 的 eta_prediction 以（租户，包裹，里程碑）为键，刷新是同一行的 DO UPDATE，被指回的前一版随即被
-- 覆盖（指针在、靶子不在），键里也没有来源，一路来源的预测会盖掉另一路的。已施加迁移是不可变资产
-- （migrations 包自述，校验和漂移即告警），同义句不改写历史文件，由本迁移在此更正。
--
-- 存量行是各条线现存的当前版：迁入版本表并立当前标记，先版本后标记以满足外键，旧表随之退场。旧行的
-- prior_version 指向已被覆盖、无从追回的前身，如实带过来，不抹成首版；本迁移之后的链条上被接替的版本都读得回。
-- prior_version 与 ETA 判断（0031）对候选版本的引用都留裸文本、不设外键：前者的存量前身读不回，后者若设外键
-- 会让预测版本的保留期限与归档（属实例半边）被判断挡住。判断的候选引用 =（包裹，里程碑，来源，版本），
-- 恰是本表一行的键，新形成的判断引用的版本都读得回。
--
-- 租户是最高数据隔离边界（ADR-0003）：包裹引用只是租户内引用，缺租户维两个租户的同名键会共用一行。版本号
-- 在租户内唯一——预测版本同时是 Outbox 意图的认领标识（ADR-0043）。

DO $$
DECLARE
    duplicated record;
BEGIN
    -- 版本号本应由身份工厂在租户内唯一签发，旧表却没有对应约束；有重复就无法迁成「租户内按版本号取一行」，
    -- 先停下并指明是哪一个，别让主键冲突的原始报错淹没真正的原因。
    SELECT tenant_id, version_id, count(*) AS copies
      INTO duplicated
      FROM visibility_exception.eta_prediction
     GROUP BY tenant_id, version_id
    HAVING count(*) > 1
     ORDER BY tenant_id, version_id
     LIMIT 1;
    IF FOUND THEN
        RAISE EXCEPTION
            '无法迁移 ETA 预测：租户 % 下版本号 % 出现在 % 行（不同包裹或里程碑）。预测版本号必须在租户内唯一，请先核对并改正重复的版本号，再重新施加迁移。',
            duplicated.tenant_id, duplicated.version_id, duplicated.copies;
    END IF;
END
$$;

CREATE TABLE visibility_exception.eta_prediction_version (
    tenant_id      text        NOT NULL,
    parcel_ref     text        NOT NULL,
    milestone_ref  text        NOT NULL,
    source         text        NOT NULL,
    version_id     text        NOT NULL,

    inputs_ref     text        NOT NULL,
    model_ref      text        NOT NULL,
    range_from     timestamptz NOT NULL,
    range_to       timestamptz NOT NULL,
    confidence_ref text        NOT NULL,
    predicted_at   timestamptz NOT NULL,
    prior_version  text,
    recorded_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT eta_prediction_version_pkey PRIMARY KEY (tenant_id, version_id),

    -- 供当前标记的五维外键引用：版本号唯一只保证版本存在，保证不了标记指的是同一条线名下的版本。
    CONSTRAINT eta_prediction_version_line_unique
        UNIQUE (tenant_id, parcel_ref, milestone_ref, source, version_id),

    CONSTRAINT eta_prediction_version_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(parcel_ref) <> ''
            AND btrim(milestone_ref) <> ''
            AND btrim(version_id) <> ''
            AND btrim(inputs_ref) <> ''
            AND btrim(model_ref) <> ''
            AND btrim(confidence_ref) <> ''
        ),
    CONSTRAINT eta_prediction_version_source_closed
        CHECK (source IN ('CARRIER_PROVIDED', 'OPERATOR_DERIVED')),
    CONSTRAINT eta_prediction_version_range_order
        CHECK (range_to > range_from),
    CONSTRAINT eta_prediction_version_prior_not_self
        CHECK (
            prior_version IS NULL
            OR (btrim(prior_version) <> '' AND prior_version <> version_id)
        )
);

CREATE TABLE visibility_exception.eta_prediction_current (
    tenant_id      text NOT NULL,
    parcel_ref     text NOT NULL,
    milestone_ref  text NOT NULL,
    source         text NOT NULL,
    version_id     text NOT NULL,

    CONSTRAINT eta_prediction_current_pkey
        PRIMARY KEY (tenant_id, parcel_ref, milestone_ref, source),

    -- 非空形状由外键传递保证：五列必须整体命中一条版本行，而版本行已拦空白与来源取值。
    CONSTRAINT eta_prediction_current_names_stored_version
        FOREIGN KEY (tenant_id, parcel_ref, milestone_ref, source, version_id)
        REFERENCES visibility_exception.eta_prediction_version
            (tenant_id, parcel_ref, milestone_ref, source, version_id)
);

INSERT INTO visibility_exception.eta_prediction_version
    (tenant_id, parcel_ref, milestone_ref, source, version_id, inputs_ref, model_ref,
     range_from, range_to, confidence_ref, predicted_at, prior_version)
SELECT tenant_id, parcel_ref, milestone_ref, source, version_id, inputs_ref, model_ref,
       range_from, range_to, confidence_ref, predicted_at, prior_version
  FROM visibility_exception.eta_prediction;

INSERT INTO visibility_exception.eta_prediction_current
    (tenant_id, parcel_ref, milestone_ref, source, version_id)
SELECT tenant_id, parcel_ref, milestone_ref, source, version_id
  FROM visibility_exception.eta_prediction;

DROP TABLE visibility_exception.eta_prediction;
