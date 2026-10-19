-- 节点控制转移册（ADR-0214 决定三，票 demo-chain-gaps/61）：权威运输交接推进节点控制时留下的东西，只追加。
--
-- 一行是一版权威运输交接在一个方向上的处理结果：转出（OUT）或转入（IN，票 demo-chain-gaps/62 落）。推进了的记 ADVANCED，带被转出或
-- 新成立的那一份控制；没推进的记 NOT_ADVANCED 并带原因——对不上作业实物、实物没有或不止一份在身控制、交接早于控制成立、撤回控制的
-- 更正要重算控制来源链。拒收与待确认不留行：它们不转出控制，也没有可入账的东西。
--
-- 主键是（租户，交接对象，交接范围，交接版本，方向）：同一版交接在同一方向上只处理一次，重投撞主键答已在册。作业实物不进主键——对不
-- 上实物的那几格没有实物可写；对得上的那几格，实物由交接对象经本上下文的当前关联认出，同一版交接认出的是同一件。
--
-- 收寄行的 control 列不改写（ADR-0214 决定三）：控制是否已转出，读收寄时经 control_with_transfer_out 把这本册上的转出折进去。所以
-- 「一份控制至多转出一次」在这里由部分唯一索引守：同一（租户，作业实物，控制依据）只有一行 OUT·ADVANCED。转出前取的实物锁与收寄认
-- 当前收寄是同一把，这本索引是锁之外的兜底。

CREATE TABLE node_operations.physical_control_transition (
    tenant_id              text        NOT NULL,
    handover_object        text        NOT NULL,
    handover_scope         text        NOT NULL,
    handover_version       text        NOT NULL,
    direction              text        NOT NULL,
    outcome                text        NOT NULL,
    reason                 text,
    unit                   text,
    node                   text,
    control_kind           text,
    control_basis          text,
    control_established_at timestamptz,
    transfer_reference     text,
    transferred_at         timestamptz,
    occurred_at            timestamptz NOT NULL,
    recorded_at            timestamptz NOT NULL,
    inserted_at            timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT physical_control_transition_pkey
        PRIMARY KEY (tenant_id, handover_object, handover_scope, handover_version, direction),

    CONSTRAINT physical_control_transition_key_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(handover_object) <> ''
           AND btrim(handover_scope) <> '' AND btrim(handover_version) <> ''),

    CONSTRAINT physical_control_transition_direction_closed
        CHECK (direction IN ('OUT', 'IN')),
    CONSTRAINT physical_control_transition_outcome_closed
        CHECK (outcome IN ('ADVANCED', 'NOT_ADVANCED')),
    CONSTRAINT physical_control_transition_reason_closed
        CHECK (reason IS NULL OR reason IN (
            'NO_ASSOCIATED_UNIT', 'SEVERAL_ASSOCIATED_UNITS', 'CONSOLIDATION_UNIT_PENDING_RULE',
            'NO_ACTIVE_CONTROL', 'SEVERAL_ACTIVE_CONTROLS', 'HANDOVER_PRECEDES_CONTROL',
            'CONTROL_SOURCE_CHAIN_PENDING_RECOMPUTE')),
    CONSTRAINT physical_control_transition_control_kind_closed
        CHECK (control_kind IS NULL OR control_kind IN ('NODE_INTAKE', 'HANDOVER_IN')),

    -- 原因只随没推进、没推进必带原因。
    CONSTRAINT physical_control_transition_reason_with_not_advanced
        CHECK ((outcome = 'NOT_ADVANCED') = (reason IS NOT NULL)),

    -- 控制五格同在同缺，带控制就带作业实物。
    CONSTRAINT physical_control_transition_control_whole
        CHECK ((node IS NULL) = (control_kind IS NULL)
           AND (node IS NULL) = (control_basis IS NULL)
           AND (node IS NULL) = (control_established_at IS NULL)
           AND (node IS NULL OR unit IS NOT NULL)),
    CONSTRAINT physical_control_transition_transfer_whole
        CHECK ((transfer_reference IS NULL) = (transferred_at IS NULL)
           AND (transfer_reference IS NULL OR (direction = 'OUT' AND node IS NOT NULL))),

    -- 推进了的带控制；转出带转出依据。
    CONSTRAINT physical_control_transition_advanced_carries_control
        CHECK (outcome <> 'ADVANCED' OR node IS NOT NULL),
    CONSTRAINT physical_control_transition_out_carries_transfer
        CHECK (NOT (direction = 'OUT' AND outcome = 'ADVANCED') OR transfer_reference IS NOT NULL),

    -- 各原因的形状：对不上实物的三格不带实物；无在身控制与不止一份只带实物；交接早于控制带那一份在身控制、不带转出；要重算的带
    -- 先前转出的那一份控制与它的转出依据。
    CONSTRAINT physical_control_transition_reason_shape
        CHECK (CASE COALESCE(reason, '')
                   WHEN 'NO_ASSOCIATED_UNIT' THEN unit IS NULL
                   WHEN 'SEVERAL_ASSOCIATED_UNITS' THEN unit IS NULL
                   WHEN 'CONSOLIDATION_UNIT_PENDING_RULE' THEN unit IS NULL
                   WHEN 'NO_ACTIVE_CONTROL' THEN unit IS NOT NULL AND node IS NULL
                   WHEN 'SEVERAL_ACTIVE_CONTROLS' THEN unit IS NOT NULL AND node IS NULL
                   WHEN 'HANDOVER_PRECEDES_CONTROL' THEN node IS NOT NULL AND transfer_reference IS NULL
                   WHEN 'CONTROL_SOURCE_CHAIN_PENDING_RECOMPUTE' THEN direction = 'OUT' AND transfer_reference IS NOT NULL
                   ELSE TRUE
               END)
);

-- 一份控制至多转出一次。
CREATE UNIQUE INDEX physical_control_transition_released_once
    ON node_operations.physical_control_transition (tenant_id, unit, control_basis)
    WHERE direction = 'OUT' AND outcome = 'ADVANCED';

-- 更正链上推进过没有：同一（对象，范围）下各版是一条更正链。
CREATE INDEX physical_control_transition_advanced_by_chain
    ON node_operations.physical_control_transition (tenant_id, handover_object, handover_scope, direction)
    WHERE outcome = 'ADVANCED';

-- 一件实物经转入成立的控制。
CREATE INDEX physical_control_transition_established_in
    ON node_operations.physical_control_transition (tenant_id, unit)
    WHERE direction = 'IN' AND outcome = 'ADVANCED';

-- 查阅口按记录时刻倒序上列。
CREATE INDEX physical_control_transition_by_recorded
    ON node_operations.physical_control_transition (tenant_id, recorded_at DESC);

-- 读控制时的折叠（ADR-0214 决定三）：一份控制（收寄行 control 列的形状）若在本册上已有转出，交回带 releasedBy / releasedAt 的那一份，
-- 否则原样交回。键是（租户，作业实物，控制依据），与 physical_control_transition_released_once 同形。已带转出的控制原样交回：经折叠
-- 读出、再被追加版照抄落册的那一份，转出已经在它自己身上。
--
-- 只合并数据，不判：读回的控制照旧经领域的 TransferOut 重放，转出早于成立之类的坏行在那里响。releasedAt 以 timestamptz 的 JSON
-- 形落（ISO 8601、带时区偏移），与写侧 RFC 3339 同读。
CREATE FUNCTION node_operations.control_with_transfer_out(p_tenant text, p_control jsonb)
RETURNS jsonb
LANGUAGE sql
STABLE
PARALLEL SAFE
AS $$
    SELECT CASE
        WHEN p_control IS NULL OR p_control ? 'releasedAt' THEN p_control
        ELSE COALESCE(
            (SELECT p_control || jsonb_build_object('releasedBy', t.transfer_reference, 'releasedAt', t.transferred_at)
               FROM node_operations.physical_control_transition AS t
              WHERE t.tenant_id = p_tenant
                AND t.unit = p_control ->> 'unit'
                AND t.control_basis = p_control ->> 'basis'
                AND t.direction = 'OUT'
                AND t.outcome = 'ADVANCED'),
            p_control)
    END
$$;
