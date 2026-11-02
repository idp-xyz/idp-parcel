-- 单据号（ADR-0355；票 document-numbering/05）：对账单的发布意图，费用争议与审核应付的单据号。
--
-- 对账单：单号在发布事务里按编号规则取，仍是（租户，单号）寻址键、仍非空；发布幂等从按单号改为按发布意图——同一结算
-- 账户、账期与截止版本（截单命令里的草稿版本）只发布一张（决定九）。截止版本以前不落库，这里补一列 draft_version，意图
-- 唯一只管记了版本的行。本决定之前截出的单要按意图同样查得到：自动截单截出的单都记在截止记录 statement_cycle_cut 上（同一
-- 账户、同一单号、账期是 CYCLE/<截止日>），版本固定 v1，按截止记录认、补上 v1——不按单号形状认：手工发布口的单号由人填，
-- 填成 STMT/<账户>/<日> 的样子也只是手工单。手工发布的单当时的版本无从得知，留空；按意图查重时同账户同账期有这种未作废的
-- 旧单，内容相同答原单，内容不同答发布意图冲突（见截单编排 findPublished）。已发布单的单号不动。
--
-- 费用争议与审核应付：各加可空的单据号与无号原因两列。有号只填前一列；开立时单据编号时区未配置只填后一列；两列都空是
-- 本决定之前开立的旧行，读作上线前开立——旧行不回填（决定三）。（租户，单据号）唯一，作取号判撞之后的最后一道保险
-- （决定八）。
--
-- 全部 CHECK 过 SQL 三值逻辑：可空列使用前先 IS NULL / IS NOT NULL。施加在有数据的库上：只加列、补自动截单的版本、建索引，
-- 不改任何已有内容列。

ALTER TABLE settlement_accounting.customer_statement
    ADD COLUMN draft_version text,
    ADD CONSTRAINT customer_statement_draft_version_not_blank
        CHECK (draft_version IS NULL OR btrim(draft_version) <> '');

UPDATE settlement_accounting.customer_statement AS statement
   SET draft_version = 'v1'
  FROM settlement_accounting.statement_cycle_cut AS cut
 WHERE statement.draft_version IS NULL
   AND cut.tenant_id = statement.tenant_id
   AND cut.account_id = statement.account_id
   AND cut.statement_number = statement.statement_number
   AND statement.period_ref = 'CYCLE/' || to_char(cut.closing_date, 'YYYY-MM-DD');

CREATE UNIQUE INDEX customer_statement_publication_intent_key
    ON settlement_accounting.customer_statement (tenant_id, account_id, period_ref, draft_version)
 WHERE draft_version IS NOT NULL;

ALTER TABLE settlement_accounting.statement_dispute
    ADD COLUMN document_number text,
    ADD COLUMN document_number_absence text,
    ADD CONSTRAINT statement_dispute_document_number_not_blank
        CHECK (document_number IS NULL OR btrim(document_number) <> ''),
    -- 有号就没有无号原因；无号原因只有一个能落库的词（上线前开立由两列都空表达）。
    ADD CONSTRAINT statement_dispute_document_number_absence_shape
        CHECK (document_number_absence IS NULL
               OR (document_number IS NULL AND document_number_absence = 'NUMBERING_TIME_ZONE_NOT_CONFIGURED'));

CREATE UNIQUE INDEX statement_dispute_document_number_key
    ON settlement_accounting.statement_dispute (tenant_id, document_number)
 WHERE document_number IS NOT NULL;

ALTER TABLE settlement_accounting.audited_payable
    ADD COLUMN document_number text,
    ADD COLUMN document_number_absence text,
    ADD CONSTRAINT audited_payable_document_number_not_blank
        CHECK (document_number IS NULL OR btrim(document_number) <> ''),
    ADD CONSTRAINT audited_payable_document_number_absence_shape
        CHECK (document_number_absence IS NULL
               OR (document_number IS NULL AND document_number_absence = 'NUMBERING_TIME_ZONE_NOT_CONFIGURED'));

CREATE UNIQUE INDEX audited_payable_document_number_key
    ON settlement_accounting.audited_payable (tenant_id, document_number)
 WHERE document_number IS NOT NULL;
