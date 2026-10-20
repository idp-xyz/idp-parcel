-- 预报有效时窗的收口（票 ps-external-mark-relations/14 裁定；PAR-NET-18）：登记时没有终点的那一行，由一份显式追加的收口结束，写明
-- 收口时刻与依据；收口之后，起点不早于收口时刻的新行照常登记。册照旧只增：在册的时窗行不改写，收口是另一张表上的另一份事实，
-- 状态由「有没有那一行收口」读出，理由同迁移 0013 头注。不取「登新行即隐式截断前一行」：那等于改历史，一次笔误就会悄悄截断在用
-- 的时窗。
--
-- 本迁移不含任何实例默认值。

-- 收口：一行即给一行开放终点的时窗落下的收口。主键即那一行（租户，起点），一行至多收口一次；外键指回那一行，收不了不存在的行。
-- 收口时刻晚于那一行的起点，库面再守一道（领域在形成处守），恰在起点会让那一行的生效区间变空。「只收登记时没有终点的行」跨表，
-- 库面 CHECK 够不着，由写口的插入谓词守（PreAdviceWindows.ClosePreAdviceWindow）。
CREATE TABLE node_operations.pre_advice_window_closure (
    tenant_id             text        NOT NULL,
    window_effective_from timestamptz NOT NULL,

    closed_at             timestamptz NOT NULL,
    basis_ref             text        NOT NULL,
    registered_at         timestamptz NOT NULL,

    CONSTRAINT pre_advice_window_closure_pkey
        PRIMARY KEY (tenant_id, window_effective_from),

    CONSTRAINT pre_advice_window_closure_window_fkey
        FOREIGN KEY (tenant_id, window_effective_from)
        REFERENCES node_operations.pre_advice_window (tenant_id, effective_from),

    CONSTRAINT pre_advice_window_closure_not_blank
        CHECK (btrim(basis_ref) <> ''),

    CONSTRAINT pre_advice_window_closure_after_start
        CHECK (closed_at > window_effective_from)
);

-- 同租户生效区间不重叠，自此按「收口之后的有效终点」判：收过口的行，终点是收口时刻。排他约束 pre_advice_window_no_overlap 比的
-- 是每一行登记时给的区间，开放终点那一行的区间永远是〔起点，无穷），而行不改写，它就会拒掉此后每一个后继——那正是本迁移要解的
-- 现象。排他约束只能比同一张表上同一行的列，读不到收口表，所以它退场，不留半截（只守带终点各行之间的半截约束会让人以为库面还管
-- 着重叠）。
--
-- 不重叠改由写口交付：时窗册的两个写口（登记、收口）先取同一把以租户为键的事务级咨询锁，再在同一笔事务的下一条语句里按有效终点
-- 查重叠、插入（PreAdviceWindows）。事务是读已提交，取锁之后的语句看得见先到那一笔已提交的行与收口。迁移 0013 头注说排他约束是
-- 「唯一不漏首插并发的那条路」，指的是行锁——首条登记没有可锁的行；咨询锁不要已有行，首插并发同样串行。
ALTER TABLE node_operations.pre_advice_window
    DROP CONSTRAINT pre_advice_window_no_overlap;
