-- 外部承运轨迹事实版本链线性：一版至多被回指一次（tf-face-algebra/06，照 0014 给有效时间规则立的那一条）。
--
-- 0011 的主键（租户，事实，版本）只防同一版本写两次，supersedes_version 上没有唯一约束。判断口与收编更正都先读
-- 当前版再写回指它的新版本，那次「先读」在读已提交事务里拦不住并发：两方都读到同一个当前版，各自回指它，一条事实
-- 就成了两条当前版；此后任何一方再改判只接在读到的那一支上，另一支永远留作当前版，visibility-exception 那边把旧
-- 一支静默当作有效。版本链的形状因此由库面守：后到的一方在这条部分唯一索引上等先到的一方，后者提交后撞上它，Save
-- 的 ON CONFLICT DO NOTHING 把它折成`已登记`，编排重读当前版再判（按本次新版本键读回只会读空）。
--
-- 存量里已有分叉就拒绝迁移，不替谁挑留哪一支：哪一支是那次判断或更正、另一支该不该接在它后面作再判，只有写下
-- 它们的一方答得出。静默删掉任何一支都是改写判断与源更正的历史，而 0011 的硬句是写入只插不改。
DO $$
DECLARE
    forked text;
BEGIN
    SELECT string_agg(format('%s/%s 的 %s（后继 %s）', tenant_id, fact_ref, supersedes_version, successors),
                      '，' ORDER BY tenant_id, fact_ref, supersedes_version)
      INTO forked
      FROM (
          SELECT tenant_id, fact_ref, supersedes_version,
                 string_agg(version, '、' ORDER BY version) AS successors
            FROM transport_fulfillment.external_carrier_tracking_fact
           WHERE supersedes_version IS NOT NULL
           GROUP BY tenant_id, fact_ref, supersedes_version
          HAVING count(*) > 1
      ) AS forked_predecessors;
    IF forked IS NOT NULL THEN
        RAISE EXCEPTION USING MESSAGE =
            'external_carrier_tracking_fact 存在被回指了不止一次的版本，不建「一版至多被回指一次」索引，也不代删任何一支：'
            || forked
            || '。请逐个前版查明哪一个后继是那次判断或更正，其余各支导出备查后由写入方处置（对链尾再判或再更正），再重跑本迁移。';
    END IF;
END
$$;

-- 一版至多被回指一次：回指前版的行在（租户，事实，前版）上唯一。首次认领不回指前版，不进这道索引。
CREATE UNIQUE INDEX external_carrier_tracking_fact_superseded_once
    ON transport_fulfillment.external_carrier_tracking_fact (tenant_id, fact_ref, supersedes_version)
    WHERE supersedes_version IS NOT NULL;
