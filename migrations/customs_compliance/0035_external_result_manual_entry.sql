-- 海关外部结果可以由人照原件录入：记下录入人与所凭原件（dox/decisions.md「产品形态」）。
--
-- 没有对接海关或报关行系统的租户，海关的受理、放行、核定税费由关务人员照回执录入。两格只在人工录入的行上非空：
-- entered_by 是录入人（操作者渠道的 OPERATOR/<发行方>#<sub>），original_ref 是所凭原件的编号或存档引用。接口来的行
-- 两格都空。同在或同缺——有录入人没有原件，等于凭记忆造了一条监管事实。它们不进内容指纹。
--
-- 约束里两格的 IS NOT NULL 要明写：只写 btrim(x) <> '' 时，一格为空会让整条算成 NULL，而 CHECK 把 NULL 当通过。

ALTER TABLE customs_compliance.external_result
    ADD COLUMN entered_by text,
    ADD COLUMN original_ref text;

ALTER TABLE customs_compliance.external_result
    ADD CONSTRAINT external_result_manual_entry_paired
        CHECK (
            (entered_by IS NULL AND original_ref IS NULL)
            OR (entered_by IS NOT NULL AND original_ref IS NOT NULL AND btrim(entered_by) <> '' AND btrim(original_ref) <> '')
        );
