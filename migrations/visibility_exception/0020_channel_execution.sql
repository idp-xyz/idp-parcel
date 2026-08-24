-- 受控通道执行留痕：VE 目录登记身份双轨的第①轨（票 12 裁决，经票 15 沿用到本上下
-- 文）。通道技术身份（OS 进程属主、主机名）由登记入口自取，不可由参数传入或覆盖——
-- 它证的是「这笔登记经受控通道执行」这一事实；第②轨（发布批准责任 approved_by）是
-- 登记内容，在各目录表自己的列上。两轨分开保存，本表不承载任何目录内容，目录表也不
-- 承载通道身份。表归本上下文自己：VE 的登记不跨库写 pilot_governance 的同名表——
-- 限界上下文表达数据所有权。
--
-- 只追加：每次执行各留一行（同一登记被执行过几次本身是要留的事实），无 UPDATE 路径，
-- 也不设幂等约束——留痕做幂等等于抹掉执行次数这件事实。
CREATE TABLE visibility_exception.channel_execution (
    execution_id     bigint      GENERATED ALWAYS AS IDENTITY,

    command          text        NOT NULL,
    record_reference text        NOT NULL,
    os_user          text        NOT NULL,
    hostname         text        NOT NULL,
    outcome          text        NOT NULL,
    executed_at      timestamptz NOT NULL,
    inserted_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT channel_execution_pkey PRIMARY KEY (execution_id),
    CONSTRAINT channel_execution_not_blank
        CHECK (
            btrim(command) <> ''
            AND btrim(record_reference) <> ''
            AND btrim(os_user) <> ''
            AND btrim(hostname) <> ''
            AND btrim(outcome) <> ''
        )
);
