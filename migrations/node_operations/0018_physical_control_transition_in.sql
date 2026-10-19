-- 节点控制转移册的转入一半（ADR-0214 决定七与文末「补记：票 62 落决定七时的补充裁定」，票 demo-chain-gaps/62）：交接转入立起的控制、
-- 转入专属的两格入账原因、转入一侧的「控制来源链待重算」，以及转出一侧交出节点声明不符的信息格。
--
-- 0017 已为转入留好表形：方向 IN、IN·ADVANCED 必带 HANDOVER_IN 控制、IN 行不带转出两格、在身控制的读法认 IN 行并同样折叠。本份只补
-- 0017 没写的几条。既有行都是转出（0017 起只有转出的写口），所以下面把原先不分方向的几格收窄到转出也不会拒任何在册行。

-- 交出节点声明不符的信息格（ADR-0214 决定四末条）：只在推进了的转出上，值是交接声明的交出节点，且不是被转出那一份控制的节点。没声明、
-- 声明的就是那个节点、没推进、转入，都不在场。
ALTER TABLE node_operations.physical_control_transition
    ADD COLUMN mismatched_releasing_node text;

ALTER TABLE node_operations.physical_control_transition
    ADD CONSTRAINT physical_control_transition_mismatched_releasing_node_shape
        CHECK (mismatched_releasing_node IS NULL
            OR (direction = 'OUT' AND outcome = 'ADVANCED'
                AND btrim(mismatched_releasing_node) <> '' AND mismatched_releasing_node <> node));

-- 转入立起的控制成立时刻就是这一版交接的业务时间：转入判「最近一次转出晚不晚于这一版」比的是同一个时刻。
ALTER TABLE node_operations.physical_control_transition
    ADD CONSTRAINT physical_control_transition_in_established_at_occurrence
        CHECK (NOT (direction = 'IN' AND outcome = 'ADVANCED') OR control_established_at = occurred_at);

-- 入账原因的全集以 0017 那一份为底，加转入专属的 CONTROL_STILL_ACTIVE、HANDOVER_PRECEDES_RELEASE。之后谁再重建这条 CHECK，都取此前
-- 各份的并集。
ALTER TABLE node_operations.physical_control_transition
    DROP CONSTRAINT physical_control_transition_reason_closed;

ALTER TABLE node_operations.physical_control_transition
    ADD CONSTRAINT physical_control_transition_reason_closed
        CHECK (reason IS NULL OR reason IN (
            'NO_ASSOCIATED_UNIT', 'SEVERAL_ASSOCIATED_UNITS', 'CONSOLIDATION_UNIT_PENDING_RULE',
            'NO_ACTIVE_CONTROL', 'SEVERAL_ACTIVE_CONTROLS', 'HANDOVER_PRECEDES_CONTROL',
            'CONTROL_SOURCE_CHAIN_PENDING_RECOMPUTE',
            'CONTROL_STILL_ACTIVE', 'HANDOVER_PRECEDES_RELEASE'));

-- 各原因的形状，以 0017 那一份为底：
--   - 对不上实物的三格两个方向都可能落，不带实物。
--   - 无在身控制、不止一份、交接早于控制只属转出；仍有在身控制、交接早于最近一次转出只属转入，后两格只带实物。
--   - 待重算两个方向都可能落：转出一侧带先前那次转出（控制与转出依据），转入一侧带先前立起的那一份 HANDOVER_IN 控制、不带转出——
--     那一份此后被谁转出，看同一份控制依据的转出行。转入行不带转出两格由 0017 的 transfer_whole 守。
ALTER TABLE node_operations.physical_control_transition
    DROP CONSTRAINT physical_control_transition_reason_shape;

ALTER TABLE node_operations.physical_control_transition
    ADD CONSTRAINT physical_control_transition_reason_shape
        CHECK (CASE COALESCE(reason, '')
                   WHEN 'NO_ASSOCIATED_UNIT' THEN unit IS NULL
                   WHEN 'SEVERAL_ASSOCIATED_UNITS' THEN unit IS NULL
                   WHEN 'CONSOLIDATION_UNIT_PENDING_RULE' THEN unit IS NULL
                   WHEN 'NO_ACTIVE_CONTROL' THEN direction = 'OUT' AND unit IS NOT NULL AND node IS NULL
                   WHEN 'SEVERAL_ACTIVE_CONTROLS' THEN direction = 'OUT' AND unit IS NOT NULL AND node IS NULL
                   WHEN 'HANDOVER_PRECEDES_CONTROL' THEN direction = 'OUT' AND node IS NOT NULL AND transfer_reference IS NULL
                                                          AND occurred_at <= control_established_at
                   WHEN 'CONTROL_STILL_ACTIVE' THEN direction = 'IN' AND unit IS NOT NULL AND node IS NULL
                   WHEN 'HANDOVER_PRECEDES_RELEASE' THEN direction = 'IN' AND unit IS NOT NULL AND node IS NULL
                   WHEN 'CONTROL_SOURCE_CHAIN_PENDING_RECOMPUTE' THEN
                       (direction = 'OUT' AND transfer_reference IS NOT NULL)
                    OR (direction = 'IN' AND node IS NOT NULL AND control_kind = 'HANDOVER_IN')
                   ELSE TRUE
               END);
