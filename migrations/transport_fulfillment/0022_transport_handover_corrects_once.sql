-- 交接更正链线性：一版最多被更正一次（tf-face-algebra/07，照 0015 给揽收更正立的那一条）。
--
-- 0005 只有主键（租户，对象，范围，版本），corrects_version 上没有唯一约束，同一前版可以被更正成两个新版本；范围汇总
-- 按版本链只折掉被替代的那一代，两个后继都留下，一个对象就被数了两次。更正口先按前版读后继再写是第一道；两份更正
-- 并发到达时两边都读得到「前版还没有后继」，只有这条部分唯一索引拦得住：后到的一方撞上它，Save 的 ON CONFLICT DO
-- NOTHING 把它折成`已登记`，编排按前版读回先到的那一份作答（按本次新版本键读回只会读空）。
--
-- 存量里已有分叉就拒绝迁移，不替谁挑留哪一枝：哪一枝是那次更正、另一枝该不该接在它后面作再更正，只有写下它们的
-- 一方答得出。静默删掉任何一枝都是改写交接判断的历史，而 0005 的硬句是更正是新版本新行、原行不删。
DO $$
DECLARE
    forked text;
BEGIN
    SELECT string_agg(format('%s/%s/%s 的 %s（后继 %s）', tenant_id, object_ref, scope_ref, corrects_version, successors),
                      '，' ORDER BY tenant_id, object_ref, scope_ref, corrects_version)
      INTO forked
      FROM (
          SELECT tenant_id, object_ref, scope_ref, corrects_version,
                 string_agg(handover_version, '、' ORDER BY handover_version) AS successors
            FROM transport_fulfillment.transport_handover
           WHERE corrects_version IS NOT NULL
           GROUP BY tenant_id, object_ref, scope_ref, corrects_version
          HAVING count(*) > 1
      ) AS forked_predecessors;
    IF forked IS NOT NULL THEN
        RAISE EXCEPTION USING MESSAGE =
            'transport_handover 存在被更正了不止一次的前版，不建「一版最多被更正一次」索引，也不代删任何一枝：' || forked
            || '。请逐个前版查明哪一个后继是那次更正，其余各枝导出备查后由写入方处置（对链尾再提更正），再重跑本迁移。';
    END IF;
END
$$;

-- 一版最多被更正一次：更正行（回指前版）在（租户，对象，范围，前版）上唯一。首登不回指前版，不进这道索引。
CREATE UNIQUE INDEX transport_handover_corrects_once
    ON transport_fulfillment.transport_handover (tenant_id, object_ref, scope_ref, corrects_version)
    WHERE corrects_version IS NOT NULL;
