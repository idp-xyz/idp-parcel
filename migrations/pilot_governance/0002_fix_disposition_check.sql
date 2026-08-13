-- 修 0001 的 stage_review_disposition_shape：SQL 三值逻辑漏洞——disposition 可空，
-- No-Go 支的 `disposition IN (...)` 在 NULL 上得 NULL，`FALSE OR NULL` 仍是 NULL，
-- CHECK 只拦 FALSE 不拦 NULL，于是「No-Go 不带处理方式」这种被 0001 注释点名要拦的
-- 行溜得进来。补显式 IS NOT NULL。已施加的迁移不可改写（校验和把关），故新文件重建
-- 约束；表此刻无真实数据，直接换无需清洗。
--
-- 同形隐患已全查：本仓其余带 NULL 列的 CHECK 或走 IS NULL 显式分支、或经 coalesce、
-- 或列本身 NOT NULL，仅此一处失守。

ALTER TABLE pilot_governance.stage_review
    DROP CONSTRAINT stage_review_disposition_shape,
    ADD CONSTRAINT stage_review_disposition_shape
        CHECK (
            (verdict = 'GO' AND disposition IS NULL)
            OR (verdict = 'NO_GO' AND disposition IS NOT NULL AND disposition IN
                ('KEEP_CURRENT_SCOPE', 'SUSPEND_NEW_ADMISSION', 'FIX_AND_REASSESS', 'OBJECT_LEVEL_TAKEOVER'))
        );
