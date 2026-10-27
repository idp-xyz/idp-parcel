-- 复核给候选出价的包裹事实出自哪里（CONTEXT「预路由使用客户声明快照；首次收寄复核使用当时已经取得的物理事实，到站复核使用
-- 当前有效物理实测」）。复核不一定形成计划，不形成计划的复核不留成本，候选评估是按声明还是按实测判的就查不出来；这一格记下来。
--
-- 只在复核向候选成本取数侧要过成本的行上有值，其余为空：形态没声明、原计划在问成本之前就已定论的复核没有出过价。
-- 取值是封闭集，且不含 DECLARATION：那是预路由取客户声明的依据，复核在任何判断点上都不按它出价；该取物理事实而没有，
-- 退回客户声明出价时记 DECLARATION_FALLBACK，如实标明是退回。PHYSICAL_UNRESOLVED 是物理事实在而定不下用哪条，客户声明
-- 不得顶替实测，各段待判断；NONE 是该取的事实一项也没有、声明也没有。
--
-- 存量安全：新列全空，CHECK 对 NULL 恒成立，直接校验，不用 NOT VALID。

ALTER TABLE network_routing.route_reassessment
    ADD COLUMN candidate_facts_basis text;

ALTER TABLE network_routing.route_reassessment
    ADD CONSTRAINT route_reassessment_candidate_facts_basis_closed
        CHECK (candidate_facts_basis IS NULL OR candidate_facts_basis IN
            ('PHYSICAL', 'PHYSICAL_WITH_DECLARATION', 'DECLARATION_FALLBACK', 'PHYSICAL_UNRESOLVED', 'NONE'));
