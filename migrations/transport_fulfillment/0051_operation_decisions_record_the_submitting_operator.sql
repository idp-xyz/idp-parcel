-- 运营决定把提交操作者记在决定记录上（ADR-0303；票 demo-chain-gaps/126）。
--
-- 关段、明确终止参与、建派送任务、装载分配、有效时间判断、承运商首次有效收寄判断与运输委托提交七口，各在自己的决定记录上
-- 多一列操作证据：作出这一次决定的操作者，取自操作者信封，写法与费用发生登记的 registered_by 同一个「发行方#sub」（0034）。
-- 只进审计：重放、内容冲突与任何门都不读这一列，同一个决定换一位操作者重放，在册的仍是先到的那一位。
--
-- 可空，不立 NOT NULL，也不回填：
--   - 存量行没有可回填的来源——0051 之前这几口只收租户，是谁做的决定从未落过任何地方；拿别的列或登记时刻推一个人出来就是
--     编造证据。存量行留 NULL，读作「没记下是谁」，不读作「没有人做」。
--   - 同一张表上本就有不经运营决定口形成的行：派送发起执行器形成的任务、按规则判断或素材到达形成的轨迹事实版本、以有效交付
--     或下一次交接结束的参与。它们没有一位提交操作者，NOT NULL 会逼着写入方编一个。
-- 给了就不得是空白（与 0034 同一条），并且只出现在它能属于的行上：closed_by 只在已关闭的段上，terminated_by 只在以明确
-- 终止结束的参与上，effective_judged_by 只在显式判断的版本上。
--
-- 存量行都是 NULL，新的 CHECK 对它们恒成立，直接校验，不必 NOT VALID。全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的
-- 纪律）：新列可空，先 IS NULL；end_kind 可空，用之前先 IS NOT NULL。

-- 关段：段上的关闭标志与时刻旁边记下作出关段决定的人。
ALTER TABLE transport_fulfillment.actual_fulfillment_segment
    ADD COLUMN closed_by text;

ALTER TABLE transport_fulfillment.actual_fulfillment_segment
    ADD CONSTRAINT actual_fulfillment_segment_closed_by_only_when_closed
        CHECK (closed_by IS NULL OR (closed AND btrim(closed_by) <> ''));

-- 明确终止：终点各列旁边记下作出终止的人。替代版本承继终点时连同这一列一起承继（同一次终止决定），所以它可以出现在
-- 回指前版的行上。
ALTER TABLE transport_fulfillment.fulfillment_participation
    ADD COLUMN terminated_by text;

ALTER TABLE transport_fulfillment.fulfillment_participation
    ADD CONSTRAINT fulfillment_participation_terminated_by_only_when_terminated
        CHECK (
            terminated_by IS NULL
            OR (end_kind IS NOT NULL AND end_kind = 'CONTROL_TERMINATED' AND btrim(terminated_by) <> '')
        );

-- 建派送任务：手工建任务的那一位；派送发起执行器形成的任务留 NULL。
ALTER TABLE transport_fulfillment.dispatch_task
    ADD COLUMN opened_by text;

ALTER TABLE transport_fulfillment.dispatch_task
    ADD CONSTRAINT dispatch_task_opened_by_not_blank
        CHECK (opened_by IS NULL OR btrim(opened_by) <> '');

-- 装载分配：每一版各记各的，不随链承继。
ALTER TABLE transport_fulfillment.load_assignment
    ADD COLUMN decided_by text;

ALTER TABLE transport_fulfillment.load_assignment
    ADD CONSTRAINT load_assignment_decided_by_not_blank
        CHECK (decided_by IS NULL OR btrim(decided_by) <> '');

-- 有效时间判断：显式判断形成的那一版记下判断人；按规则判断与素材到达形成的版本留 NULL。
ALTER TABLE transport_fulfillment.external_carrier_tracking_fact
    ADD COLUMN effective_judged_by text;

ALTER TABLE transport_fulfillment.external_carrier_tracking_fact
    ADD CONSTRAINT external_carrier_tracking_fact_judged_by_only_when_explicit
        CHECK (
            effective_judged_by IS NULL
            OR (effective_basis = 'JUDGED_EXPLICITLY' AND btrim(effective_judged_by) <> '')
        );

-- 承运商首次有效收寄判断：每一版各记各的判断人，与 judged_at 并列。
ALTER TABLE transport_fulfillment.carrier_first_effective_pickup
    ADD COLUMN judged_by text;

ALTER TABLE transport_fulfillment.carrier_first_effective_pickup
    ADD CONSTRAINT carrier_first_effective_pickup_judged_by_not_blank
        CHECK (judged_by IS NULL OR btrim(judged_by) <> '');

-- 运输委托提交：提交那一位，与 submitted_at 并列。取消与开始只改各自的列（Replace），不碰这一列。
ALTER TABLE transport_fulfillment.transport_commission
    ADD COLUMN submitted_by text;

ALTER TABLE transport_fulfillment.transport_commission
    ADD CONSTRAINT transport_commission_submitted_by_not_blank
        CHECK (submitted_by IS NULL OR btrim(submitted_by) <> '');
