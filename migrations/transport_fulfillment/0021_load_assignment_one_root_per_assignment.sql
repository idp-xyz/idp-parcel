-- 装载分配一分配一个首版（tf-face-algebra/02）。
--
-- 0009 的主键（租户，分配，版本）只防同一版本写两次，不防同一分配另起一个不回指任何前版的版本——那样「一次分配」
-- 在库里就成了几条并列的链，而 0009 与端口头注说的都是一条版本链。登记口先按分配读链头再写是第一道；两份不同版本号
-- 的首版并发到达时，两边都读得到空链头，只有这条部分唯一索引拦得住：后到的一方撞上它，Save 的 ON CONFLICT DO NOTHING
-- 把它折成`已登记`，编排按分配读回链头作答（按本次键读回只会读空）。
--
-- 存量里已有并列首版就拒绝迁移，不替谁挑留哪一根：哪一根是那次分配、另几根是该并成变化还是本就是别的分配，只有
-- 写下它们的一方答得出。静默删掉任何一根都是改写分配历史，而 0009 的硬句是只插不改。
DO $$
DECLARE
    forked text;
BEGIN
    SELECT string_agg(format('%s/%s（%s 个首版）', tenant_id, assignment_ref, roots), '，' ORDER BY tenant_id, assignment_ref)
      INTO forked
      FROM (
          SELECT tenant_id, assignment_ref, count(*) AS roots
            FROM transport_fulfillment.load_assignment
           WHERE corrects_version IS NULL
           GROUP BY tenant_id, assignment_ref
          HAVING count(*) > 1
      ) AS parallel_roots;
    IF forked IS NOT NULL THEN
        RAISE EXCEPTION USING MESSAGE =
            'load_assignment 存在同一分配下的并列首版，不建「一分配一个首版」索引，也不代删任何一根：' || forked
            || '。请逐个分配查明哪一根是那次分配的首版，其余各根导出备查后由写入方处置（另起分配引用重登，或经变化入口'
            || '形成回指前版的新版本），再重跑本迁移。';
    END IF;
END
$$;

-- 一分配一个首版：首版（不回指前版）在（租户，分配）上唯一。变化与撤回各自回指前版，不进这道索引。
CREATE UNIQUE INDEX load_assignment_one_root_per_assignment
    ON transport_fulfillment.load_assignment (tenant_id, assignment_ref)
    WHERE corrects_version IS NULL;
