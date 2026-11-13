-- 披露决定记下是谁发起、谁确认的，并让同一条异常对同一个客户只有一份按规则形成的决定、每份待授权只被确认一次
-- （UC-VE-006 步 3–5）。
--
-- decided_by：作出这一行决定的操作者——按规则形成的那一行是发起判断的人，授权确认形成的那一行是确认人。可空：披露
-- 判断此前没有任何生产入口，既有行没有人；不经运营口形成的决定也没有。
-- authorizes_decided_at：只在授权确认形成的那一行上非空，指回它所确认的那份待授权决定的决定时间。
--
-- 两条唯一索引各守一个并发窗口。这张表的主键含决定时间，所以两笔同时到达的写入各带各的时刻、互不相撞：
-- - 同一（发作期，客户）只有一行按规则形成的决定。没有它，两个人同时对同一条信号发起判断会各落一行，谁是「当前」
--   只看谁的时钟晚。
-- - 同一份待授权决定只被确认一次。没有它，两个人同时确认会落两份披露决定，通知按披露决定去重，同一条异常就会对
--   同一个客户发两次。

ALTER TABLE visibility_exception.disclosure_decision
    ADD COLUMN decided_by text,
    ADD COLUMN authorizes_decided_at timestamptz;

ALTER TABLE visibility_exception.disclosure_decision
    ADD CONSTRAINT disclosure_decision_authorization_shape
        CHECK (
            (decided_by IS NULL OR btrim(decided_by) <> '')
            AND (
                authorizes_decided_at IS NULL
                OR (conclusion = 'DISCLOSE' AND decided_by IS NOT NULL AND authorizes_decided_at < decided_at)
            )
        );

CREATE UNIQUE INDEX disclosure_decision_one_rule_formed_per_episode_customer
    ON visibility_exception.disclosure_decision (tenant_id, episode_id, customer_ref)
    WHERE authorizes_decided_at IS NULL;

CREATE UNIQUE INDEX disclosure_decision_one_authorization_per_awaiting
    ON visibility_exception.disclosure_decision (tenant_id, episode_id, customer_ref, authorizes_decided_at)
    WHERE authorizes_decided_at IS NOT NULL;
