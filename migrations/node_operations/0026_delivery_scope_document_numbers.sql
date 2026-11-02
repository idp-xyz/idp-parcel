-- 送站预报与送站交接单的单据号（ADR-0355 决定二、三、七、八；票 document-numbering/08）。单据号是开立时按单据编号规则分配、给人看的
-- 业务编号，不是标识：预报标识与交接单标识照旧做主键与重放身份，本迁移不动它们。
--
-- 每本册加两格：单据号，与没有单据号时的原因。两格都空即「上线前开立」——本迁移之前落下的旧行就是这样，不回填、不补号（决定三
-- 「本决定生效前已开立的单据不补号」）；开立时单据编号时区未配置而无号的，记那一个原因（决定七）。原因只有这一种可写：上线前开立是
-- 两格都空读出来的，开立路径写不出它，库面也不收它。两格不能同在。
--
-- （租户，单据号）唯一，是决定八「由各单据所在表的唯一约束担保」的那一道最后保险：取号编排先在同一事务里查本表跳过已用的号，这条
-- 约束兜的是绕过编排的写入。NULL 互不相等，旧行与无号行不相撞。
--
-- 单据号是开立时与单据同一行写下的，此后不改，表仍只增：开立路径先锁住重放身份、确定要落单才取号，号随那一行 INSERT，没有 UPDATE。
--
-- 只加可空列、检查约束与唯一约束，能在有数据的库上施加：旧行两格都空，三条检查都过，唯一约束不因 NULL 相撞。不含任何实例默认值。

ALTER TABLE node_operations.delivery_pre_advice
    ADD COLUMN document_number         text,
    ADD COLUMN document_number_absence text,

    ADD CONSTRAINT delivery_pre_advice_document_number_not_blank
        CHECK (document_number IS NULL OR btrim(document_number) <> ''),

    ADD CONSTRAINT delivery_pre_advice_document_number_absence_closed
        CHECK (document_number_absence IS NULL OR document_number_absence = 'NUMBERING_TIME_ZONE_NOT_CONFIGURED'),

    ADD CONSTRAINT delivery_pre_advice_document_number_or_absence
        CHECK (document_number IS NULL OR document_number_absence IS NULL),

    ADD CONSTRAINT delivery_pre_advice_document_number_unique
        UNIQUE (tenant_id, document_number);

ALTER TABLE node_operations.delivery_handover_note
    ADD COLUMN document_number         text,
    ADD COLUMN document_number_absence text,

    ADD CONSTRAINT delivery_handover_note_document_number_not_blank
        CHECK (document_number IS NULL OR btrim(document_number) <> ''),

    ADD CONSTRAINT delivery_handover_note_document_number_absence_closed
        CHECK (document_number_absence IS NULL OR document_number_absence = 'NUMBERING_TIME_ZONE_NOT_CONFIGURED'),

    ADD CONSTRAINT delivery_handover_note_document_number_or_absence
        CHECK (document_number IS NULL OR document_number_absence IS NULL),

    ADD CONSTRAINT delivery_handover_note_document_number_unique
        UNIQUE (tenant_id, document_number);
