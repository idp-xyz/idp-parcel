-- 版本化网络目录：稳定网络定义六类 + 临时网络可用性调整 + 目录修订锚。
--
-- 本迁移是 ADR-0053 预告的那一步（「下一次真要做网络定义时，先设计原语模式」），由
-- nr-route-evidence-views/issues/01 的用户批复（2026-08-20「建」）启动；决定记录见
-- ADR-0068。列与 CHECK 只由 CONTEXT 硬句推导，不含任何 PAR-NET-* 取值：
--
--   「稳定网络定义和临时网络可用性调整必须分离」——调整独立成表，稳定六表不因调整
--   改行；「节点、连接、线路、服务区域、服务日历或路由策略发生永久变化时形成新版本，
--   不覆盖原版本」——版本行只增不改，同一身份至多一个未闭区间版本由部分唯一索引担保；
--   「新版本自明确生效时间起参与适用范围内的新判断」——生效边界是 effective_from，
--   选版按 [effective_from, effective_to) 判 asOf；「每个节点、网络连接和适用线路必须
--   明确业务时区」「跨节点时间按绝对时刻比较」——三表各带 business_timezone NOT NULL，
--   而有效区间一律 timestamptz（绝对时刻）。
--
-- 折叠语义一列不埋：候选生成、过滤、排序，服务区域/日历/截单/可用性的**采用方式**，
-- 冻结边界与改路条件全属 PAR-NET-14（登记册待提供）。因此服务区域行没有地理内容列
-- （国家/行政区域/邮编范围「或其他」是开放集，现在拟形态就是替租户拟），服务日历行
-- 没有营业日/节假日/服务窗口/截单内容列——它们等 PAR-NET-14 的形态定了再以新迁移
-- 扩列。本迁移只建结构，不种任何默认行（AGENTS.md：机制现在就做，实例留空并拒绝
-- 默认值）。
--
-- 与 network_definition（0007）的分工：那张表按（租户+服务目的）登「有没有网络定义、
-- 视图修订是哪版」，供三个证据视图答`未配置`；本目录是定义原语本体，按租户存放。
-- 视图修订与目录修订的合流属解析层设计（PAR-NET-14 之后）——在解析层存在之前，三个
-- 证据视图不读本目录（票 01 护栏：三口取数侧不接）。

-- 物流节点版本。节点是「可以发生收寄、处理、交接、转运、关务衔接、注入或交付控制
-- 转换的稳定网络位置」（CONTEXT 语言）；节点的网络身份归本上下文，经营方、可使用
-- 法人等归 party-commercial，这里不设它们的列。
CREATE TABLE network_routing.logistics_node_version (
    tenant_id         text        NOT NULL,
    node_code         text        NOT NULL,
    version           integer     NOT NULL,

    -- 「每个节点……必须明确业务时区。截单、营业日和节假日按当地业务语义维护」。
    business_timezone text        NOT NULL,

    effective_from    timestamptz NOT NULL,
    effective_to      timestamptz,

    CONSTRAINT logistics_node_version_pkey
        PRIMARY KEY (tenant_id, node_code, version),

    CONSTRAINT logistics_node_version_positive
        CHECK (version >= 1),
    CONSTRAINT logistics_node_version_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(node_code) <> ''
            AND btrim(business_timezone) <> ''
        ),
    CONSTRAINT logistics_node_version_interval_ordered
        CHECK (effective_to IS NULL OR effective_to > effective_from)
);

-- 同一节点身份至多一个未闭区间版本（先例：VE 映射目录同款索引）。两个并存的当前版
-- 本会让「按 asOf 选版」有两个答案，而选版不许挑一个——库先挡住未闭的那一半，已闭
-- 区间的重叠由读口兜错。
CREATE UNIQUE INDEX logistics_node_version_one_open_per_identity
    ON network_routing.logistics_node_version (tenant_id, node_code)
    WHERE effective_to IS NULL;

-- 网络连接版本。「网络连接必须表达相邻节点之间的明确方向」——from/to 两列即方向；
-- 「两个相邻物流节点之间」要求两端相异。连接引用节点按**身份**（node_code）而非某个
-- 版本行，故不设 FK：某时点两端是否各有适用版本，是折叠层按 asOf 才答得出的问题，
-- 库上一条指向版本行的外键反而把「引用身份」写成了「引用版本」。
CREATE TABLE network_routing.network_connection_version (
    tenant_id         text        NOT NULL,
    connection_code   text        NOT NULL,
    version           integer     NOT NULL,

    from_node_code    text        NOT NULL,
    to_node_code      text        NOT NULL,
    business_timezone text        NOT NULL,

    effective_from    timestamptz NOT NULL,
    effective_to      timestamptz,

    CONSTRAINT network_connection_version_pkey
        PRIMARY KEY (tenant_id, connection_code, version),

    CONSTRAINT network_connection_version_positive
        CHECK (version >= 1),
    CONSTRAINT network_connection_version_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(connection_code) <> ''
            AND btrim(from_node_code) <> ''
            AND btrim(to_node_code) <> ''
            AND btrim(business_timezone) <> ''
        ),
    CONSTRAINT network_connection_version_directed_between_two_nodes
        CHECK (from_node_code <> to_node_code),
    CONSTRAINT network_connection_version_interval_ordered
        CHECK (effective_to IS NULL OR effective_to > effective_from)
);

CREATE UNIQUE INDEX network_connection_version_one_open_per_identity
    ON network_routing.network_connection_version (tenant_id, connection_code)
    WHERE effective_to IS NULL;

-- 线路版本。「线路由一个或多个网络连接按顺序组成」——segments 是连接身份的有序
-- 数组（jsonb），至少一段；「新线路版本……必须具有明确生效时间和适用范围」——
-- applicable_scope 是版本自带的范围声明，其解释属折叠层，这里只要求非空。
CREATE TABLE network_routing.line_version (
    tenant_id         text        NOT NULL,
    line_code         text        NOT NULL,
    version           integer     NOT NULL,

    segments          jsonb       NOT NULL,
    business_timezone text        NOT NULL,
    applicable_scope  text        NOT NULL,

    effective_from    timestamptz NOT NULL,
    effective_to      timestamptz,

    CONSTRAINT line_version_pkey
        PRIMARY KEY (tenant_id, line_code, version),

    CONSTRAINT line_version_positive
        CHECK (version >= 1),
    CONSTRAINT line_version_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(line_code) <> ''
            AND btrim(business_timezone) <> ''
            AND btrim(applicable_scope) <> ''
        ),
    CONSTRAINT line_version_segments_ordered_nonempty
        CHECK (jsonb_typeof(segments) = 'array' AND jsonb_array_length(segments) >= 1),
    CONSTRAINT line_version_interval_ordered
        CHECK (effective_to IS NULL OR effective_to > effective_from)
);

CREATE UNIQUE INDEX line_version_one_open_per_identity
    ON network_routing.line_version (tenant_id, line_code)
    WHERE effective_to IS NULL;

-- 服务区域版本。「带有版本和适用期的……地理覆盖定义」——版本与适用期在此；覆盖
-- 内容（国家、行政区域、邮编范围「或其他」）是开放集，形态未定，不预埋内容列。
CREATE TABLE network_routing.service_area_version (
    tenant_id      text        NOT NULL,
    area_code      text        NOT NULL,
    version        integer     NOT NULL,

    effective_from timestamptz NOT NULL,
    effective_to   timestamptz,

    CONSTRAINT service_area_version_pkey
        PRIMARY KEY (tenant_id, area_code, version),

    CONSTRAINT service_area_version_positive
        CHECK (version >= 1),
    CONSTRAINT service_area_version_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(area_code) <> ''),
    CONSTRAINT service_area_version_interval_ordered
        CHECK (effective_to IS NULL OR effective_to > effective_from)
);

CREATE UNIQUE INDEX service_area_version_one_open_per_identity
    ON network_routing.service_area_version (tenant_id, area_code)
    WHERE effective_to IS NULL;

-- 服务日历版本。「节点、网络连接或线路在明确业务时区下适用的营业日、节假日、服务
-- 窗口和截单条件。服务日历按版本生效，不能通过覆盖历史日历改变既有路由依据」——
-- 身份是（适用对象类别+对象身份），版本行只增不改。业务时区不另设列：「业务时区按
-- 各自节点/线路解释」，日历的时区就是其适用对象的时区，再存一份就是第二处定义。
-- 营业日/节假日/服务窗口/截单内容列等 PAR-NET-14 的采用方式定了再扩。
CREATE TABLE network_routing.service_calendar_version (
    tenant_id      text        NOT NULL,
    target_kind    text        NOT NULL,
    target_code    text        NOT NULL,
    version        integer     NOT NULL,

    effective_from timestamptz NOT NULL,
    effective_to   timestamptz,

    CONSTRAINT service_calendar_version_pkey
        PRIMARY KEY (tenant_id, target_kind, target_code, version),

    -- 适用对象封闭三类（CONTEXT：节点、网络连接或线路）。
    CONSTRAINT service_calendar_version_target_closed
        CHECK (target_kind IN ('NODE', 'CONNECTION', 'LINE')),

    CONSTRAINT service_calendar_version_positive
        CHECK (version >= 1),
    CONSTRAINT service_calendar_version_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(target_code) <> ''),
    CONSTRAINT service_calendar_version_interval_ordered
        CHECK (effective_to IS NULL OR effective_to > effective_from)
);

CREATE UNIQUE INDEX service_calendar_version_one_open_per_identity
    ON network_routing.service_calendar_version (tenant_id, target_kind, target_code)
    WHERE effective_to IS NULL;

-- 临时网络可用性调整。「针对节点、网络连接或线路形成的临时停运、关闭、恢复或适用
-- 范围调整。调整必须记录来源、范围、生效时间和解除时间，不覆盖稳定网络定义」；
-- 「调整的形成、变化和解除历史必须保留」——同一调整身份的版本是**历史链**（形成→
-- 变化→解除各成一行），当前陈述取最大版本，与稳定六表按区间并行选版的机制不同，
-- 这正是「稳定定义与临时调整必须分离」在选版机制上的那一半。
CREATE TABLE network_routing.availability_adjustment (
    tenant_id       text        NOT NULL,
    adjustment_code text        NOT NULL,
    version         integer     NOT NULL,

    target_kind     text        NOT NULL,
    target_code     text        NOT NULL,
    adjustment_kind text        NOT NULL,
    source          text        NOT NULL,

    effective_at    timestamptz NOT NULL,
    lifted_at       timestamptz,

    CONSTRAINT availability_adjustment_pkey
        PRIMARY KEY (tenant_id, adjustment_code, version),

    CONSTRAINT availability_adjustment_target_closed
        CHECK (target_kind IN ('NODE', 'CONNECTION', 'LINE')),

    -- 调整种类封闭四格（CONTEXT：临时停运、关闭、恢复或适用范围调整）。
    CONSTRAINT availability_adjustment_kind_closed
        CHECK (adjustment_kind IN (
            'SUSPENSION', 'CLOSURE', 'RESUMPTION', 'SCOPE_ADJUSTMENT'
        )),

    CONSTRAINT availability_adjustment_positive
        CHECK (version >= 1),
    CONSTRAINT availability_adjustment_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(adjustment_code) <> ''
            AND btrim(target_code) <> ''
            AND btrim(source) <> ''
        ),
    CONSTRAINT availability_adjustment_lift_ordered
        CHECK (lifted_at IS NULL OR lifted_at > effective_at)
);

-- 路由策略版本。「在明确范围和有效期内……版本化业务规则」「新线路版本和路由策略
-- 版本必须具有明确生效时间和适用范围」。规则正文（过滤、排序、冻结边界、改善阈值、
-- 改路条件与权限）全属 PAR-NET-14，不设内容列。
CREATE TABLE network_routing.route_strategy_version (
    tenant_id        text        NOT NULL,
    strategy_code    text        NOT NULL,
    version          integer     NOT NULL,

    applicable_scope text        NOT NULL,

    effective_from   timestamptz NOT NULL,
    effective_to     timestamptz,

    CONSTRAINT route_strategy_version_pkey
        PRIMARY KEY (tenant_id, strategy_code, version),

    CONSTRAINT route_strategy_version_positive
        CHECK (version >= 1),
    CONSTRAINT route_strategy_version_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(strategy_code) <> ''
            AND btrim(applicable_scope) <> ''
        ),
    CONSTRAINT route_strategy_version_interval_ordered
        CHECK (effective_to IS NULL OR effective_to > effective_from)
);

CREATE UNIQUE INDEX route_strategy_version_one_open_per_identity
    ON network_routing.route_strategy_version (tenant_id, strategy_code)
    WHERE effective_to IS NULL;

-- 目录修订锚。CONTEXT 要求每次判断保留「当前修订标识」，ADR-0052 裁定「修订标识由
-- 登记册派生，不由调用方传入；同一次取回的全部事实族共用同一个修订」。本表是派生的
-- 产地：每一笔目录写入在**同一事务**里把本行 +1，读口用单条语句把七类适用行与本行
-- 一并取回——单语句单快照，事实与修订必然同版（票 01 件④）。
--
-- 「登记过与否」也立在本行上：行不存在即这个租户从未登记过任何网络定义（未配置），
-- 行存在而某时点无适用版本是「登记过、该时点无定义」——ADR-0052 明令这两者不能靠
-- 「查出零行」推断，本行就是那个分辨器。
CREATE TABLE network_routing.network_catalog_revision (
    tenant_id text   NOT NULL,
    revision  bigint NOT NULL,

    CONSTRAINT network_catalog_revision_pkey PRIMARY KEY (tenant_id),

    CONSTRAINT network_catalog_revision_positive
        CHECK (revision >= 1),
    CONSTRAINT network_catalog_revision_not_blank
        CHECK (btrim(tenant_id) <> '')
);
