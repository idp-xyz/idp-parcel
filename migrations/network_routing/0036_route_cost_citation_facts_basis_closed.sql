-- 逐段成本出处里的包裹事实依据（jsonb 里的 facts_basis 键）在库里收封闭集。
--
-- 为什么加：0032 立、0035 收窄的封闭集只钉在复核记录的 candidate_facts_basis 那一列上。同一个依据随逐段成本出处落在 jsonb
-- 里，那里没有任何约束，库收任何字面，要到读回时 costCitationsOfRows 先 ParseParcelFactsBasis、再过 NewPlannedLegCostCitation
-- 才报错。本迁移给每一处会落出处行模型（costCitationRow）的 jsonb 列各加一条 CHECK：读回会拒的，写入那一刻就拒，不躺到有人读它
-- 的那天。读回解析那道防线不撤，它挡的是约束管不到的行。
--
-- 落点以适配器里 json.Marshal 的行模型为准：计划行模型 planRow 的 cost_citations，落在 initial_route.plan、
-- route_reassessment.new_plan、authorized_reroute_decision.new_plan；改善依据行模型 improvementBasisRow 的
-- superseded_cost_citations，落在 route_reassessment.improvement_basis。
--
-- 封闭集是出处读得回的那几格依据，即 ParcelFactsBasis.PricesACandidate 为真的：DECLARATION、PHYSICAL、DECLARATION_FALLBACK。
-- 成本出处只记出了价的段，PHYSICAL_UNRESOLVED 与 NONE 没有出价，NewPlannedLegCostCitation 不让它们进出处，读回同样拒，所以库也
-- 不收；0035 已退役的 PHYSICAL_WITH_DECLARATION 与任何认不出的字面同样不收。这个集合与复核记录 candidate_facts_basis 那一列的
-- 封闭集不是同一个：那一列记的是这次复核按什么依据去要价，含没要到价的那几格。
--
-- 缺席为什么放行：facts_basis 键缺席即客户声明——包裹事实依据出现之前落库的出处没有这一格，读回不回填。所以列为 NULL、文档里
-- 没有出处数组、数组为空、某条出处没有这个键，都放行；键在场就必须是封闭集里的字符串，空串不算缺席（适配器写出处时 omitempty，
-- 从不写空串）。
--
-- 写法：只用内置的 jsonb_path_exists（IMMUTABLE），不建自定义函数。jsonpath 的 lax 模式里不同类型的比较结果是 unknown，过滤器
-- 当假处理，只写「不等于各字面」会把数字、对象、布尔放过去，所以先以 type() 断言它是字符串，再比字面；JSON null 也算在场，
-- 同样拒收。过滤落在出处这一层而不是落在 facts_basis 的值上：值是数组时 lax 模式会把它展开逐项比，["PHYSICAL"] 就混过去了。
-- CHECK 对 NULL 结果放行，写成会算出 NULL 的形式就会无声放过坏行；这里每个比较的两侧都已是字符串，只有真假。
--
-- 存量为什么安全：生产装配从未装过能取得物理事实的来源（物理事实读口装的是 NoPhysicalParcelFacts），出处的依据只会是
-- DECLARATION、DECLARATION_FALLBACK，或没有这一格的更早存量；没有出价的依据领域本就不让它进出处，适配器写不出它们。库里不会有
-- 越界字面。真有这样的行，加约束时校验失败、整份迁移回滚、一条约束也不留——那是该停下来看的信号（有人接了来源，或手写了行），
-- 不用 NOT VALID 绕过，也不替人改写那些行。不种任何租户的行。

ALTER TABLE network_routing.initial_route
    ADD CONSTRAINT initial_route_plan_facts_basis_closed
        CHECK (plan IS NULL OR NOT jsonb_path_exists(plan,
            '$.cost_citations[*] ? (exists(@.facts_basis) && (@.facts_basis.type() != "string" ||
                !(@.facts_basis == "DECLARATION" || @.facts_basis == "PHYSICAL"
                  || @.facts_basis == "DECLARATION_FALLBACK")))'));

ALTER TABLE network_routing.route_reassessment
    ADD CONSTRAINT route_reassessment_new_plan_facts_basis_closed
        CHECK (new_plan IS NULL OR NOT jsonb_path_exists(new_plan,
            '$.cost_citations[*] ? (exists(@.facts_basis) && (@.facts_basis.type() != "string" ||
                !(@.facts_basis == "DECLARATION" || @.facts_basis == "PHYSICAL"
                  || @.facts_basis == "DECLARATION_FALLBACK")))')),
    ADD CONSTRAINT route_reassessment_improvement_basis_facts_basis_closed
        CHECK (improvement_basis IS NULL OR NOT jsonb_path_exists(improvement_basis,
            '$.superseded_cost_citations[*] ? (exists(@.facts_basis) && (@.facts_basis.type() != "string" ||
                !(@.facts_basis == "DECLARATION" || @.facts_basis == "PHYSICAL"
                  || @.facts_basis == "DECLARATION_FALLBACK")))'));

ALTER TABLE network_routing.authorized_reroute_decision
    ADD CONSTRAINT authorized_reroute_decision_new_plan_facts_basis_closed
        CHECK (new_plan IS NULL OR NOT jsonb_path_exists(new_plan,
            '$.cost_citations[*] ? (exists(@.facts_basis) && (@.facts_basis.type() != "string" ||
                !(@.facts_basis == "DECLARATION" || @.facts_basis == "PHYSICAL"
                  || @.facts_basis == "DECLARATION_FALLBACK")))'));
