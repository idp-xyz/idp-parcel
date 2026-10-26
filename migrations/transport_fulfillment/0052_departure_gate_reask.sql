-- 出发门禁补问读数（票 demo-chain-gaps/121，ADR-0305）：出发落册那一刻停在本侧（段不在册、那一刻段里没有在控对象）或某一口
-- 读不动的出发，日后对象凭`已交接`或对象级场外揽收进了段、或又有一份这样的控制事实时，按原出发时点替这件对象再问一次关务，
-- 问来的读数追加在这里。
--
-- **首版不改。** 移动事实那一行的 gate_reading 是登记那一刻问来的记录，只插不改（0026）；补问另起一行，一次补问一行，同一条
-- 出发的同一件对象按 sequence 从 1 往上数，撞键即另一方先补了这一次（答已登记，不覆盖）。
--
-- **范围是一件对象。** 补问由一件对象的控制事实唤醒，只问这一件（集运单元按出发那一刻的成员展开）；object_ref 是唤醒它的那一件，
-- 读数里逐对象那几格是这一件与它展开出来的成员。trigger_ref 记唤醒它的那一份控制事实，供人追溯，不进任何判断。
--
-- 外键挂在移动事实那一版上：补问只补在册的出发。读数两列的形状与 0026 同一纪律——整格收进封闭词表，停点与逐对象答复恰有其一，
-- 整格与明细是否一致由领域重建门核（RehydrateDepartureGateReading）。
--
-- 全部 CHECK 过 SQL 三值逻辑那一眼（f822e1d 入册的纪律）：本表各列都不可空；jsonb 取键可能取到 NULL，形状判断包在 CASE 里，
-- 取不到时落 ELSE false。
CREATE TABLE transport_fulfillment.departure_gate_reask (
    tenant_id           text        NOT NULL,
    fact_ref            text        NOT NULL,
    version             text        NOT NULL,
    object_ref          text        NOT NULL,
    sequence            integer     NOT NULL,
    gate_reading        text        NOT NULL,
    gate_reading_detail jsonb       NOT NULL,
    trigger_ref         text        NOT NULL,
    recorded_at         timestamptz NOT NULL,

    CONSTRAINT departure_gate_reask_pkey
        PRIMARY KEY (tenant_id, fact_ref, version, object_ref, sequence),

    CONSTRAINT departure_gate_reask_departure
        FOREIGN KEY (tenant_id, fact_ref, version)
        REFERENCES transport_fulfillment.transport_movement_fact (tenant_id, fact_ref, version),

    CONSTRAINT departure_gate_reask_not_blank
        CHECK (btrim(object_ref) <> '' AND btrim(trigger_ref) <> ''),

    CONSTRAINT departure_gate_reask_sequence_positive
        CHECK (sequence >= 1),

    CONSTRAINT departure_gate_reask_reading_closed
        CHECK (gate_reading IN ('NOT_GUARDED', 'ADMITTED', 'UNDECIDED', 'UNREADABLE', 'BLOCKED')),

    CONSTRAINT departure_gate_reask_reading_detail_shape
        CHECK (
            CASE
                WHEN jsonb_typeof(gate_reading_detail) = 'object'
                     AND jsonb_typeof(gate_reading_detail -> 'stops') = 'array'
                     AND jsonb_typeof(gate_reading_detail -> 'findings') = 'array'
                THEN (jsonb_array_length(gate_reading_detail -> 'stops') > 0)
                     <> (jsonb_array_length(gate_reading_detail -> 'findings') > 0)
                ELSE false
            END
        )
);
