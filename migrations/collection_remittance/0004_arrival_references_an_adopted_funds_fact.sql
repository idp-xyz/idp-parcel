-- 运营企业真实到账的证据引用是 settlement-accounting 已采用外部资金事实的引用（ADR-0274、ADR-0330）：登记时按引用读回
-- 它此刻的链头，版本钉在到账上留痕；一条已采用事实对一条指令至多支撑一条真实到账。证据列 evidence_ref 照旧存引用本身，
-- 本迁移只加留痕的版本列与两道库面兜底，不改任何已落行。
--
-- 与领域构造门、编排各一份同一规则（两处同笔改）：
--   - 版本只属于真实到账，且新登记的真实到账必有版本；
--   - 同一（租户，采用事实引用，指令）至多一条真实到账——编排在本上下文的事实锁下判，这条唯一索引兜住绕过锁的写入。
-- 每条已采用事实名下到账金额之和不超过事实此刻的金额要读结算一侧，库上看不见，只在编排里判。
--
-- 按旧口径登记的真实到账（证据引用是任意字符串）没有版本、也不指向任何已采用事实，存量不回填：下面的「新登记必有版本」
-- 以 NOT VALID 加，只管之后的写入。这些存量到账撑不起移位——移位的采用事实门读不到它引用的事实，答依据不在册，要为同一
-- 笔钱按引用另登一条到账。同一（采用事实引用，指令）下旧口径登了不止一条的，唯一索引建不起来，建之前先预检并拒绝
-- 施加、整笔迁移回滚，不替谁挑留哪一条：哪一条是重复登记的，只有登记它们的一方答得出。

ALTER TABLE collection_remittance.collection_fact
    ADD COLUMN funds_fact_version text;

ALTER TABLE collection_remittance.collection_fact
    ADD CONSTRAINT collection_fact_funds_version_only_on_arrival
        CHECK (
            funds_fact_version IS NULL
            OR (source_layer = 'OPERATOR_BANK_CREDIT' AND btrim(funds_fact_version) <> '')
        );

ALTER TABLE collection_remittance.collection_fact
    ADD CONSTRAINT collection_fact_arrival_pins_funds_version
        CHECK (source_layer <> 'OPERATOR_BANK_CREDIT' OR funds_fact_version IS NOT NULL)
        NOT VALID;

DO $$
DECLARE
    duplicated text;
BEGIN
    SELECT string_agg(format('%s/%s/%s（到账 %s）', tenant_id, evidence_ref, instruction_ref, facts),
                      '，' ORDER BY tenant_id, evidence_ref, instruction_ref)
      INTO duplicated
      FROM (
          SELECT tenant_id, evidence_ref, instruction_ref,
                 string_agg(fact_ref, '、' ORDER BY fact_ref) AS facts
            FROM collection_remittance.collection_fact
           WHERE source_layer = 'OPERATOR_BANK_CREDIT'
           GROUP BY tenant_id, evidence_ref, instruction_ref
          HAVING count(*) > 1
      ) AS doubled_arrivals;
    IF duplicated IS NOT NULL THEN
        RAISE EXCEPTION USING MESSAGE =
            'collection_fact 存在同一证据引用、同一指令下登记了不止一条的运营企业真实到账，不建「一条已采用事实对一条指令至多一条到账」索引，也不代删任何一条：'
            || duplicated
            || '。请逐条查明哪一条是重复登记的，其余各条导出备查后由写入方处置，再重跑本迁移。';
    END IF;
END
$$;

CREATE UNIQUE INDEX collection_fact_one_arrival_per_funds_and_instruction
    ON collection_remittance.collection_fact (tenant_id, evidence_ref, instruction_ref)
    WHERE source_layer = 'OPERATOR_BANK_CREDIT';

-- 按已采用事实读它名下的全部到账（跨指令）：登记与移位判合计要读这一组。
CREATE INDEX collection_fact_arrivals_by_funds
    ON collection_remittance.collection_fact (tenant_id, evidence_ref)
    WHERE source_layer = 'OPERATOR_BANK_CREDIT';
