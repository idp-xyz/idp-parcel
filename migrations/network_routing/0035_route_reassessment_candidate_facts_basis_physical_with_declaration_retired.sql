-- 复核给候选出价的包裹事实依据（0032 立的封闭集）去掉 PHYSICAL_WITH_DECLARATION。
--
-- 为什么去掉：物理事实整份取用（network-routing「包裹事实」）——取得了物理事实，重量与外廓都出自它，客户声明一格都不读、
-- 不补。物理事实唯一的来源是节点作业的实际测量，一条测量同时带毛重与长宽高，「只取得一部分」无从产生；这个值标的是
-- 「重量与外廓里一部分取物理事实、其余取客户声明」的混合出价，如今没有任何一条路造得出它。它留在封闭集里，库就仍收一个
-- 代码已不认的字面，要到读回时才暴露。
--
-- 存量为什么安全：这个值只有取数侧「按物理事实、缺的项补客户声明」那一支产生，而生产装配从未装过能取得物理事实的来源
-- （物理事实读口装的是「一项都没取得」，复核恒退回客户声明、标 DECLARATION_FALLBACK），库里不会有它。真有这样的行，
-- 加约束时校验失败、整份迁移回滚、旧约束仍在原处——那是该停下来看的信号（有人接了来源，或手写了行），不用 NOT VALID
-- 绕过，也不替人改写那些行。不种任何租户的行。
--
-- 只收窄这一条 CHECK，不动列。逐段成本出处里的依据落在 jsonb 的 facts_basis 里、没有 CHECK，没有可收的约束；那里读回
-- 认不出的字面（含这个值）由读回解析报错，与任何未知字面同样对待。

ALTER TABLE network_routing.route_reassessment
    DROP CONSTRAINT route_reassessment_candidate_facts_basis_closed;

ALTER TABLE network_routing.route_reassessment
    ADD CONSTRAINT route_reassessment_candidate_facts_basis_closed
        CHECK (candidate_facts_basis IS NULL OR candidate_facts_basis IN
            ('PHYSICAL', 'DECLARATION_FALLBACK', 'PHYSICAL_UNRESOLVED', 'NONE'));
