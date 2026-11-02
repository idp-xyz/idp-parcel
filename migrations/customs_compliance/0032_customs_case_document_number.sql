-- 关务案件的单据号（ADR-0355 决定二、三、七、八；CC CONTEXT「关务案件」生命周期）。
--
-- 单据号是案件上另加的一项，不是案件标识：case_id 照旧做主键之外的反查键、幂等与跨上下文引用，范围键照旧做建案幂等，
-- 单据号不参与判重。号在建案事务里取、随行落下，之后没有任何改写它的路径——案件是责任容器，适配器没有 UPDATE 语句。
--
-- 两列至多一列非空：
--   document_number          建案时按 CUSTOMS_CASE 的规则取到的号；
--   document_number_absence  建案时没取到号的情形。只收 NUMBERING_TIME_ZONE_NOT_CONFIGURED（关务案件的开立方没有
--                            业务时区，租户又没登单据编号时区）；
--   两列皆空                 本迁移之前建立的旧行，读成「上线前开立，无单据号」。迁移不回填、不补号（决定三）。
-- 「上线前开立」不进 absence 的取值集：它只是旧行的读法，新行写成它就是在冒充旧行。
--
-- （租户，单据号）唯一：案件表只放一类单据，类型代码这一维不必进键（决定八）。取号编排先在同一事务里查这张表跳过
-- 已用的号，约束是最后一道保险。NULL 互不相等，旧行与无号的行都不受约束。
--
-- 存量安全：只加两个可空列、两条对 NULL 恒成立的 CHECK 与一条唯一约束，旧行一格不动，直接校验，不用 NOT VALID。

ALTER TABLE customs_compliance.customs_case
    ADD COLUMN document_number text,
    ADD COLUMN document_number_absence text;

ALTER TABLE customs_compliance.customs_case
    ADD CONSTRAINT customs_case_document_number_shape
        CHECK (document_number IS NULL OR (
            document_number ~ '\S'
            AND document_number !~ '^\s'
            AND document_number !~ '\s$'
            AND document_number !~ '[[:cntrl:]]'
        )),
    ADD CONSTRAINT customs_case_document_number_absence_closed
        CHECK (document_number_absence IS NULL OR (
            document_number_absence = 'NUMBERING_TIME_ZONE_NOT_CONFIGURED'
            AND document_number IS NULL
        )),
    ADD CONSTRAINT customs_case_document_number_unique
        UNIQUE (tenant_id, document_number);
