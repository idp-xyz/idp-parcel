-- 单据编号规则册（ADR-0355 决定五、六、七）：编号技术组件的配置，不是任何上下文的领域数据，所以独占一个
-- schema，不借平台序列表那个 schema（那张表的资产归平台库），也不借任何业务 schema。
--
-- 两张表都只增不改：换规则、换时区都是新的一行，不 UPDATE 旧行——已开立的单据是按旧行出的号，改掉旧行就
-- 答不出「那一刻生效的是哪一版」。本迁移不种任何行：租户的规则版本与单据编号时区都是租户取值，产品默认
-- 模板随代码发布、不进库。
--
-- 只存类型代码，不引用任何单据表：册不拥有单据（CONTEXT-MAP「公司级技术组件」）。

CREATE TABLE document_numbering.rule_version (
    tenant_id      text        NOT NULL,
    type_code      text        NOT NULL,
    effective_from timestamptz NOT NULL,
    template       text        NOT NULL,
    registered_by  text        NOT NULL,
    registered_at  timestamptz NOT NULL,
    inserted_at    timestamptz NOT NULL DEFAULT now(),

    -- 同一租户同一类型同一生效时刻只有一版：同内容再登是重放，异内容再登答生效时刻已被占，
    -- 两种答案都靠这把键判，不靠登记口记得先查。
    CONSTRAINT rule_version_pkey
        PRIMARY KEY (tenant_id, type_code, effective_from),

    CONSTRAINT rule_version_text_present
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(type_code) <> ''
            AND btrim(template) <> ''
            AND btrim(registered_by) <> ''
        )
);

CREATE TABLE document_numbering.numbering_time_zone_version (
    tenant_id      text        NOT NULL,
    effective_from timestamptz NOT NULL,
    time_zone      text        NOT NULL,
    registered_by  text        NOT NULL,
    registered_at  timestamptz NOT NULL,
    inserted_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT numbering_time_zone_version_pkey
        PRIMARY KEY (tenant_id, effective_from),

    CONSTRAINT numbering_time_zone_version_text_present
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(time_zone) <> ''
            AND btrim(registered_by) <> ''
        )
);
