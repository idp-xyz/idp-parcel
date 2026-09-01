-- 普通客户费用调整单列追加式登记册（SA CONTEXT「费用形成与证据」：确认后出现新事实、
-- 源事实更正或规则适用性变化时形成新版本或调整明细，不覆盖历史结果；「费用明细」生命
-- 周期：由对应唯一创建用例追加调整明细，原确认费用仍保留，**对账纳入和资金核销不能
-- 代替金额调整**）。形状照 ADR-0087 决定二。
--
-- 不走版本链：供应商侧的计价纠错「是一个重述全额的新成本版本，不是带借/贷方向的差额
-- 调整」，而客户侧的调整带借贷方向，照搬 supplier_expected_cost 的版本链等于把两种不同
-- 的东西塞进同一套表达。
--
-- 这张册立起来之后，customer_statement.adjustment_lines 退回它本来的角色——**纳入关系**，
-- 与 subsequent_inclusion 一致，不再是调整的唯一存身处。在此之前库上的因果是反的：一笔
-- 调整只有先进了某张对账单才存在，而 CONTEXT 那句禁的正是这个顺序。
--
-- **唯一创建用例这道门落在种类封闭集上。** CONTEXT「调整类型与唯一所有权」表把计价纠错
-- 与商业让利判给 UC-SA-002，赔付与索赔退款归 UC-SA-007、供应商账单贷项归 UC-SA-004、
-- 代垫回收调整归 UC-SA-001，各有自己的册。本册的 kind 只开 UC-SA-002 拥有的两格，别的
-- 种类在这里连可表达的取值都没有——写一个通用 kind text 才是那条「谁都能造调整」的路。
--
-- 只追加不改写：主键（租户+调整）一次成形，写口无 UPDATE 分支，重放答原件。「重复触发
-- 返回原结果，语义或范围不同则形成独立关联调整」（CONTEXT 同节），因此不设内容摘要冲突
-- 格——语义不同的那一笔本就该是另一个调整标识。
--
-- charge_id 以标识引用不设外键，照本模块既有做法（advance_recovery 引 assessment_id、
-- recovery_adjustment 引 recovery_id 均同）。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：可空列使用前先
-- IS NULL / IS NOT NULL。

CREATE TABLE settlement_accounting.charge_adjustment (
    tenant_id           text        NOT NULL,
    adjustment_id       text        NOT NULL,

    charge_id           text        NOT NULL,
    kind                text        NOT NULL,
    direction           text        NOT NULL,
    evaluation_ref      text,
    authorization_ref   text,
    original_currency   text        NOT NULL,
    original_minor      bigint      NOT NULL,
    settlement_currency text        NOT NULL,
    settlement_minor    bigint      NOT NULL,
    conversion_ref      text,
    formed_at           timestamptz NOT NULL,
    recorded_at         timestamptz NOT NULL,
    inserted_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT charge_adjustment_pkey
        PRIMARY KEY (tenant_id, adjustment_id),

    CONSTRAINT charge_adjustment_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(adjustment_id) <> ''
            AND btrim(charge_id) <> ''
            AND btrim(kind) <> ''
            AND btrim(direction) <> ''
            AND btrim(original_currency) <> ''
            AND btrim(settlement_currency) <> ''
        ),

    CONSTRAINT charge_adjustment_amounts_positive
        CHECK (original_minor > 0 AND settlement_minor > 0),

    -- 种类封闭在 UC-SA-002 拥有的两格：这既是「拒绝无语义调整」（AT-SA-056），也是唯一
    -- 创建用例那道门在册上的形式。
    CONSTRAINT charge_adjustment_kind_closed
        CHECK (kind IN ('PRICING_CORRECTION', 'COMMERCIAL_CONCESSION')),

    CONSTRAINT charge_adjustment_direction_closed
        CHECK (direction IN ('DEBIT', 'CREDIT')),

    -- 依据按种类各占一格且有此无彼：纠错必挂新评价、让利必挂商业授权。Kind 之外得有第二
    -- 个维分辨那个字符串指的是什么，填错格与两格齐填同样拒收（与 FormChargeAdjustment
    -- 一字不差）。
    CONSTRAINT charge_adjustment_basis_slotted_by_kind
        CHECK (
            (kind = 'PRICING_CORRECTION'
             AND evaluation_ref IS NOT NULL AND btrim(evaluation_ref) <> ''
             AND authorization_ref IS NULL)
            OR (kind = 'COMMERCIAL_CONCESSION'
                AND authorization_ref IS NOT NULL AND btrim(authorization_ref) <> ''
                AND evaluation_ref IS NULL)
        ),

    -- 跨币种必带换算依据、同币种两额必须相等：两道门与 customer_charge 同款，调整与费用
    -- 本体同构地受三件组约束——同一条链上费用能表达的跨币种，它的调整必须也能表达。
    CONSTRAINT charge_adjustment_conversion_present
        CHECK (
            original_currency = settlement_currency
            OR (conversion_ref IS NOT NULL AND btrim(conversion_ref) <> '')
        ),

    CONSTRAINT charge_adjustment_same_currency_amounts_agree
        CHECK (
            original_currency <> settlement_currency
            OR original_minor = settlement_minor
        )
);

-- 按被调整费用反查：调整册的主要读法是「这笔费用被调整过哪些」，与 subsequent_inclusion
-- 从纳入侧的读法互补。
CREATE INDEX charge_adjustment_by_charge
    ON settlement_accounting.charge_adjustment (tenant_id, charge_id, formed_at);
