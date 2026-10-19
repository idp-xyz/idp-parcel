-- 权威运输交接的`已交接`版本随记交出门禁读数（ADR-0213 决定四；票 demo-chain-gaps/65）。
--
-- **读数是同一行上的一格，不是拒收的理由。** 登记时在写事务之前按交出意图（at 取交出节点、onward 取去向节点）向关务的
-- 动作准入判断问，问来的答复——放行、阻断、不受门禁、未决、读不动——照实存下，阻断也照记；与出发门禁读数（迁移 0026）
-- 同一套封闭集。只有`已交接`带读数：拒收与待确认不转出控制，没有交出可读。
--
-- 读数整格另成一列并收进封闭词表，按格查「哪些交出被阻断」不必解 jsonb；停点、问关务时的意图与那一格答复存 jsonb。
-- 交接是对象级判断，一版恰一件对象，所以明细里停点与答复各至多一个、恰有其一。整格与明细是否一致、读数与这一版的
-- 对象和节点声明是否对得上，由领域重建门核（RehydrateHandoverGateReading、RehydrateTransportHandover），库面只守形状。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：可空列使用前先 IS NULL / IS NOT NULL；jsonb 取键可能取到
-- NULL，形状判断包在 CASE 里，取不到时落 ELSE false，而不是让 NULL 把 CHECK 放过去。
ALTER TABLE transport_fulfillment.transport_handover
    ADD COLUMN gate_reading        text,
    ADD COLUMN gate_reading_detail jsonb;

-- 本迁移之前落册的`已交接`没问过关务。如实记成未决、停点是「登记于门禁读数之前」，不猜一格；拒收与待确认不带读数。
-- 这是一次形状回填，不是改写交接：各列一字不动、内容摘要不重算（读数不进摘要），补的是那时没有的那一格的诚实答案。
UPDATE transport_fulfillment.transport_handover
   SET gate_reading = 'UNDECIDED',
       gate_reading_detail = '{"node": null, "onward": null, "stop": "RECORDED_BEFORE_GATE_READING", "finding": null}'::jsonb
 WHERE verdict = 'HANDED_OVER';

ALTER TABLE transport_fulfillment.transport_handover
    -- `已交接`必带读数，拒收与待确认不带。
    ADD CONSTRAINT transport_handover_gate_reading_on_handed_over
        CHECK ((verdict = 'HANDED_OVER') = (gate_reading IS NOT NULL)),

    ADD CONSTRAINT transport_handover_gate_reading_closed
        CHECK (gate_reading IS NULL OR gate_reading IN ('NOT_GUARDED', 'ADMITTED', 'UNDECIDED', 'UNREADABLE', 'BLOCKED')),

    ADD CONSTRAINT transport_handover_gate_reading_detail_coupled
        CHECK ((gate_reading IS NULL) = (gate_reading_detail IS NULL)),

    ADD CONSTRAINT transport_handover_gate_reading_detail_shape
        CHECK (
            gate_reading_detail IS NULL
            OR CASE
                WHEN jsonb_typeof(gate_reading_detail) = 'object'
                     AND jsonb_typeof(gate_reading_detail -> 'stop') IN ('string', 'null')
                     AND jsonb_typeof(gate_reading_detail -> 'finding') IN ('object', 'null')
                THEN (jsonb_typeof(gate_reading_detail -> 'stop') = 'string')
                     <> (jsonb_typeof(gate_reading_detail -> 'finding') = 'object')
                ELSE false
            END
        );
