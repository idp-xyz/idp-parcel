-- 实际承运商判断（tf-segment-lifecycle-closure/02，ADR-0103）。
--
-- CONTEXT「实际承运商判断」：本上下文就一个实际履约段是谁在承运所形成的带版本的判断记录；每个版本固定
-- 判断值、业务时间、判断形成时间与依据的来源事实；待确认是一个有起止的版本而不是判断缺席。规则节：
-- 「新版本只追加、不覆盖」「不追溯覆盖原来的未知期间和判断历史」。
--
-- 三张表而不是一张：头行钉段与段成立时刻；版本行一版一行、**主键带序号、只插不改**——改判就是新版本
-- （ADR-0103 决定八「不给判断开改判口」），表上没有 current 列，当前版按序号最大者派生；依据行按（版本，
-- 来源事实引用）一行，**逐版本各存一份**——每个版本固定它自己的依据列表（决定二），撤回一条依据重新派生
-- 时新版本不含它、旧版本原样带着它，「不倒填」在表形上就成立，代价是几行重复，而一个段的依据只有几条。
--
-- 判断不是段或参与关系上的一格：本迁移不给 actual_fulfillment_segment / fulfillment_participation 加任何列。
--
-- 逐格完备性在库内再守一遍：领域构造门与重建门是第一道，CHECK 是第二道，两道互补不互替（0001 立的纪律）。
-- 全部 CHECK 过 SQL 三值逻辑那一眼：可空列使用前先 IS NULL / IS NOT NULL。

-- 头行：键（租户，段）与段成立时刻。段成立时刻是首版业务时间的下界，由领域按成员最早控制起点派生后
-- 存在这里——判断自己要拿它守「业务时间不得早于段成立」，不每次回去算段。
CREATE TABLE transport_fulfillment.actual_carrier_judgment (
    tenant_id              text        NOT NULL,
    segment_ref            text        NOT NULL,
    segment_established_at timestamptz NOT NULL,
    recorded_at            timestamptz NOT NULL,

    CONSTRAINT actual_carrier_judgment_pkey
        PRIMARY KEY (tenant_id, segment_ref),

    -- 一段一份判断历史；没有段就没有判断（决定一）。
    CONSTRAINT actual_carrier_judgment_segment_fkey
        FOREIGN KEY (tenant_id, segment_ref)
        REFERENCES transport_fulfillment.actual_fulfillment_segment (tenant_id, segment_ref),

    CONSTRAINT actual_carrier_judgment_scope_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(segment_ref) <> '')
);

-- 版本：判断值恰居其一——已识别带承运主体两列，待确认带原因一列；序号从 1 起。
--
-- subject_kind 封闭两支（外部参与方 / 自营运营法人，决定三），subject_ref 只是对 party-commercial 身份的
-- 引用——本表不存承运方名称，名称在依据行的素材列上：判断值里存名称串正是「从品牌或单号推断」（决定八）。
CREATE TABLE transport_fulfillment.actual_carrier_judgment_version (
    tenant_id      text        NOT NULL,
    segment_ref    text        NOT NULL,
    sequence_no    integer     NOT NULL,

    subject_kind   text,
    subject_ref    text,
    pending_reason text,

    -- 业务时间取自依据的来源事实（源的）；形成时间由本上下文铸（本仓的）；落库时刻另记（决定五）。
    business_time  timestamptz NOT NULL,
    formed_at      timestamptz NOT NULL,
    recorded_at    timestamptz NOT NULL,

    CONSTRAINT actual_carrier_judgment_version_pkey
        PRIMARY KEY (tenant_id, segment_ref, sequence_no),

    CONSTRAINT actual_carrier_judgment_version_judgment_fkey
        FOREIGN KEY (tenant_id, segment_ref)
        REFERENCES transport_fulfillment.actual_carrier_judgment (tenant_id, segment_ref),

    CONSTRAINT actual_carrier_judgment_version_sequence_positive
        CHECK (sequence_no >= 1),

    -- 判断值恰居其一：没有「既不识别也无原因」的第三态，也没有「既识别又待确认」。
    CONSTRAINT actual_carrier_judgment_version_verdict_exclusive
        CHECK (
            (subject_kind IS NOT NULL AND subject_ref IS NOT NULL AND pending_reason IS NULL)
            OR (subject_kind IS NULL AND subject_ref IS NULL AND pending_reason IS NOT NULL)
        ),

    CONSTRAINT actual_carrier_judgment_version_subject_kind_closed
        CHECK (subject_kind IS NULL OR subject_kind IN ('EXTERNAL_PARTY', 'OPERATING_LEGAL_ENTITY')),

    CONSTRAINT actual_carrier_judgment_version_subject_ref_not_blank
        CHECK (subject_ref IS NULL OR btrim(subject_ref) <> ''),

    -- 待确认原因封闭三格（决定二；按 ADR-0029 分格）。
    CONSTRAINT actual_carrier_judgment_version_pending_reason_closed
        CHECK (
            pending_reason IS NULL
            OR pending_reason IN ('NO_QUALIFIED_EVIDENCE', 'SOURCE_CONFLICT', 'IDENTITY_NOT_REGISTERED')
        )
);

-- 依据：某一版在场的一条来源事实。来源封闭四格（决定四）；承运主体要么是在册身份引用、要么只有名称素材，
-- 恰居其一——「名称作为素材随依据保留」（决定六）就落在 name_material 这一列上，它只在身份未登记时非空。
CREATE TABLE transport_fulfillment.actual_carrier_judgment_basis (
    tenant_id       text        NOT NULL,
    segment_ref     text        NOT NULL,
    sequence_no     integer     NOT NULL,
    evidence_ref    text        NOT NULL,

    evidence_source text        NOT NULL,
    occurred_at     timestamptz NOT NULL,

    subject_kind    text,
    subject_ref     text,
    name_material   text,

    -- 同一版里同一来源事实只出现一次。
    CONSTRAINT actual_carrier_judgment_basis_pkey
        PRIMARY KEY (tenant_id, segment_ref, sequence_no, evidence_ref),

    CONSTRAINT actual_carrier_judgment_basis_version_fkey
        FOREIGN KEY (tenant_id, segment_ref, sequence_no)
        REFERENCES transport_fulfillment.actual_carrier_judgment_version (tenant_id, segment_ref, sequence_no),

    CONSTRAINT actual_carrier_judgment_basis_evidence_ref_not_blank
        CHECK (btrim(evidence_ref) <> ''),

    -- 领域注释：「运输委托、承运接受、订舱、舱单、面单、渠道品牌与单号都不是来源」。库面镜像封闭集，
    -- 绕过构造门的写入路径同样不该落进一个集外取值。
    CONSTRAINT actual_carrier_judgment_basis_source_closed
        CHECK (evidence_source IN (
            'CARRIER_PICKUP_SCAN', 'CARRIER_RECEIPT_VOUCHER', 'TRANSPORT_HANDOVER_TO_CARRIER', 'TRUSTED_CHANNEL_CALLBACK'
        )),

    CONSTRAINT actual_carrier_judgment_basis_subject_exclusive
        CHECK (
            (subject_kind IS NOT NULL AND subject_ref IS NOT NULL AND name_material IS NULL)
            OR (subject_kind IS NULL AND subject_ref IS NULL AND name_material IS NOT NULL)
        ),

    CONSTRAINT actual_carrier_judgment_basis_subject_kind_closed
        CHECK (subject_kind IS NULL OR subject_kind IN ('EXTERNAL_PARTY', 'OPERATING_LEGAL_ENTITY')),

    CONSTRAINT actual_carrier_judgment_basis_subject_ref_not_blank
        CHECK (subject_ref IS NULL OR btrim(subject_ref) <> ''),

    CONSTRAINT actual_carrier_judgment_basis_material_not_blank
        CHECK (name_material IS NULL OR btrim(name_material) <> '')
);

-- 两条**故意没有下沉**的不变量，读到这里的人会问「那……呢」，答案是有人守，只是不在这几张表上：
--
--   · 「业务时间不得早于段成立」——跨表（版本行对头行），CHECK 表达不动；留在领域（转换门与重建门各守一次）。
--   · 「无合格证据 ⇔ 没有依据」——跨表（版本行对依据行），同上。
--
-- 硬写成触发器就等于为同一条不变量立第二个口径，领域改一次判据、那一份会悄悄漂移（0006 立的同一条纪律）。
