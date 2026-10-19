-- 有效交付的每一版随记交付门禁读数（ADR-0213 决定五；票 demo-chain-gaps/66）。
--
-- **读数是同一行上的一格，不是拒收的理由。** 登记时在写事务之前按交付意图（at 取对象进入这次交付所在那一段时那次交接的
-- 交出节点）向关务的动作准入判断问，问来的答复——放行、阻断、不受门禁、未决、读不动——照实存下，阻断也照记；与出发、
-- 交出门禁读数（迁移 0026、0041）同一套封闭集。每一版都带：POD 更正改的是证据，不是在哪、何时、交了哪件，更正版照前版
-- 承继同一份。
--
-- 读数整格另成一列并收进封闭词表，按格查「哪些交付被阻断」不必解 jsonb；停点、问关务时的 at 与那一格答复存 jsonb。
-- 一份交付恰一件对象，所以明细里停点与答复各至多一个、恰有其一。整格与明细是否一致由领域重建门核
-- （RehydrateDeliveryGateReading）。问了关务的那一格答的是不是这一行的对象，库面也守一道，理由同迁移 0041：对不上的行一旦
-- 进库，重建门拒装回，重放与 PS 终局的消费者都会把它当未决一再重试，所以在写入那一刻拒。at 取自段册与交接册，不在这一行，
-- 库面守不到，由写事务之前问读数的那一侧一处定。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：可空列使用前先 IS NULL / IS NOT NULL；jsonb 取键可能取到
-- NULL，形状判断包在 CASE 里，取不到时落 ELSE false，而不是让 NULL 把 CHECK 放过去。
ALTER TABLE transport_fulfillment.effective_delivery
    ADD COLUMN gate_reading        text,
    ADD COLUMN gate_reading_detail jsonb;

-- 本迁移之前落册的有效交付没问过关务。如实记成未决、停点是「登记于门禁读数之前」，不猜一格。这是一次形状回填，不是改写
-- 交付：各列一字不动、内容摘要不重算（读数不进摘要），补的是那时没有的那一格的诚实答案。
UPDATE transport_fulfillment.effective_delivery
   SET gate_reading = 'UNDECIDED',
       gate_reading_detail = '{"node": null, "stop": "RECORDED_BEFORE_GATE_READING", "finding": null}'::jsonb;

ALTER TABLE transport_fulfillment.effective_delivery
    ALTER COLUMN gate_reading SET NOT NULL,
    ALTER COLUMN gate_reading_detail SET NOT NULL;

ALTER TABLE transport_fulfillment.effective_delivery
    ADD CONSTRAINT effective_delivery_gate_reading_closed
        CHECK (gate_reading IN ('NOT_GUARDED', 'ADMITTED', 'UNDECIDED', 'UNREADABLE', 'BLOCKED')),

    -- 停点与那一格答复恰有其一；问了关务（有那一格）就一定有 at，停在本上下文这一侧就没有。
    ADD CONSTRAINT effective_delivery_gate_reading_detail_shape
        CHECK (
            CASE
                WHEN jsonb_typeof(gate_reading_detail) = 'object'
                     AND jsonb_typeof(gate_reading_detail -> 'node') IN ('string', 'null')
                     AND jsonb_typeof(gate_reading_detail -> 'stop') IN ('string', 'null')
                     AND jsonb_typeof(gate_reading_detail -> 'finding') IN ('object', 'null')
                THEN (jsonb_typeof(gate_reading_detail -> 'stop') = 'string')
                     <> (jsonb_typeof(gate_reading_detail -> 'finding') = 'object')
                     AND (jsonb_typeof(gate_reading_detail -> 'finding') = 'object')
                         = (jsonb_typeof(gate_reading_detail -> 'node') = 'string')
                ELSE false
            END
        ),

    -- 问了关务的那一格答的就是这一行的对象。停在本上下文这一侧的读数没问过谁，不在这道里。
    ADD CONSTRAINT effective_delivery_gate_reading_matches_delivery
        CHECK (
            CASE
                WHEN jsonb_typeof(gate_reading_detail -> 'finding') = 'object'
                THEN (gate_reading_detail -> 'finding' ->> 'object') IS NOT DISTINCT FROM object_ref
                ELSE true
            END
        );
