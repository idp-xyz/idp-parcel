-- 解析键登记面扩信用二维，让含 CREDIT_POLICY 的登记行形成立得起来的键
-- （ADR-0127 Consequences 点名的 PS 那一格，票 wiring-baseline-remainder/08）。
--
-- 0007 的名集里 CREDIT_POLICY 一直在——立表时信用政策还只是按范围选的版本壳，键上没有信用
-- 专属维度，放行它不会形成立不起来的键。ADR-0127 决定二给键加了 CreditSelector（商业权限
-- 等级 × 费用类型），纪律与结算选择器、价格方向相同：请求信用依据必填，其余请求必缺，部分
-- 给出即`输入未受理`。自那一刻起本表放行 CREDIT_POLICY 却没有那两维，含它的登记行经
-- FormResolutionKey 形成的键在闭包解析处一律答`输入未受理`——一个登记得进去、永远立不起来
-- 的键；而 ADR-0127 决定四让 settlement-accounting 账期分支的授信额度从闭包交出的信用依据取，
-- 「租户登记的解析键没要求这一项」的恢复动作是补解析键与正文。登记面不承载两维，那条路就
-- 没有入口。这里补上。
--
-- 两维在租户配置里是稳的：按哪一等级、哪一费用类型授信是租户与货主客户之间的商业约定，
-- 不随闭包里任何成员换版而变。法人与时点不在其中：键上已有 legal_entity_ref 与 anchor_at，
-- 重复携带就允许两者不一致。与结算三维不同，信用没有「由闭包解出」的那一维——两维都由
-- 登记方给全，所以下面的约束也没有 0008 那条「要结算就要合同」的对应项。
--
-- 两列都是非空引用值而不是封闭名集：商业权限等级是租户的版本化业务授权，party-commercial
-- 不预设它有哪几档（AuthorityLevel 是 requiredValue），费用类型同理；所以这里没有像
-- required_bases 那样的名集约束，只守「在场即非空」。
--
-- 迁移不回改 0007/0008：只加列、加约束。名集不变（CREDIT_POLICY 本来就在），PRICE_RULE 仍挡着
-- ——价格方向那一维与信用二维不是同一件事，是另一道决定，不在这里预留。
--
-- ADD CONSTRAINT 而无既有行回填：CI 走的是空库、本机门禁库每次是新库，没有一行既有数据要补；
-- 真有的话那些行也是本仓自己的合成 S，不是租户数据。

ALTER TABLE parcel_shipment.commercial_resolution_key_registration
    ADD COLUMN credit_level       text,
    ADD COLUMN credit_charge_type text;

-- 两维与信用依据同进同出（ADR-0044 的「含则必填、不含则必缺」在信用上的那一格，与
-- ..._settlement_paired 同形）。写成一条整体约束而不是两条各管一维，理由同 0008：部分给出的
-- 选择器与「给了维度却没要信用依据」都必须挡在这里。领域侧 ClosureResolutionKey 的最小身份
-- 校验与登记面 validateCredit 是同一判据的另两道镜像。
ALTER TABLE parcel_shipment.commercial_resolution_key_registration
    ADD CONSTRAINT commercial_resolution_key_registration_credit_paired
        CHECK (
            (
                credit_level IS NOT NULL
                AND credit_charge_type IS NOT NULL
                AND 'CREDIT_POLICY' = ANY (required_bases)
            )
            OR (
                credit_level IS NULL
                AND credit_charge_type IS NULL
                AND NOT ('CREDIT_POLICY' = ANY (required_bases))
            )
        );

ALTER TABLE parcel_shipment.commercial_resolution_key_registration
    ADD CONSTRAINT commercial_resolution_key_registration_credit_not_blank
        CHECK (
            (credit_level IS NULL OR btrim(credit_level) <> '')
            AND (credit_charge_type IS NULL OR btrim(credit_charge_type) <> '')
        );
