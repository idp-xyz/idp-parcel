-- 节点关务区域登记册与动作准入判断册（票 demo-chain-gaps/31，ADR-0206）。所有权取 CONTEXT 所有权句：节点是谁归
-- network-routing，节点落在哪个关务区域、两节点之间算不算跨关务区域归本上下文。区域取值属租户取值（PAR-CUS-08），
-- 本迁移不含任何实例默认值；演示走 SYN- 合成种子。

-- 节点关务区域：一行即「该节点在该生效区间内属于该关务区域」。版本维形状照 0013 口岸目录：起点随登记给出并入键，
-- 终点不是登记输入——后继版本登记时落定，NULL 即尚无终点；同（租户，节点）区间不重叠由排他约束交付，一个评估
-- 时点至多解析出一版。node_ref 按 NR 节点代码拼写不透明保存，不设外键：节点目录在另一个上下文里。
CREATE TABLE customs_compliance.node_customs_zone (
    tenant_id     text        NOT NULL,
    node_ref      text        NOT NULL,
    applies_from  timestamptz NOT NULL,
    applies_until timestamptz,

    zone_ref      text        NOT NULL,

    CONSTRAINT node_customs_zone_pkey
        PRIMARY KEY (tenant_id, node_ref, applies_from),

    CONSTRAINT node_customs_zone_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(node_ref) <> '' AND btrim(zone_ref) <> ''),

    CONSTRAINT node_customs_zone_interval_ordered
        CHECK (applies_until IS NULL OR applies_until > applies_from),

    CONSTRAINT node_customs_zone_no_overlap
        EXCLUDE USING gist (
            tenant_id WITH =,
            node_ref WITH =,
            tstzrange(applies_from, applies_until) WITH &&
        )
);

-- 动作准入判断：一行即一次请求在当时三源上的判断。判断标识由内容派生（租户、对象、意图、动作时点、区域读数、
-- 逐动作答复与依据），同一请求在同样三源上重放撞同一主键；三源一变即换标识另成一行，旧行不改。只建立、不更新，
-- 也不入队（今天没有订阅者，ADR-0049）。
--
-- 逐动作的答复与依据存 jsonb：依据的形状（种类、范围、程序、来源）随答复格变化，拆表只会多出一串只为装它的连接；
-- 整次答复另成一列并收进封闭词表，按对象查「最近一次答了什么」不必解 jsonb。
CREATE TABLE customs_compliance.action_admission_judgement (
    tenant_id       text        NOT NULL,
    judgement_id    text        NOT NULL,

    object_ref      text        NOT NULL,
    intent_kind     text        NOT NULL,
    node_ref        text        NOT NULL,
    -- NULL 即意图没有去向（交付；未给去向的交出）。
    onward_node_ref text,
    action_at       timestamptz NOT NULL,
    cross_zone      text        NOT NULL,
    answer          text        NOT NULL,
    zones           jsonb       NOT NULL,
    findings        jsonb       NOT NULL,
    judged_at       timestamptz NOT NULL,

    CONSTRAINT action_admission_judgement_pkey
        PRIMARY KEY (tenant_id, judgement_id),

    CONSTRAINT action_admission_judgement_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(judgement_id) <> ''
            AND btrim(object_ref) <> ''
            AND btrim(node_ref) <> ''
            AND (onward_node_ref IS NULL OR btrim(onward_node_ref) <> '')
        ),

    -- 三个封闭词表与 domain 的 ActionIntentKind / CrossZoneState / AdmissionAnswer 的 String 同词。
    CONSTRAINT action_admission_judgement_intent_closed
        CHECK (intent_kind IN ('DEPARTURE', 'HANDOVER', 'DELIVERY')),
    -- 出发必有终点（领域构造同门）。
    CONSTRAINT action_admission_judgement_departure_has_onward
        CHECK (intent_kind <> 'DEPARTURE' OR onward_node_ref IS NOT NULL),
    CONSTRAINT action_admission_judgement_cross_zone_closed
        CHECK (cross_zone IN ('NOT_ASKED', 'SAME_ZONE', 'CROSSING', 'UNDETERMINED')),
    CONSTRAINT action_admission_judgement_answer_closed
        CHECK (answer IN ('NOT_GUARDED', 'ADMITTED', 'UNDECIDED', 'NOT_CONFIGURED', 'BLOCKED')),

    CONSTRAINT action_admission_judgement_shapes
        CHECK (
            jsonb_typeof(zones) = 'array'
            AND jsonb_typeof(findings) = 'array'
            AND jsonb_array_length(findings) >= 1
        )
);

CREATE INDEX action_admission_judgement_by_object
    ON customs_compliance.action_admission_judgement (tenant_id, object_ref, action_at);

-- 准入判断按申报范围盘放行事实：0015 只按（租户，来源标识）建了主键，按范围查会扫全表。
CREATE INDEX release_outcome_by_scope
    ON customs_compliance.release_outcome (tenant_id, scope_ref);
