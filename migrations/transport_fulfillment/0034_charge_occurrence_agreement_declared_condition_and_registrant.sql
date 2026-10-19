-- 运输收费发生项按采用协议声明的收费发生条件判（ADR-0209；票 demo-chain-gaps/29）：发生条件的依据多出「按协议声明
-- 判」一格，每一项记下登记者，并为「这个范围是不是已经计过」立索引。
--
-- 一、依据的封闭集合加 AGREEMENT_DECLARED。断言期（REGISTRANT_ASSERTED）的行照旧可读、不改写：那几项当时就是由人
--     判的，替它们补一个「按协议判过」就是改写历史。按协议判的那一格没有断言人与原话——判断出自协议，协议已记在
--     agreement_ref 上——CHECK 守住它不带这两格。
-- 二、registered_by 是做出这一次登记决定的操作者（「发行方#sub」），每一项都有。断言期只在断言那一格记人，换成按
--     协议判之后不另记，审计就找不到是谁报的服务方、协议版本与成员；0032 的断言人一列含义不改（票 demo-chain-gaps/27
--     评审修订的代决 2：「另加登记者一列，不改这一列的含义」）。已有的断言行由断言人回填——断言人就是那一次的登记者
--     ——再立 NOT NULL；断言那一格的两列此后必须相等。
-- 三、登记前按（租户、原因、主要业务范围）问覆盖（ADR-0209 决定四：按段、按尝试、按订舱计的一项盖住整个范围）。
--     0032 的索引从事实依据起头，答不了按范围的这一问。
ALTER TABLE transport_fulfillment.transport_charge_occurrence
    DROP CONSTRAINT transport_charge_occurrence_condition_basis_closed;

ALTER TABLE transport_fulfillment.transport_charge_occurrence
    ADD CONSTRAINT transport_charge_occurrence_condition_basis_closed
        CHECK (condition_basis IN ('REGISTRANT_ASSERTED', 'AGREEMENT_DECLARED'));

ALTER TABLE transport_fulfillment.transport_charge_occurrence
    ADD CONSTRAINT transport_charge_occurrence_declared_condition_carries_no_assertion
        CHECK (condition_basis <> 'AGREEMENT_DECLARED'
               OR (condition_asserted_by IS NULL AND condition_assertion IS NULL));

ALTER TABLE transport_fulfillment.transport_charge_occurrence
    ADD COLUMN registered_by text;

UPDATE transport_fulfillment.transport_charge_occurrence
   SET registered_by = condition_asserted_by
 WHERE condition_basis = 'REGISTRANT_ASSERTED';

ALTER TABLE transport_fulfillment.transport_charge_occurrence
    ALTER COLUMN registered_by SET NOT NULL;

ALTER TABLE transport_fulfillment.transport_charge_occurrence
    ADD CONSTRAINT transport_charge_occurrence_registered_by_not_blank
        CHECK (btrim(registered_by) <> '');

ALTER TABLE transport_fulfillment.transport_charge_occurrence
    ADD CONSTRAINT transport_charge_occurrence_asserter_is_the_registrant
        CHECK (condition_basis <> 'REGISTRANT_ASSERTED' OR condition_asserted_by = registered_by);

CREATE INDEX transport_charge_occurrence_scope_idx
    ON transport_fulfillment.transport_charge_occurrence (tenant_id, reason, scope_ref);
