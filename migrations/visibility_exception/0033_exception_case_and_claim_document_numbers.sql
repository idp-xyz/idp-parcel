-- 异常案件与客户索赔项的单据号（ADR-0355 决定二、三、七、八；CONTEXT「异常案件」「客户索赔项」）。
--
-- 每张表加两格，都可空：document_number 是开立事务里取到的号；document_number_absence 只在开立时单据编号时区未配置、单据照常
-- 开立而没带号时落值。两格都空就是本迁移之前开立的旧行——「上线前开立」不另落一个值，旧行一行不改、不回填（决定三：
-- 不补号），本迁移因此在有数据的库上照样施加。
--
-- （租户，单据号）唯一是最后一道保险（决定八）：取号编排先查已用、撞了跳号，正常路径撞不上这把键；它挡的是绕过编排的
-- 写入。没有号的行（NULL）不参与唯一。号的格式归规则册，这里只拒空白，不复判模板。

ALTER TABLE visibility_exception.exception_case
    ADD COLUMN document_number text,
    ADD COLUMN document_number_absence text;

ALTER TABLE visibility_exception.exception_case
    ADD CONSTRAINT exception_case_document_number_not_blank
        CHECK (document_number IS NULL OR btrim(document_number) <> ''),
    -- 无号原因只有一个可落的值：有号的行不带原因，「上线前开立」两格都空。
    ADD CONSTRAINT exception_case_document_number_absence_closed_set
        CHECK (
            document_number_absence IS NULL
            OR (document_number_absence = 'NUMBERING_TIME_ZONE_NOT_CONFIGURED' AND document_number IS NULL)
        ),
    ADD CONSTRAINT exception_case_document_number_unique
        UNIQUE (tenant_id, document_number);

ALTER TABLE visibility_exception.claim_item
    ADD COLUMN document_number text,
    ADD COLUMN document_number_absence text;

ALTER TABLE visibility_exception.claim_item
    ADD CONSTRAINT claim_item_document_number_not_blank
        CHECK (document_number IS NULL OR btrim(document_number) <> ''),
    ADD CONSTRAINT claim_item_document_number_absence_closed_set
        CHECK (
            document_number_absence IS NULL
            OR (document_number_absence = 'NUMBERING_TIME_ZONE_NOT_CONFIGURED' AND document_number IS NULL)
        ),
    ADD CONSTRAINT claim_item_document_number_unique
        UNIQUE (tenant_id, document_number);

-- 案件撞号册：分诊走向是自动建案、案件单据号却撞上已用的号到了跳号上限（决定八）时，发作期、分诊结论与案件一样都不落，
-- 那一次命中记在这里，由派发进程按 next_attempt_at 定时原样再交分诊——信号多半只来一次，不记就没了。键与发作期定位同一把
-- （租户、对象、类型、因果条件）：同一原因再撞一次覆盖成最近那一次。命中各样照 signal_episode 的列记，够重建一次完整的
-- 命中；template 是撞上旧号的那份格式，只给人指认。这一原因的发作期一落（重驱建成，或另一份命中先走通了），这一行就销掉。
CREATE TABLE visibility_exception.case_numbering_collision (
    tenant_id         text        NOT NULL,
    parcel_ref        text        NOT NULL,
    kind_ref          text        NOT NULL,
    causal_condition  text        NOT NULL,
    rule_ref          text        NOT NULL,
    confidence_ref    text        NOT NULL,
    fact_basis        jsonb       NOT NULL,
    hit_at            timestamptz NOT NULL,
    template          text        NOT NULL,
    first_collided_at timestamptz NOT NULL,
    last_collided_at  timestamptz NOT NULL,
    collisions        integer     NOT NULL,
    next_attempt_at   timestamptz NOT NULL,

    CONSTRAINT case_numbering_collision_pkey PRIMARY KEY (tenant_id, parcel_ref, kind_ref, causal_condition),

    CONSTRAINT case_numbering_collision_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(parcel_ref) <> ''
            AND btrim(kind_ref) <> ''
            AND btrim(causal_condition) <> ''
            AND btrim(rule_ref) <> ''
            AND btrim(confidence_ref) <> ''
            AND btrim(template) <> ''
        ),
    CONSTRAINT case_numbering_collision_counts
        CHECK (collisions >= 1 AND last_collided_at >= first_collided_at)
);

-- 重驱每拍按到期时刻取一页，跨租户。
CREATE INDEX case_numbering_collision_due
    ON visibility_exception.case_numbering_collision (next_attempt_at);
