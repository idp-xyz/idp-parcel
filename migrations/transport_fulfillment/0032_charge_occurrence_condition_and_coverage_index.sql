-- 运输收费发生项记下它凭什么满足了采用协议的发生条件（票 demo-chain-gaps/27，ADR-0207 决定三），并为登记前的
-- 成员交集一问立索引（决定四，见文末）。
--
-- CONTEXT 只许满足采用协议所需事实条件的明确范围形成发生项。协议正文还没有声明发生条件的文法，首版由登记者断言
-- 条件已满足：断言人（操作者信封的主体，「发行方#sub」）与断言原话同发生项一行落库，日后复核的人才查得到「这一项
-- 当时是谁凭什么判的」。依据一格是封闭集合，首版只有 REGISTRANT_ASSERTED；协议能声明条件之后多出的那一格随
-- ADR-0209 落地时放开。
--
-- 依据一格不可空，也不给缺省值。断言人与原话两格可空，由下面的 CHECK 按依据逐格守：断言那一格两件必带，将来按协议
-- 声明判的那一格没有人断言、也没有原话可带。0007 以来本册没有任何生产写入口（ADR-0098 后果：机制齐备而生产触发链
-- 未接），表里应当没有行；有行就拒绝迁移——那几行没有人断言过，替它们补一个「已断言」就是替不在场的人作答。
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
    ADD COLUMN condition_basis       text NOT NULL,
    ADD COLUMN condition_asserted_by text,
    ADD COLUMN condition_assertion   text;

ALTER TABLE transport_fulfillment.transport_charge_occurrence
    ADD CONSTRAINT transport_charge_occurrence_condition_basis_closed
        CHECK (condition_basis IN ('REGISTRANT_ASSERTED'));

-- 断言那一格必带断言人与原话：只记「断言过」，复核的人就不知道当时是谁断言的、断言的是什么。
ALTER TABLE transport_fulfillment.transport_charge_occurrence
    ADD CONSTRAINT transport_charge_occurrence_condition_assertion_not_blank
        CHECK (condition_basis <> 'REGISTRANT_ASSERTED'
               OR (condition_asserted_by IS NOT NULL AND btrim(condition_asserted_by) <> ''
                   AND condition_assertion IS NOT NULL AND btrim(condition_assertion) <> ''));

-- 登记前要问「这些成员在同一件事实上是否已被另一项计过」（ADR-0207 决定四）：按（租户、原因、事实依据）取一组，
-- 再逐项看成员。主键从发生项身份起头，答不了这一问；没有这条索引，每次登记都扫全册。
CREATE INDEX transport_charge_occurrence_fact_basis_idx
    ON transport_fulfillment.transport_charge_occurrence (tenant_id, reason, fact_basis);
