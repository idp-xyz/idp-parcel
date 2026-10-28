-- 关闭依据项补「冲突」（票 docs-consistency/code/03；CC CONTEXT「关闭依据项」已定四值）。
--
-- 冲突项阻止关闭，必须带冲突依据 conflicts_with：相互矛盾的事实引用，至少两份互不相同；其余三值不带。
-- 已登记的行都是旧三值，conflicts_with 为 NULL，与新 CHECK 一致，无需回填。

ALTER TABLE customs_compliance.closure_obligation_item
    ADD COLUMN conflicts_with text[];

ALTER TABLE customs_compliance.closure_obligation_item
    DROP CONSTRAINT closure_obligation_item_state_closed;

ALTER TABLE customs_compliance.closure_obligation_item
    ADD CONSTRAINT closure_obligation_item_state_closed
        CHECK (item_state IN ('CONCLUDED', 'HANDED_OVER', 'UNRESOLVED', 'CONFLICT'));

-- CHECK 不许含子查询，冲突依据的形状判断因此放进不可变函数：至少两份互不相同的非空引用，不含 NULL。
CREATE FUNCTION customs_compliance.conflict_basis_valid(refs text[])
RETURNS boolean
LANGUAGE sql IMMUTABLE
AS $$
    SELECT refs IS NOT NULL
       AND array_position(refs, NULL) IS NULL
       AND NOT EXISTS (SELECT 1 FROM unnest(refs) AS ref WHERE btrim(ref) = '')
       AND (SELECT count(DISTINCT ref) FROM unnest(refs) AS ref) >= 2
$$;

-- 带 NULL 的 CHECK 走 IS NULL 显式分支：非冲突项该列必须是 NULL（空数组也不行）。
ALTER TABLE customs_compliance.closure_obligation_item
    ADD CONSTRAINT closure_obligation_item_conflict_basis
        CHECK (
            CASE
                WHEN item_state = 'CONFLICT' THEN customs_compliance.conflict_basis_valid(conflicts_with)
                ELSE conflicts_with IS NULL
            END
        );
