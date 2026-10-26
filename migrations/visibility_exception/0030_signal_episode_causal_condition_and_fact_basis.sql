-- 信号发作期补上因果条件与事实依据两维（票 demo-chain-gaps/135，代决复核报告 M18；ADR-0205 文末补记越权风险点 22、23）。
--
-- CONTEXT 要求「同一对象、类型、因果条件和连续影响期内的重复命中更新同一信号发作期」，又要求「每个信号必须保存……事实依据」。
-- 0002 立表时发作期只按（对象，类型）定位、不存依据：一件包裹头一次出发被关务判阻断开了发作期，此后另一次违规出发也只给它
-- 记一次命中、不再分诊，人工复核也追不回是哪一次出发、关务凭哪一份判断答的阻断。
--
-- causal_condition 进定位键；fact_basis 是一个数组，每条指回本上下文已接受事实的键维（源上下文、事实引用、版本），可带源事实里随带
-- 的判断标识（出发门禁读数逐对象那一格的关务判断标识），形状见适配器 signalFactBasisRow。
--
-- 存量行回填为 RECORDED_BEFORE_CAUSAL_CONDITION、依据为空数组：它们当初按（对象，类型）定位，记的是哪一次原因已经说不出，替它们
-- 猜一个原因就是虚构依据。新命中带的因果条件永远不等于这一格，所以存量发作期从此不再认领新命中——它们照旧可读回、可查阅；同一
-- 原因在本迁移之后再命中会另开一期、再分诊一次，这是如实回填的代价（票面代决记明）。新写入的发作期依据至少一条，CHECK 只放存量
-- 那一格为空。

ALTER TABLE visibility_exception.signal_episode
    ADD COLUMN causal_condition text,
    ADD COLUMN fact_basis       jsonb;

UPDATE visibility_exception.signal_episode
   SET causal_condition = 'RECORDED_BEFORE_CAUSAL_CONDITION',
       fact_basis       = '[]'::jsonb;

ALTER TABLE visibility_exception.signal_episode
    ALTER COLUMN causal_condition SET NOT NULL,
    ALTER COLUMN fact_basis       SET NOT NULL,
    ADD CONSTRAINT signal_episode_causal_condition_not_blank
        CHECK (btrim(causal_condition) <> ''),
    ADD CONSTRAINT signal_episode_fact_basis_shape
        CHECK (
            -- AND 不保证求值次序，jsonb_array_length 遇到非数组会报错而不是答假，所以先用 CASE 判形。
            CASE WHEN jsonb_typeof(fact_basis) = 'array'
                 THEN (causal_condition = 'RECORDED_BEFORE_CAUSAL_CONDITION') = (jsonb_array_length(fact_basis) = 0)
                 ELSE false
            END
        );

-- 命中定位的读面：FindLatestForCause 按（租户，对象，类型，因果条件）取最近一期。0003 那条（租户，对象，类型）索引留给不问因果条件
-- 的 FindLatest。
CREATE INDEX signal_episode_latest_for_cause
    ON visibility_exception.signal_episode (tenant_id, parcel_ref, kind_ref, causal_condition, started_at DESC, seq DESC);
