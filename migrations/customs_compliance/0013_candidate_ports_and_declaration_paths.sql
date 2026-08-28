-- 口岸目录与申报路径目录两本登记册（票 admin-remainder-mechanism-batch/03）。所有权取
-- CONTEXT 所有权句：本上下文拥有合规候选区域、口岸、申报路径和关务适用性判断；
-- network-routing 只在合格候选中选择。此前关务迁移无口岸/申报路径表（08-25 复核改判
-- 建模先行的那个断点），「口岸与申报路径」页因此无表可读——本迁移补的就是这两张。
--
-- 版本维形状照 0011 解释规则先例：起点随登记给出并入键；终点不是登记输入——它在后继
-- 版本登记时落定（换版），NULL 即「尚无终点」。同键区间不重叠由排他约束交付，一个
-- 评估时点至多解析出一版。btree_gist 已由 0011 安装（同模块迁移按序号先后施加）。
--
-- 目录只登事实，不做路由决策（NR 的事）、不做案件判断（案件链已有归属）。真实口岸、
-- 路径与申报模式全部属实例半边（PAR-NET-02 / PAR-CUS-01 待提供），本迁移不含任何
-- 实例默认值；隔离演示走 SYN- 前缀合成种子（ADR-0078）。

-- 合规候选口岸：一行即「该口岸在该生效区间内是本租户的合规候选」。目录事实只有
-- 键与区间——口岸的物理属性、所属区域与适用性判断都不在本册（区域维未建模，等
-- 自己的票；适用性判断是判断链的产物，不是目录事实）。
CREATE TABLE customs_compliance.candidate_port (
    tenant_id     text        NOT NULL,
    port_ref      text        NOT NULL,
    applies_from  timestamptz NOT NULL,
    applies_until timestamptz,

    CONSTRAINT candidate_port_pkey
        PRIMARY KEY (tenant_id, port_ref, applies_from),

    CONSTRAINT candidate_port_not_blank
        CHECK (btrim(tenant_id) <> '' AND btrim(port_ref) <> ''),

    CONSTRAINT candidate_port_interval_ordered
        CHECK (applies_until IS NULL OR applies_until > applies_from),

    -- 同（租户，口岸）下生效区间不重叠：tstzrange 默认半开 [)，与读口一致；上界
    -- NULL 即无界，开放版与任何更晚起点的登记天然相斥——换版必须先由写口给前版
    -- 落终点（判据同 0011）。
    CONSTRAINT candidate_port_no_overlap
        EXCLUDE USING gist (
            tenant_id WITH =,
            port_ref WITH =,
            tstzrange(applies_from, applies_until) WITH &&
        )
);

-- 申报路径：一行即「该路径在该生效区间内经此口岸、按此方向、以此申报模式申报」。
-- port_ref 是标识引用，刻意不设外键：口岸册按区间版本化，(tenant_id, port_ref)
-- 不是它的键，外键立不住；且「路径引用的口岸此刻是否在册」是读侧核对的判断，
-- 不是目录行的登记前提——裁量记在票面（03）。
CREATE TABLE customs_compliance.declaration_path (
    tenant_id        text        NOT NULL,
    path_ref         text        NOT NULL,
    applies_from     timestamptz NOT NULL,
    applies_until    timestamptz,

    port_ref         text        NOT NULL,
    direction        text        NOT NULL,
    -- 申报模式是引用不是封闭词表：真实模式集属监管规则实例半边，CHECK 里枚举任何
    -- 具体模式都是替租户拟默认值。
    declaration_mode text        NOT NULL,

    CONSTRAINT declaration_path_pkey
        PRIMARY KEY (tenant_id, path_ref, applies_from),

    CONSTRAINT declaration_path_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(path_ref) <> ''
            AND btrim(port_ref) <> ''
            AND btrim(declaration_mode) <> ''
        ),
    -- 进出口方向封闭二值，与 domain.ManifestDirection / 0009 舱单方向同词。
    CONSTRAINT declaration_path_direction_closed
        CHECK (direction IN ('IMPORT', 'EXPORT')),

    CONSTRAINT declaration_path_interval_ordered
        CHECK (applies_until IS NULL OR applies_until > applies_from),

    CONSTRAINT declaration_path_no_overlap
        EXCLUDE USING gist (
            tenant_id WITH =,
            path_ref WITH =,
            tstzrange(applies_from, applies_until) WITH &&
        )
);
