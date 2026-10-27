-- 内部合规限制登记册（票 docs-consistency/code/27，ADR-0241 决定二、ADR-0334）。
--
-- 内部合规限制是运营企业依据贸易合规规则形成的内部阻断决定，只由原限制责任来源解除，可部分解除。它与
-- regulatory_restriction（挂监管决定、只凭责任来源接受的监管结果整条解除）是两种限制，各用各的表：监管限制不长覆盖项、
-- 责任来源与部分解除，内部合规限制也不挂监管决定。
--
-- internal_compliance_restriction：限制本身——限制责任来源、依据、决定方式、实际决定方、解除条件与开始时间。键=租户+限制标识。
-- 建立后不改写：身份外的一切变化都是追加的决定。
--
-- internal_restriction_coverage：限制的覆盖项，一行即「对一个明确范围的一个受限动作的覆盖」。同一限制可有多个覆盖项，
-- 逐项保存；开始时的适用性（有效 / 待确认 / 冲突）随行写下，后来的变化只由决定表给出。不设监管边界列：覆盖按范围加动作成立，
-- 不因程序不同而分叉。
--
-- internal_restriction_decision：限制责任来源对覆盖项形成的追加式决定，确认（待确认或冲突的覆盖项由来源确认为有效）或解除
-- （来源解除覆盖项，可部分）。sequence 是接受次序：读回时按它重演，每一步都过领域构造重验。决定标识由请求方给出，
-- 同一标识同一内容重放答已有、异内容答冲突（应用层比，库里的主键挡住第二份）。
--
-- internal_restriction_decision_item：决定作用的覆盖项。同一覆盖项至多一次确认、至多一次解除由部分唯一索引在库里再守一遍；
-- 并发由应用层对限制行取锁串行（SELECT ... FOR UPDATE，限制行是锁序里这一册唯一的锁）。
--
-- 带 NULL 列的 CHECK 一律走 IS NULL 显式分支；本迁移的列都 NOT NULL，没有这个问题。

CREATE TABLE customs_compliance.internal_compliance_restriction (
    tenant_id             text        NOT NULL,
    restriction_id        text        NOT NULL,

    source_ref            text        NOT NULL,
    basis_ref             text        NOT NULL,
    decision_mode         text        NOT NULL,
    decided_by_ref        text        NOT NULL,
    release_condition_ref text        NOT NULL,
    started_at            timestamptz NOT NULL,

    CONSTRAINT internal_compliance_restriction_pkey
        PRIMARY KEY (tenant_id, restriction_id),

    CONSTRAINT internal_compliance_restriction_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(restriction_id) <> ''
            AND btrim(source_ref) <> ''
            AND btrim(basis_ref) <> ''
            AND btrim(decided_by_ref) <> ''
            AND btrim(release_condition_ref) <> ''
        ),
    -- 与 domain 的 DecisionMode 的 String 同词。
    CONSTRAINT internal_compliance_restriction_mode_closed
        CHECK (decision_mode IN ('RULE', 'MANUAL'))
);

CREATE TABLE customs_compliance.internal_restriction_coverage (
    tenant_id        text NOT NULL,
    restriction_id   text NOT NULL,
    scope_ref        text NOT NULL,
    action           text NOT NULL,
    initial_standing text NOT NULL,

    CONSTRAINT internal_restriction_coverage_pkey
        PRIMARY KEY (tenant_id, restriction_id, scope_ref, action),
    CONSTRAINT internal_restriction_coverage_restriction
        FOREIGN KEY (tenant_id, restriction_id)
        REFERENCES customs_compliance.internal_compliance_restriction (tenant_id, restriction_id),

    CONSTRAINT internal_restriction_coverage_not_blank
        CHECK (btrim(scope_ref) <> ''),
    -- 与 domain 的 GuardedAction / CoverageStanding 的 String 同词。
    CONSTRAINT internal_restriction_coverage_action_closed
        CHECK (action IN ('OUTBOUND_RELEASE', 'LOADING_DEPARTURE', 'CROSS_CUSTOMS_MOVEMENT', 'FINAL_DELIVERY')),
    CONSTRAINT internal_restriction_coverage_standing_closed
        CHECK (initial_standing IN ('IN_FORCE', 'UNCONFIRMED', 'CONFLICTING'))
);

-- 准入判断按（租户，范围）盘出覆盖它的全部限制。
CREATE INDEX internal_restriction_coverage_by_scope
    ON customs_compliance.internal_restriction_coverage (tenant_id, scope_ref);

CREATE TABLE customs_compliance.internal_restriction_decision (
    tenant_id      text        NOT NULL,
    restriction_id text        NOT NULL,
    decision_id    text        NOT NULL,

    kind           text        NOT NULL,
    source_ref     text        NOT NULL,
    evidence_ref   text        NOT NULL,
    decided_by_ref text        NOT NULL,
    authority_ref  text        NOT NULL,
    decided_at     timestamptz NOT NULL,
    sequence       bigint      GENERATED ALWAYS AS IDENTITY,

    CONSTRAINT internal_restriction_decision_pkey
        PRIMARY KEY (tenant_id, restriction_id, decision_id),
    -- 决定的种类要在条目表里被外键带过去，条目表据此在库里守「同一覆盖项至多一次确认、一次解除」。
    CONSTRAINT internal_restriction_decision_kind_key
        UNIQUE (tenant_id, restriction_id, decision_id, kind),
    CONSTRAINT internal_restriction_decision_restriction
        FOREIGN KEY (tenant_id, restriction_id)
        REFERENCES customs_compliance.internal_compliance_restriction (tenant_id, restriction_id),

    CONSTRAINT internal_restriction_decision_not_blank
        CHECK (
            btrim(decision_id) <> ''
            AND btrim(source_ref) <> ''
            AND btrim(evidence_ref) <> ''
            AND btrim(decided_by_ref) <> ''
            AND btrim(authority_ref) <> ''
        ),
    CONSTRAINT internal_restriction_decision_kind_closed
        CHECK (kind IN ('CONFIRM', 'RELEASE'))
);

CREATE TABLE customs_compliance.internal_restriction_decision_item (
    tenant_id      text NOT NULL,
    restriction_id text NOT NULL,
    decision_id    text NOT NULL,
    kind           text NOT NULL,
    scope_ref      text NOT NULL,
    action         text NOT NULL,

    CONSTRAINT internal_restriction_decision_item_pkey
        PRIMARY KEY (tenant_id, restriction_id, decision_id, scope_ref, action),
    CONSTRAINT internal_restriction_decision_item_decision
        FOREIGN KEY (tenant_id, restriction_id, decision_id, kind)
        REFERENCES customs_compliance.internal_restriction_decision (tenant_id, restriction_id, decision_id, kind),
    -- 决定只能作用于这条限制自己的覆盖项。
    CONSTRAINT internal_restriction_decision_item_coverage
        FOREIGN KEY (tenant_id, restriction_id, scope_ref, action)
        REFERENCES customs_compliance.internal_restriction_coverage (tenant_id, restriction_id, scope_ref, action)
);

CREATE UNIQUE INDEX internal_restriction_one_release_per_coverage
    ON customs_compliance.internal_restriction_decision_item (tenant_id, restriction_id, scope_ref, action)
    WHERE kind = 'RELEASE';

CREATE UNIQUE INDEX internal_restriction_one_confirmation_per_coverage
    ON customs_compliance.internal_restriction_decision_item (tenant_id, restriction_id, scope_ref, action)
    WHERE kind = 'CONFIRM';
