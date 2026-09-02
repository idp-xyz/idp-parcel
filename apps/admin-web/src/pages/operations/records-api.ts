// 作业与履约两张治理查阅页的读面（GET /node-operations-records、
// GET /transport-fulfillment-records，票 admin-skeleton-closure-batch/05）。
//
// 形状以 internal/nodeoperations 与 internal/transportfulfillment 两包
// adapters/http/query_*_records.go 为准，此处只做镜像不虚构：七种册子行形状互不
// 相同，registry 是传输形状由调用方给定；响应外壳的 outcome 词与列表键已把册子
// 分开，无需回显 registry。传输与五格判读收敛在共享 catalogue-api。
//
// 页面五区里没有对应册子的区（节点侧的实际测量与交接证据、运输侧的承运总单与
// 运输舱单）在这里也没有函数——服务端的册名封闭集刻意不含那几格（没有表就没有
// 读法，票 05 Comments），本文件不造会被 400 拒掉的查询。

import { exchangeMasterData, type ApiResult } from '../catalogue-api';

export type { ApiResult } from '../catalogue-api';

// ---- 节点作业三册（node-operations-review 页） ----

/** 册名封闭集，与传输层 ?registry= 分派同词。 */
export type NodeOperationsRegistry =
  | 'reception'
  | 'unidentified-item'
  | 'consolidation-unit';

/**
 * 一行收寄登记。kind 是收寄判断的封闭词（INTAKE_FORMED / PENDING_IDENTIFICATION），
 * 词表在 presentation.ts；released 两键成对缺席表示实物仍在节点控制中——转出只能
 * 由权威交接引用建立，读面不代判「在库/出库」一类派生状态词。
 */
export interface ReceptionRecord {
  sourceId: string;
  kind: string;
  unit: string;
  node: string;
  deliveredBy: string;
  receivedAt: string;
  controlKind: string;
  controlEstablishedAt: string;
  controlReleasedBy?: string;
  controlReleasedAt?: string;
  serviceMarkers: string[];
  recordedAt: string;
}

/**
 * 一行待识别实物登记。association 缺席说的是「登记那一刻还没有正式关联」，不是
 * 「至今没有」——身份确认由 parcel-shipment 以版本化关联形成，识别成功不回写本行。
 */
export interface UnidentifiedItemRecord {
  sourceId: string;
  unit: string;
  node: string;
  candidates: string[];
  identityConflict: boolean;
  receivedAt: string;
  association?: string;
  recordedAt: string;
}

/**
 * 一个集运单元实例。phase 三相封闭词（OPEN / SEALED / CLOSED）；memberCount 是
 * 当前成员数不是成员清单；latestSeal 两键成对缺席即从未封装过，sealCount 把
 * 「从未封装」与「重新封装过」分开。
 *
 * openedSourceId 与 openedBy 是开启那一次作业的来源与执行方，三相上恒在场；
 * latestSealSourceId 与 latestSealPerformedBy 是最近一次封装的同两件，与
 * latestSeal 同缺同在。来源身份要与执行方一起显示，因为**分辨一线过渡期导入
 * 进来的事实与设备扫描的事实靠的是它**——执行方在两条路上可以是同一个人。
 */
export interface ConsolidationUnitRecord {
  unitId: string;
  asset: string;
  phase: string;
  memberCount: number;
  sealCount: number;
  openedSourceId: string;
  openedBy: string;
  latestSeal?: string;
  latestSealSourceId?: string;
  latestSealPerformedBy?: string;
  latestSealedAt?: string;
  closedAt?: string;
}

export type NodeOperationsListResponseBody =
  | { outcome: 'RECEPTIONS_LISTED'; receptions: ReceptionRecord[] }
  | { outcome: 'UNIDENTIFIED_ITEMS_LISTED'; items: UnidentifiedItemRecord[] }
  | { outcome: 'CONSOLIDATION_UNITS_LISTED'; units: ConsolidationUnitRecord[] };

interface NodeBodyByRegistry {
  reception: Extract<NodeOperationsListResponseBody, { outcome: 'RECEPTIONS_LISTED' }>;
  'unidentified-item': Extract<
    NodeOperationsListResponseBody,
    { outcome: 'UNIDENTIFIED_ITEMS_LISTED' }
  >;
  'consolidation-unit': Extract<
    NodeOperationsListResponseBody,
    { outcome: 'CONSOLIDATION_UNITS_LISTED' }
  >;
}

/** 返回类型按请求的 registry 收窄：三册行形状互不相同，页面按区择形状。 */
export function listNodeOperationsRecords<Registry extends NodeOperationsRegistry>(
  registry: Registry,
): Promise<ApiResult<NodeBodyByRegistry[Registry]>> {
  return exchangeMasterData<NodeBodyByRegistry[Registry]>(
    `/node-operations-records?registry=${encodeURIComponent(registry)}`,
  );
}

// ---- 运输履约四册（transport-fulfillment-review 页） ----

export type TransportFulfillmentRegistry =
  | 'transport-schedule'
  | 'capacity-pool'
  | 'transport-handover'
  | 'effective-delivery';

/**
 * 一个具体班次。没有执行准备键、也没有实际执行键——那两组在存储上没有登记格
 * （班次行只登身份、方向与出发时刻），缺席说的是「无处可登」，页面不代填
 * 「未出发」一类派生状态词。
 */
export interface TransportScheduleRecord {
  scheduleId: string;
  direction: string;
  departsAt: string;
  recordedAt: string;
}

/**
 * 一个容量池。四量各自照实转写为十进制计数串（服务端防 2^53 取整），不互相
 * 抵扣——可用量是领域按时点算的判断，读面不代算；单位随维度引用给出，不同池
 * 之间不换算。
 */
export interface CapacityPoolRecord {
  poolId: string;
  schedule: string;
  unit: string;
  capacity: string;
  reserved: string;
  released: string;
  consumed: string;
  recordedAt: string;
}

/**
 * 一个权威交接判断版本。verdict 三值封闭词（HANDED_OVER / REFUSED /
 * PENDING_CONFIRMATION），词表在 presentation.ts；basis 只在拒收与待确认上在场；
 * corrects 两键成对缺席即首登版本——一行一版本，更正是新行指回前版。
 */
export interface TransportHandoverRecord {
  object: string;
  scope: string;
  version: string;
  releasedBy: string;
  receivedBy: string;
  verdict: string;
  basis?: string;
  correctsVersion?: string;
  correctedAt?: string;
  judgedAt: string;
  recordedAt: string;
}

/**
 * 一行当前版有效交付结果。proof 是交付证明引用不是证据内容——「已签收」不是可
 * 直接修改的状态，更正走新判断版本（corrects 两键在场即更正版）。
 */
export interface EffectiveDeliveryRecord {
  object: string;
  attempt: string;
  version: string;
  place: string;
  method: string;
  recipient: string;
  proof: string;
  correctsVersion?: string;
  correctedAt?: string;
  occurredAt: string;
  recordedAt: string;
}

export type TransportFulfillmentListResponseBody =
  | { outcome: 'TRANSPORT_SCHEDULES_LISTED'; schedules: TransportScheduleRecord[] }
  | { outcome: 'CAPACITY_POOLS_LISTED'; pools: CapacityPoolRecord[] }
  | { outcome: 'TRANSPORT_HANDOVERS_LISTED'; handovers: TransportHandoverRecord[] }
  | { outcome: 'EFFECTIVE_DELIVERIES_LISTED'; deliveries: EffectiveDeliveryRecord[] };

interface TransportBodyByRegistry {
  'transport-schedule': Extract<
    TransportFulfillmentListResponseBody,
    { outcome: 'TRANSPORT_SCHEDULES_LISTED' }
  >;
  'capacity-pool': Extract<
    TransportFulfillmentListResponseBody,
    { outcome: 'CAPACITY_POOLS_LISTED' }
  >;
  'transport-handover': Extract<
    TransportFulfillmentListResponseBody,
    { outcome: 'TRANSPORT_HANDOVERS_LISTED' }
  >;
  'effective-delivery': Extract<
    TransportFulfillmentListResponseBody,
    { outcome: 'EFFECTIVE_DELIVERIES_LISTED' }
  >;
}

/** 返回类型按请求的 registry 收窄，同 listNodeOperationsRecords。 */
export function listTransportFulfillmentRecords<
  Registry extends TransportFulfillmentRegistry,
>(registry: Registry): Promise<ApiResult<TransportBodyByRegistry[Registry]>> {
  return exchangeMasterData<TransportBodyByRegistry[Registry]>(
    `/transport-fulfillment-records?registry=${encodeURIComponent(registry)}`,
  );
}
