-- 受控通道执行留痕：治理登记身份双轨的第①轨（票 12 裁决）。通道技术身份（OS 进程
-- 属主、主机名）由登记入口自取，不可由参数传入或覆盖——它证的是「这笔登记经受控
-- 通道执行」这一事实；第②轨（决定人/执行人）是登记内容，在各记录表自己的列上。
-- 两轨分开保存，本表不承载任何登记内容，登记表也不承载通道身份。
--
-- 只追加：每次执行各留一行（重放登记的执行同样是一次执行），无 UPDATE 路径，
-- 也不设幂等约束——留痕做幂等等于抹掉「同一登记被执行过几次」这件事实。
CREATE TABLE pilot_governance.channel_execution (
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
