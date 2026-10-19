-- 运输收费发生项记下它凭什么满足了采用协议的发生条件（票 demo-chain-gaps/27，ADR-0207 决定三）。
--
-- CONTEXT 只许满足采用协议所需事实条件的明确范围形成发生项。协议正文还没有声明发生条件的文法，首版由登记者断言
-- 条件已满足：断言是登记者的原话，与发生项同一行落库，日后复核的人才查得到「这一项当时是谁凭什么判的」。依据一格
-- 是封闭集合，首版只有 REGISTRANT_ASSERTED；协议能声明条件之后多出的那一格随 ADR-0209 落地时放开。
--
-- 依据一格不可空，也不给缺省值。原话一格可空，由下面的 CHECK 按依据逐格守：断言那一格必带原话，将来按协议声明
-- 判的那一格没有原话可带。0007 以来本册没有任何生产写入口（ADR-0098 后果：机制齐备而生产触发链未接），表里
-- 应当没有行；有行就拒绝迁移——那几行没有人断言过，替它们补一个「已断言」就是替不在场的人作答。
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM transport_fulfillment.transport_charge_occurrence) THEN
        RAISE EXCEPTION USING MESSAGE =
            'transport_charge_occurrence 已有行，不替它们补发生条件的依据：请导出这些行备查，由登记它们的一方'
            || '按发生条件重新登记后清空本表（连同成员表），再重跑本迁移。';
    END IF;
END
$$;

ALTER TABLE transport_fulfillment.transport_charge_occurrence
    ADD COLUMN condition_basis     text NOT NULL,
    ADD COLUMN condition_assertion text;

ALTER TABLE transport_fulfillment.transport_charge_occurrence
    ADD CONSTRAINT transport_charge_occurrence_condition_basis_closed
        CHECK (condition_basis IN ('REGISTRANT_ASSERTED'));

-- 断言那一格必带原话：只记「断言过」，复核的人就不知道当时断言的是什么。
ALTER TABLE transport_fulfillment.transport_charge_occurrence
    ADD CONSTRAINT transport_charge_occurrence_condition_assertion_not_blank
        CHECK (condition_basis <> 'REGISTRANT_ASSERTED'
               OR (condition_assertion IS NOT NULL AND btrim(condition_assertion) <> ''));
