-- 准入范围对照（ADR-0180 决定一、二；票 operator-channel/19）：某租户在（能力、事实类型）上的生产写，在生效区间内落在
-- 治理登记册的哪个对象范围，带登记依据。册的列由产品定；行是租户取值（参数登记册 PAR-INT-11），本迁移不种任何行。
--
-- 对照只做坐标换算：「谁在写生产」的唯一权威仍是 pilot_governance 的生产权威区间，治理登记册照旧没有租户维（ADR-0083
-- 决定一）。对象范围是那本册里的不透明引用，本册只存、不解释。
--
-- 两张表只增不改，理由同 0001：结束是另一张表里的一行，被结束的对照是那段时间里它确实有效的证据。
--
-- 同键（租户、能力、事实类型）任一时刻至多一行生效（ADR-0180 决定二）。重叠由登记口在写入侧判，事务级咨询锁把判定与
-- 写入关在一起；库不重判——被结束的行只算到结束时刻，那要两张表合起来才答得出，CHECK 与排他约束都够不着。能力与事实
-- 类型的词表归产品策略（决定四），由领域构造门守，不在这里另抄一份封闭集。

CREATE TABLE access_identity.admission_scope_mapping (
    tenant_id           text        NOT NULL,
    mapping_id          text        NOT NULL,
    capability          text        NOT NULL,
    fact_kind           text        NOT NULL,
    object_scope        text        NOT NULL,
    effective_starts_at timestamptz NOT NULL,
    effective_ends_at   timestamptz,
    basis_ref           text        NOT NULL,
    recorded_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT admission_scope_mapping_pkey PRIMARY KEY (tenant_id, mapping_id),

    CONSTRAINT admission_scope_mapping_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(mapping_id) <> ''
            AND btrim(capability) <> ''
            AND btrim(fact_kind) <> ''
            AND btrim(object_scope) <> ''
            AND btrim(basis_ref) <> ''
        ),

    -- 含起点、不含终点，终点缺席即不设终点。
    CONSTRAINT admission_scope_mapping_interval_coherent
        CHECK (effective_ends_at IS NULL OR effective_ends_at > effective_starts_at)
);

-- 铸信封前那一查按键发问，登记时的重叠判定也按键取。
CREATE INDEX admission_scope_mapping_by_key
    ON access_identity.admission_scope_mapping (tenant_id, capability, fact_kind);

CREATE TABLE access_identity.admission_scope_mapping_end (
    tenant_id   text        NOT NULL,
    mapping_id  text        NOT NULL,
    ended_at    timestamptz NOT NULL,
    basis_ref   text        NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),

    -- 一行对照至多结束一次：结束是终局，再要就另登一行。
    CONSTRAINT admission_scope_mapping_end_pkey PRIMARY KEY (tenant_id, mapping_id),

    CONSTRAINT admission_scope_mapping_end_mapping_fkey
        FOREIGN KEY (tenant_id, mapping_id)
        REFERENCES access_identity.admission_scope_mapping (tenant_id, mapping_id),

    CONSTRAINT admission_scope_mapping_end_basis_not_blank
        CHECK (btrim(basis_ref) <> '')
);
