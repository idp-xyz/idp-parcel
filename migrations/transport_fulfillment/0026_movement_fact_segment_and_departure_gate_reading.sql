-- 实际移动事实带声明的段与资源，出发与门禁读数同一行落册（票 demo-chain-gaps/32，ADR-0205）。
--
-- **段与资源是登记方声明的引用，不加外键。** 理由同 0010 那句「也没有履约段外键」：事实可能先于段的登记到达，
-- 那时正确的做法是照实记下事实，不是拒收它。段在不在册只影响门禁读数问不问得到对象，不影响落册。
--
-- **出发一律落册，门禁读数是同一行上的一格，不是拒收的理由。** 0010 末尾那条「受监管门禁约束的出发在没有放行依据
-- 时立不成出发」由本迁移退役：设门与否改由本上下文登记时向关务问（ADR-0206 动作准入判断），问来的答复——放行、
-- 阻断、不受门禁、未决、读不动——照实存下，阻断也照记。gate_clearance 一列不动，它从此只是设备出示的证据。
--
-- 读数整格另成一列并收进封闭词表，按格查「哪些出发被阻断」不必解 jsonb；停点、起讫与逐对象答复的形状随格而变，存
-- jsonb。整格与明细是否一致由领域重建门核（RehydrateDepartureGateReading），库面只守形状：停点与逐对象答复恰有其一。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：可空列使用前先 IS NULL / IS NOT NULL；jsonb 取键可能取到
-- NULL，形状判断包在 CASE 里，取不到时落 ELSE false，而不是让 NULL 把 CHECK 放过去。
ALTER TABLE transport_fulfillment.transport_movement_fact
    ADD COLUMN segment_ref         text,
    ADD COLUMN resource_ref        text,
    ADD COLUMN gate_reading        text,
    ADD COLUMN gate_reading_detail jsonb;

-- 本迁移之前落册的出发没问过关务。如实记成未决、停点是「登记于门禁读数之前」，不猜一格；移动与到达不带读数。
-- 这是一次形状回填，不是改写事实：事实各列一字不动，补的是那时没有的那一格的诚实答案。
UPDATE transport_fulfillment.transport_movement_fact
   SET gate_reading = 'UNDECIDED',
       gate_reading_detail = '{"from": null, "to": null, "stops": ["RECORDED_BEFORE_GATE_READING"], "findings": []}'::jsonb
 WHERE kind = 'DEPARTURE';

ALTER TABLE transport_fulfillment.transport_movement_fact
    ADD CONSTRAINT transport_movement_fact_segment_not_blank
        CHECK (segment_ref IS NULL OR btrim(segment_ref) <> ''),

    ADD CONSTRAINT transport_movement_fact_resource_not_blank
        CHECK (resource_ref IS NULL OR btrim(resource_ref) <> ''),

    -- 出发必带读数，移动与到达不带：门禁约束的是装载出发动作。
    ADD CONSTRAINT transport_movement_fact_gate_reading_on_departure
        CHECK ((kind = 'DEPARTURE') = (gate_reading IS NOT NULL)),

    ADD CONSTRAINT transport_movement_fact_gate_reading_closed
        CHECK (gate_reading IS NULL OR gate_reading IN ('NOT_GUARDED', 'ADMITTED', 'UNDECIDED', 'UNREADABLE', 'BLOCKED')),

    ADD CONSTRAINT transport_movement_fact_gate_reading_detail_coupled
        CHECK ((gate_reading IS NULL) = (gate_reading_detail IS NULL)),

    ADD CONSTRAINT transport_movement_fact_gate_reading_detail_shape
        CHECK (
            gate_reading_detail IS NULL
            OR CASE
                WHEN jsonb_typeof(gate_reading_detail) = 'object'
                     AND jsonb_typeof(gate_reading_detail -> 'stops') = 'array'
                     AND jsonb_typeof(gate_reading_detail -> 'findings') = 'array'
                THEN (jsonb_array_length(gate_reading_detail -> 'stops') > 0)
                     <> (jsonb_array_length(gate_reading_detail -> 'findings') > 0)
                ELSE false
            END
        );
