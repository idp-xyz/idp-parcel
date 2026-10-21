-- 仍适用线上的改善改路（ADR-0173 决定三）也形成`已改路`。那一行的原计划仍可执行，没有失效依据；它的依据是改善本身。
-- 0004 的 rerouted_shape 仍按「`已改路`只出自失效线」要求失效依据在场，改善改路那一行写不进来。
--
-- improvement_basis 记被替代计划成本、新计划成本、币种与所用阈值（行模型见 route_reassessment.go 的
-- improvementBasisRow），内容由读回时的领域构造门复验，库面只钉在场与类型。`已改路`的依据改为失效依据或改善依据恰有其一：
-- 两者都缺说不出为什么改，两者都在分不出是哪条线改的。改善依据只出现在`已改路`上。
--
-- 存量安全：新列全空；存量`已改路`都出自失效线、都带失效依据，满足恰有其一；其余结论的行改善依据为空，满足后一条。
-- 两条 CHECK 都直接校验存量，不用 NOT VALID。前向 DROP+ADD 理由同 0004；表达式其余各项与 0004 相同。不要改写 0002—0004。

ALTER TABLE network_routing.route_reassessment
    ADD COLUMN improvement_basis jsonb;

ALTER TABLE network_routing.route_reassessment
    DROP CONSTRAINT route_reassessment_rerouted_shape;

-- 两侧都是 IS NOT NULL，不会是 NULL，<> 给确定的真假。
ALTER TABLE network_routing.route_reassessment
    ADD CONSTRAINT route_reassessment_rerouted_shape
    CHECK (conclusion <> 'REROUTED' OR (
        reviewed_plan IS NOT NULL
        AND (lapse_basis IS NOT NULL) <> (improvement_basis IS NOT NULL)
        AND new_plan IS NOT NULL AND decision IS NOT NULL
        AND suggestion IS NULL
        AND reroute_state IS NOT DISTINCT FROM 'AUTOMATIC_ALLOWED'));

-- 改善依据须是对象：数组或标量要等读回时才在构造门上炸，写入处就拒。
ALTER TABLE network_routing.route_reassessment
    ADD CONSTRAINT route_reassessment_improvement_basis_on_rerouted
    CHECK (improvement_basis IS NULL OR (
        conclusion = 'REROUTED'
        AND jsonb_typeof(improvement_basis) = 'object'));
