-- 范围版本覆盖关系边：治理侧库面的权威事实。关系要被准入查询（FindUnresumedSuspension）
-- 读到，只有库面有落点；表结构由关系语义定死（范围版本引用对、边的种类、所属决定引用、
-- 登记时点），不含任何实例值——结构先行同 ADR-0068，这里的形状由语义背书，不取决于
-- 租户材料的样子。
--
-- 边随范围版本升版那次 Go/No-Go 决定一并登记：外键指回 stage_review——无决定就无关系
-- 登记，结构性成立。同一有序对至多一条边，第二份由主键拦住（治理记录不可覆盖）。
-- 承继有向，只在登记的方向成立；互不相干是对称事实，读侧按双向解释。自指边无意义。
CREATE TABLE pilot_governance.scope_version_relation (
    successor_scope   text        NOT NULL,
    predecessor_scope text        NOT NULL,
    relation_kind     text        NOT NULL,
    objective         text        NOT NULL,
    candidate_set_id  text        NOT NULL,
    registered_at     timestamptz NOT NULL,
    inserted_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT scope_version_relation_pkey
        PRIMARY KEY (successor_scope, predecessor_scope),
    CONSTRAINT scope_version_relation_decision_fkey
        FOREIGN KEY (objective, candidate_set_id)
        REFERENCES pilot_governance.stage_review (objective, candidate_set_id),
    CONSTRAINT scope_version_relation_not_blank
        CHECK (
            btrim(successor_scope) <> ''
            AND btrim(predecessor_scope) <> ''
            AND btrim(objective) <> ''
            AND btrim(candidate_set_id) <> ''
        ),
    CONSTRAINT scope_version_relation_kind_closed
        CHECK (relation_kind IN ('INHERITS_SUSPENSIONS', 'UNRELATED')),
    CONSTRAINT scope_version_relation_no_self_edge
        CHECK (successor_scope <> predecessor_scope)
);
