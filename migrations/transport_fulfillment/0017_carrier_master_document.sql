-- 总单登记册（ADR-0113；票 tf-carrier-master-document-register/01）。
--
-- CONTEXT「总单」：运营企业与外部运输服务提供方之间针对明确运输范围形成的主运输凭证；身份由登记方以
-- 总单引用声明并带版本，签发方引 party-commercial 身份，关联的集运单元 / 包裹 / 实际履约段逐条登记，
-- 撤销与替代形成新版本回指前版，替代必须指名替代它的那份总单。规则节：总单、运输舱单和外部承运凭证
-- 具有不同业务身份，不能合并为一张可覆盖运输单——所以这是一张自己的表，不是 0012 凭证表上加一格。
--
-- **主键带版本，写入只插不改。** 撤销、替代、关联重述各形成一个新版本回指前版；原版本一字不动。
-- 「当前版」按回指派生（没有任何行回指它），表上没有 current 列——形照 0012 与 0016。
--
-- **总单引用是登记方声明的身份，本表不解析它。** 它通常就是签发方给的总单号，但号码格式、号段与校验
-- 属实例半边，这里一列 text 照存；仓内不写任何真实总单号。
CREATE TABLE transport_fulfillment.carrier_master_document (
    tenant_id            text        NOT NULL,
    master_document_ref  text        NOT NULL,
    version              text        NOT NULL,

    -- 签发方：外部运输服务提供方，引 party-commercial 的参与方身份，本上下文不铸。
    issuer_ref           text        NOT NULL,
    -- 主运输凭证范围：登记方声明的范围引用，不从线路或计划履约段推导。
    scope_ref            text        NOT NULL,
    -- CONTEXT「总单可以引用运输委托或订舱关系」——是「可以」，两列都可缺。
    commission_ref       text,
    booking_ref          text,

    standing             text        NOT NULL,
    -- 本版本改变前版的业务时间（撤销 / 替代 / 关联重述），首版没有。与 recorded_at 是两个时间。
    changed_at           timestamptz,
    supersedes_version   text,
    replaced_by_document text,

    recorded_at          timestamptz NOT NULL,

    CONSTRAINT carrier_master_document_pkey
        PRIMARY KEY (tenant_id, master_document_ref, version),

    CONSTRAINT carrier_master_document_scope_not_blank
        CHECK (
            btrim(tenant_id) <> ''
            AND btrim(master_document_ref) <> ''
            AND btrim(version) <> ''
            AND btrim(issuer_ref) <> ''
            AND btrim(scope_ref) <> ''
        ),

    CONSTRAINT carrier_master_document_optional_refs_not_blank
        CHECK (
            (commission_ref IS NULL OR btrim(commission_ref) <> '')
            AND (booking_ref IS NULL OR btrim(booking_ref) <> '')
        ),

    CONSTRAINT carrier_master_document_standing_closed
        CHECK (standing IN ('IN_FORCE', 'REVOKED', 'SUPERSEDED')),

    -- 首版：不回指、无改变时间；后续版本：两者都有。关联重述也是后续版本，所以这里不看 standing。
    CONSTRAINT carrier_master_document_root_or_change
        CHECK ((supersedes_version IS NULL) = (changed_at IS NULL)),

    -- 一份总单不会一出生就是已撤销或已替代：撤销与替代都是对前版的改变。
    CONSTRAINT carrier_master_document_root_is_in_force
        CHECK (supersedes_version IS NOT NULL OR standing = 'IN_FORCE'),

    -- 替代者只在已替代时出现，且已替代必有替代者；替代者不能是自己。
    CONSTRAINT carrier_master_document_replacement_matches_standing
        CHECK ((standing = 'SUPERSEDED') = (replaced_by_document IS NOT NULL)),

    CONSTRAINT carrier_master_document_replacement_not_self
        CHECK (replaced_by_document IS NULL OR replaced_by_document <> master_document_ref),

    CONSTRAINT carrier_master_document_replacement_not_blank
        CHECK (replaced_by_document IS NULL OR btrim(replaced_by_document) <> ''),

    -- 前版引用不空白、不指向自己。
    CONSTRAINT carrier_master_document_supersedes_coherent
        CHECK (
            supersedes_version IS NULL
            OR (btrim(supersedes_version) <> '' AND supersedes_version <> version)
        ),

    -- 自引用外键：回指的必须是**同租户同总单**下登过的一版——链不悬空、不跨总单。首版 NULL 不受检。
    CONSTRAINT carrier_master_document_supersedes_fkey
        FOREIGN KEY (tenant_id, master_document_ref, supersedes_version)
        REFERENCES transport_fulfillment.carrier_master_document (tenant_id, master_document_ref, version)
);

-- 两道部分唯一索引让链严格线性，FindCurrent 才答得出唯一的当前版：一份总单只有一个首版（编排对第二个
-- 首版答`已有版本链`，这里是它的第二道门）；一版最多被回指一次（并发两次改同一前版，后到的撞它）。
CREATE UNIQUE INDEX carrier_master_document_one_root_per_document
    ON transport_fulfillment.carrier_master_document (tenant_id, master_document_ref)
    WHERE supersedes_version IS NULL;

CREATE UNIQUE INDEX carrier_master_document_supersedes_once
    ON transport_fulfillment.carrier_master_document (tenant_id, master_document_ref, supersedes_version)
    WHERE supersedes_version IS NOT NULL;

-- 版本表按登记先后列全部版本；查阅页按租户上列。
CREATE INDEX carrier_master_document_by_document
    ON transport_fulfillment.carrier_master_document (tenant_id, master_document_ref, recorded_at);

-- 关联子表：GLOSSARY「一个总单可以关联一个或多个集运单元、包裹或运输履约范围，但关联必须明确且可追溯」。
-- 关联挂在形成它的那个版本上（复合外键指向版本行）；关联重述就是带另一组关联的新版本。
--
-- 不对被关联对象设外键：集运单元、包裹、实际履约段分属 node-operations / parcel-shipment / 本上下文
-- 三本册，且关联是历史事实——对象后来被拆并、段后来关闭，都不改「它曾列入这一版总单」。
-- 「列入总单」不证明装载、交接或运输（CONTEXT 硬句），本表不带任何控制或装载语义的列。
CREATE TABLE transport_fulfillment.carrier_master_document_association (
    tenant_id           text NOT NULL,
    master_document_ref text NOT NULL,
    version             text NOT NULL,
    associated_kind     text NOT NULL,
    associated_ref      text NOT NULL,

    CONSTRAINT carrier_master_document_association_pkey
        PRIMARY KEY (tenant_id, master_document_ref, version, associated_kind, associated_ref),

    CONSTRAINT carrier_master_document_association_version_fkey
        FOREIGN KEY (tenant_id, master_document_ref, version)
        REFERENCES transport_fulfillment.carrier_master_document (tenant_id, master_document_ref, version),

    CONSTRAINT carrier_master_document_association_kind_closed
        CHECK (associated_kind IN ('CONSOLIDATION_UNIT', 'PARCEL', 'FULFILLMENT_SEGMENT')),

    CONSTRAINT carrier_master_document_association_ref_not_blank
        CHECK (btrim(associated_ref) <> '')
);
