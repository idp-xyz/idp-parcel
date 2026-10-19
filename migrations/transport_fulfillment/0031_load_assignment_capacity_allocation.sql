-- 装载分配逐池的分配量，与「一版至多被一个后继回指」（票 demo-chain-gaps/21，ADR-0203 决定二与文末补记）。
--
-- 分配量随装载分配的版本存，一版一池一行，只插不改，与成员行同一纪律：每一版各存自己的一组，撤回版一行也不带。预占上的
-- 分配关系不另存，读池时取各分配当前那一版（没有后继、未撤回）的这几行派生——分配量只在这里存一份，撤回或改版之后，
-- 当前版换了，旧版那一组自然不再算数，归还不靠任何人记得去改预占。
--
-- 派生「当前那一版」靠版本链是一条线：0021 只守一分配一个首版，不守一版只被回指一次。一版若长出两个后继，两个都没有
-- 后继、都算当前，同一次分配在预占上就被算了两遍。所以这里补那一道唯一。变化与撤回的入口还没接，并列后继今天只有直接
-- 写库才造得出；存量里真有就拒绝施加，不替谁挑留哪一根，理由同 0021。
--
-- 预占外键延迟到提交时核：池按值换存（Replace 先删本池全部预占再按新值插回），同一笔里预占行有一刻不在，提交时都已插回。
-- 立即核的外键会让每一次预占、释放都撞在已有分配上；随预占删又会让换存把分配量一并删掉。
--
-- 单位照声明存一份：它是这一版声明了什么的记录，读回装载分配不必去读池。它必然等于池的单位（不等不成分配），跨表的
-- 这一条库上不钉，由编排判。来源种类与预占同一组封闭词（0030）。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：本迁移新增的列都是 NOT NULL。
DO $$
DECLARE
    forked text;
BEGIN
    SELECT string_agg(format('%s/%s 的 %s（%s 个后继）', tenant_id, assignment_ref, corrects_version, successors),
                      '，' ORDER BY tenant_id, assignment_ref, corrects_version)
      INTO forked
      FROM (
          SELECT tenant_id, assignment_ref, corrects_version, count(*) AS successors
            FROM transport_fulfillment.load_assignment
           WHERE corrects_version IS NOT NULL
           GROUP BY tenant_id, assignment_ref, corrects_version
          HAVING count(*) > 1
      ) AS parallel_successors;
    IF forked IS NOT NULL THEN
        RAISE EXCEPTION USING MESSAGE =
            'load_assignment 存在被并列回指的版本，不建「一版至多一个后继」索引，也不代删任何一根：' || forked
            || '。请逐个分配查明哪一个后继是那次变化或撤回，其余导出备查后由写入方处置，再重跑本迁移。';
    END IF;
END
$$;

-- 一版至多一个后继：后继（回指前版的版本）在（租户，分配，前版）上唯一。首版不回指，不进这道索引。
CREATE UNIQUE INDEX load_assignment_one_successor_per_version
    ON transport_fulfillment.load_assignment (tenant_id, assignment_ref, corrects_version)
    WHERE corrects_version IS NOT NULL;

CREATE TABLE transport_fulfillment.load_assignment_allocation (
    tenant_id      text        NOT NULL,
    assignment_ref text        NOT NULL,
    version        text        NOT NULL,
    pool_id        text        NOT NULL,

    reservation_id text        NOT NULL,
    quantity       bigint      NOT NULL,
    unit_ref       text        NOT NULL,
    source_kind    text        NOT NULL,
    source_ref     text        NOT NULL,
    quantity_basis text        NOT NULL,

    recorded_at    timestamptz NOT NULL,

    -- 一版一池一行：判据是对每个池引用一份预占，同一个池两行就是在一个池上分两份。
    CONSTRAINT load_assignment_allocation_pkey
        PRIMARY KEY (tenant_id, assignment_ref, version, pool_id),

    CONSTRAINT load_assignment_allocation_version_fkey
        FOREIGN KEY (tenant_id, assignment_ref, version)
        REFERENCES transport_fulfillment.load_assignment (tenant_id, assignment_ref, version),

    CONSTRAINT load_assignment_allocation_pool_fkey
        FOREIGN KEY (tenant_id, pool_id)
        REFERENCES transport_fulfillment.capacity_pool (tenant_id, pool_id),

    CONSTRAINT load_assignment_allocation_reservation_fkey
        FOREIGN KEY (tenant_id, pool_id, reservation_id)
        REFERENCES transport_fulfillment.capacity_reservation (tenant_id, pool_id, reservation_id)
        DEFERRABLE INITIALLY DEFERRED,

    CONSTRAINT load_assignment_allocation_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(assignment_ref) <> ''
            AND btrim(version) <> ''
            AND btrim(pool_id) <> ''
            AND btrim(reservation_id) <> ''
            AND btrim(unit_ref) <> ''
            AND btrim(source_ref) <> ''
        ),

    CONSTRAINT load_assignment_allocation_quantity_positive
        CHECK (quantity > 0),

    CONSTRAINT load_assignment_allocation_source_kind_closed
        CHECK (source_kind IN ('OPERATOR_DECLARATION', 'PREPARATION_DEMAND_SNAPSHOT')),

    CONSTRAINT load_assignment_allocation_quantity_basis_closed
        CHECK (quantity_basis IN ('DECLARED', 'MEMBER_COUNT')),

    -- 只有件数维的池可以拿成员数作分配量（UC-TF-003 禁止用对象数顶替别的维度）。池身份由（班次、维度）派生，0027 已钉住
    -- 派生，所以单看池身份的维度词就判得出。
    CONSTRAINT load_assignment_allocation_member_count_on_pieces
        CHECK (quantity_basis <> 'MEMBER_COUNT' OR pool_id LIKE '%/PIECES')
);

-- 读池时按（租户，池）取分配关系。
CREATE INDEX load_assignment_allocation_by_pool
    ON transport_fulfillment.load_assignment_allocation (tenant_id, pool_id, reservation_id);

-- 两条故意没有下沉的不变量，理由同 0009 文末：
--
-- 一、撤回版不带分配量。它要看版本行的 withdrawn，是跨表条件；留在领域（Withdraw 清空、重建门拒）与登记册写入前的核对，
--     读池派生时另按未撤回过滤一遍。
-- 二、已分配 + 已释放 + 已消耗 ≤ 预占量。分配量在这张表、释放与消耗在预占行上，跨表；留在领域的分配判定与池的重建门。
