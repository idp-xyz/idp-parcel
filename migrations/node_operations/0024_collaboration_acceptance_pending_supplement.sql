-- 承接决定加第四值「待补充」与轮次（票 docs-consistency/code/18）。
--
-- 待补充不形成承接，补充到位后的决定是下一轮的新决定、回指它，原待补充行不改写，所以主键从（租户+事项）
-- 扩为（租户+事项+轮次）；存量行都是第 1 轮。「第 n+1 轮之前的第 n 轮必为待补充」由编排判，库面只钉
-- 每一轮自身的形状——跨行约束要触发器，而同形的幂等键判断在本包都在编排里。
--
-- 待补充行：缺口清单非空、承接范围为空；其余三值缺口为空。依据在待补充上可有可无。CHECK 沿用 0002 的
-- 纪律：可空列先验 IS NULL / IS NOT NULL，jsonb 先验 IS NOT NULL 再取长度。

ALTER TABLE node_operations.collaboration_acceptance
    ADD COLUMN round integer NOT NULL DEFAULT 1,
    ADD COLUMN gaps  jsonb   NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE node_operations.collaboration_acceptance
    DROP CONSTRAINT collaboration_acceptance_pkey,
    ADD CONSTRAINT collaboration_acceptance_pkey
        PRIMARY KEY (tenant_id, item_ref, round),
    ADD CONSTRAINT collaboration_acceptance_round_positive
        CHECK (round >= 1),
    DROP CONSTRAINT collaboration_acceptance_decision_closed,
    ADD CONSTRAINT collaboration_acceptance_decision_closed
        CHECK (decision IN ('ACCEPTED', 'DECLINED', 'PARTIALLY_ACCEPTED', 'PENDING_SUPPLEMENT')),
    ADD CONSTRAINT collaboration_acceptance_gaps_shaped
        CHECK (gaps IS NOT NULL AND jsonb_typeof(gaps) = 'array'),
    DROP CONSTRAINT collaboration_acceptance_decision_shape,
    ADD CONSTRAINT collaboration_acceptance_decision_shape
        CHECK (
            (decision = 'ACCEPTED'
                AND basis IS NULL
                AND jsonb_array_length(accepted_units) > 0
                AND jsonb_array_length(accepted_actions) > 0
                AND jsonb_array_length(gaps) = 0)
            OR (decision = 'DECLINED'
                AND basis IS NOT NULL AND btrim(basis) <> ''
                AND jsonb_array_length(accepted_units) = 0
                AND jsonb_array_length(accepted_actions) = 0
                AND jsonb_array_length(gaps) = 0)
            OR (decision = 'PARTIALLY_ACCEPTED'
                AND basis IS NOT NULL AND btrim(basis) <> ''
                AND jsonb_array_length(accepted_units) > 0
                AND jsonb_array_length(accepted_actions) > 0
                AND jsonb_array_length(gaps) = 0)
            OR (decision = 'PENDING_SUPPLEMENT'
                AND jsonb_array_length(accepted_units) = 0
                AND jsonb_array_length(accepted_actions) = 0
                AND jsonb_array_length(gaps) > 0)
        );
