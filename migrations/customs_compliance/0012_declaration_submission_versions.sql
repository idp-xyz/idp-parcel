-- 原案内更正/补充：同一逻辑申报目标容纳多个不可覆盖提交版本。
--
-- CONTEXT 硬句 169「监管规则允许的更正或补充在原案件内形成新的正式申报资料和提交
-- 版本，原提交及其结果永久保留」与生命周期「提交后允许更正或补充 → 在原案件内形成
-- 新的资料快照与提交版本；原版本不可修改」——0002 的一键一行装不下第二版，键从
-- （租户+单元+程序）扩为逐版本一行；「当前版」由 is_current 部分唯一索引承担
-- （先例：transport_fulfillment 交付登记的 is_current 部分唯一索引、更正翻旧插新）。
--
-- 0002 注释「行内无任何可回写列」在内容列上照旧成立：is_current 是当前指针不属版本
-- 内容，翻转它不是改写版本；corrected_from 指名被更正的前一版——替代关系由源上下文
-- 随更正一并给出（VE CONTEXT「来源事实替代关系」），落在被更正一侧的下一版行上，
-- 前版行一列不动。

ALTER TABLE customs_compliance.declaration_submission
    ADD COLUMN is_current boolean NOT NULL DEFAULT true;
-- 既有行都是各自键下的唯一版本，默认 true 即当前版，无需回填；写入方此后显式给值，
-- 留着默认会让漏写的一行静默变成当前版。
ALTER TABLE customs_compliance.declaration_submission
    ALTER COLUMN is_current DROP DEFAULT;

ALTER TABLE customs_compliance.declaration_submission
    ADD COLUMN corrected_from text;
ALTER TABLE customs_compliance.declaration_submission
    ADD CONSTRAINT declaration_submission_corrected_from_not_blank
        CHECK (corrected_from IS NULL OR btrim(corrected_from) <> '');
-- 首版无前身、更正版必有前身——但「必有」只能由写入方与编排把守：SQL 层分不出
-- 「首版」与「漏写前身的更正版」，这里只拦空串冒充。

ALTER TABLE customs_compliance.declaration_submission
    DROP CONSTRAINT declaration_submission_pkey;
ALTER TABLE customs_compliance.declaration_submission
    ADD CONSTRAINT declaration_submission_pkey
        PRIMARY KEY (tenant_id, unit_id, procedure_ref, version_id);

-- 每个逻辑申报目标恰一个当前版；并发更正由它裁决（第二个翻转者 UPDATE 到零行）。
CREATE UNIQUE INDEX declaration_submission_current_unique
    ON customs_compliance.declaration_submission (tenant_id, unit_id, procedure_ref)
    WHERE is_current;
