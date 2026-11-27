import { postMasterData, type ApiResult } from '../catalogue-api';
import type { RegistrationResponseBody } from '../../components/registration';
import { releaseExceptionsText } from './wave-rule';

/** 仓配其余各页写口的统一答复译法。把 reason、field、missing、status、documentNumber、occurrenceId 与 issues 折成登记面板能示出的原因。 */
export interface WarehouseActionBody {
  outcome: string;
  refusal?: string;
  reason?: string;
  field?: string;
  status?: string;
  missing?: string;
  documentNumber?: string;
  occurrenceId?: string;
  version?: number;
  issues?: ReadonlyArray<{ side?: string; line?: number; carton?: number; kind?: string }>;
  /** 放波次勾了「放行能放的」时，点名了而没放进波次的出库请求和缘由。 */
  exceptions?: ReadonlyArray<{ requestId: string; reason: string }>;
}

export function asWarehouseAction(body: WarehouseActionBody): RegistrationResponseBody {
  const parts: string[] = [];
  if (body.reason) parts.push(`理由：${body.reason}`);
  if (body.field) parts.push(`指出的格：${body.field}`);
  if (body.missing) parts.push(`缺少：${body.missing}`);
  if (body.status) parts.push(`当前状态：${body.status}`);
  if (body.documentNumber) parts.push(`单据号：${body.documentNumber}`);
  if (body.occurrenceId) {
    parts.push(body.version ? `发生项：${body.occurrenceId}（第 ${body.version} 版）` : `发生项：${body.occurrenceId}`);
  }
  for (const issue of body.issues ?? []) {
    const place =
      issue.side !== undefined
        ? `${issue.side} 第 ${issue.line ?? 0} 行`
        : issue.carton !== undefined && issue.carton > 0
          ? `第 ${issue.carton} 箱`
          : issue.line !== undefined
            ? `第 ${issue.line} 行`
            : '条目';
    parts.push(`${place}：${issue.kind ?? '不合'}`);
  }
  const skipped = releaseExceptionsText(body.exceptions);
  if (skipped !== '') parts.push(skipped);
  return {
    outcome: body.outcome,
    ...(body.refusal ? { refusalReason: body.refusal } : {}),
    ...(parts.length > 0 ? { cause: parts.join('；') } : {}),
  };
}

export async function postWarehouseAction(
  path: string,
  snapshot: unknown,
): Promise<ApiResult<RegistrationResponseBody>> {
  const answer = await postMasterData<WarehouseActionBody>(path, snapshot);
  if (answer.kind !== 'outcome') return answer;
  return { kind: 'outcome', status: answer.status, body: asWarehouseAction(answer.body) };
}

export interface WarehouseActionSpec {
  id: string;
  label: string;
  title: string;
  endpoint: string;
  /** 'registry' 读登记册配置写授予；其余字符串读对应运营决定授予。 */
  grant: 'registry' | string;
  snapshotHint: string;
}

const noActorNote = ' 租户与操作者由系统按登录身份记下，报文里不要填。';

export const warehouseActionOutcomeLabels: Record<string, string> = {
  REGISTERED: '已登记',
  ALREADY_REGISTERED: '已经在册，内容相同，没有再写一次',
  RECORDED: '已记录',
  ACCEPTED: '已受理',
  EXISTING_RESULT: '已有结果，没有重复写入',
  CONTENT_CONFLICT: '同一标识下内容不同，没有覆盖',
  SOURCE_CONFLICT: '同一标识下内容不同，没有覆盖',
  CONTENT_INVALID: '内容不合，没有落库',
  NOT_ACCEPTED: '没有受理',
  NOT_CONFIGURED: '未配置：所需的租户取值或规则还没登记',
  NOT_FOUND: '查无此记录',
  WAREHOUSE_NOT_FOUND: '这个仓还没登记',
  WAREHOUSE_NOT_ACCEPTING: '这个仓不受理这一操作',
  STATUS_MISMATCH: '当前状态不允许这一操作',
  CLOSED: '已完结',
  CLAIMED: '已认领',
  ALREADY_CLAIMED: '已经被认领',
  CLAIM_REVOKED: '已撤销认领',
  REVOCATION_NOT_ALLOWED: '这一认领已被入账引用，不能撤销',
  DISPOSED: '已处置',
  DISPOSITION_NOT_ALLOWED: '期限未到或方式不在处置规则里',
  ARRIVAL_NOT_FOUND: '没有这件待认领到货',
  CONCLUDED: '已记下差异结论',
  CONCLUSION_HELD: '已有货主结论，代录不覆盖',
  DISCREPANCY_NOT_FOUND: '没有这份差异',
  RESTORED: '已转为良品',
  VOIDED: '已作废',
  FROZEN: '已冻结',
  UNFROZEN: '已解冻',
  CREATED: '已创建盘点',
  STARTED: '已开始盘点',
  CANNOT_START: '盘点开始不了：范围里的货被别的盘点占着，或锁住它会挤占已有的预占、冻结',
  COMPLETED: '已完成盘点',
  POSTED: '已入账',
  APPROVED: '已批准',
  REJECTED: '已驳回',
  DUTY_BLOCKED: '提交人不能批准自己这一份',
  NOT_VALID: '内容已不再成立，没有入账',
  RESERVED: '已预占',
  PARTIALLY_RESERVED: '部分预占',
  PENDING_POLICY: '库存不够且未登记缺货处置，停在待预占',
  HANDED_OFF: '代理仓出库请求已受理，待转出',
  SPLIT: '已拆单',
  ALLOCATED: '已放波次并生成拣货任务',
  WITHDRAWN: '已从波次撤出，出库单回到已预占',
  ASSIGNED: '已指派：这个波次归这位拣货人拣',
  UNASSIGNED: '已取消指派：这个波次谁都可以拣、可以领',
  WAVE_ASSIGNED_TO_OTHER: '这个波次已经派给了别人',
  WAVE_RULE_NOT_FOUND: '这个仓没有登记过这条成波规则',
  WAVE_RULE_DEACTIVATED: '这条成波规则已经停用',
  NOTHING_TO_RELEASE: '没有可放的出库单，一个波次也没放出',
  TOTES_REGISTERED: '已登记：这一批里新的箱号已在册，停用过的已恢复在用',
  TOTE_RETIRED: '已停用，拣货不再认这个箱',
  TOTE_NOT_FOUND: '这个仓没有登记过这个箱号',
  TOTE_IN_USE: '这个箱上还绑着出库单，货还在里面',
  TOTE_RELEASED: '已放开，这个箱可以绑别的出库单了',
  TOTE_NOT_ACCEPTING: '这个箱已停用，不能再往里放货',
  TOTE_REQUIRED: '这个仓拣货必须用周转箱，这一笔没带箱号，没有记',
  SHORT_SHIPPED: '已按已拣数量发货：没拣到的预占放开，出库单进已拣货，往下按已拣数量复核打包',
  REMAINDER_SPLIT: '已拆出缺货部分：原单进已拣货、按已拣数量往下走，缺的另成一张出库单，在原单右栏能点过去',
  PARTIAL_SHIPMENT_NOT_CONFIGURED: '这张单没写明拣不齐时怎么发，货主也没登记默认，不能少发，也不能拆出缺货部分',
  PARTIAL_SHIPMENT_NOT_ALLOWED: '货主对这张单许的不是这一种发法，没有办',
  PREFETCH_STANDS: '这张单是早先预取的面单，当时没留下申报了几件，打包时更正不了委托上的件数，所以不办；等有货了再放波次，或截单',
  // 这个词几个口都答，指的东西不一样：撤单是没有这个波次，放波次是点名的出库单找不到或不在这个仓，
  // 更正出库确认是没有这条确认。所以只说对不上，不说是哪一样。
  WAVE_NOT_FOUND: '查无此记录：波次、出库单或确认事实对不上',
  STRATEGY_REGISTERED: '已采用参考分配策略',
  POLICY_REGISTERED: '已登记缺货处置',
  CANCELLED: '已截单取消',
  RETURN_REQUIRED: '已截单，待把已拣货移回库位',
  REFUSED: '委托未放行，截单不成立',
  FORMED: '已形成',
  FORWARDED: '已转出给代理仓',
  PACKED: '代理仓已报打包',
  CONFIRMED: '已确认',
  VALID: '有效',
  UNKNOWN: '已过期或未知',
  SUPERSEDED: '已有更新声明',
  RETURN_ACCEPTED: '已受理退货预报',
  DISPOSED_RETURN: '已记下回库处置',
  CONFLICT: '同一版本冲突，没有覆盖',
};

/** 收货与待认领页的写动作。 */
export const receivingActionSpecs: readonly WarehouseActionSpec[] = [
  {
    id: 'pre-advice-on-behalf',
    label: '代录入库预报',
    title: '代录货主入库预报',
    endpoint: '/warehouse-fulfillment-inbound-pre-advice-on-behalf',
    grant: 'INBOUND_PRE_ADVICE_RECORD_ON_BEHALF',
    snapshotHint:
      '填 ownerAccount、decisionId、evidenceReference、warehouseCode、arrivalMode（FULL_CONTAINER / PALLET / LOOSE_CARTON）、inboundMode（BY_GOODS / BY_CARTON）、expectedAt（RFC 3339），按入库方式填 goodsLines 或 cartons。' +
      noActorNote,
  },
  {
    id: 'pre-advice-notice',
    label: '代录预报告知',
    title: '代录更新入库预报告知',
    endpoint: '/warehouse-fulfillment-inbound-pre-advice-notice-on-behalf',
    grant: 'INBOUND_PRE_ADVICE_RECORD_ON_BEHALF',
    snapshotHint:
      '填 preAdviceId、decisionId、evidenceReference、expectedAt（RFC 3339），可选 arrivalClue、ownerReference。货主取自预报，报文里不要带 ownerAccount。' +
      noActorNote,
  },
  {
    id: 'pre-advice-withdrawal',
    label: '代录撤回预报',
    title: '代录撤回入库预报',
    endpoint: '/warehouse-fulfillment-inbound-pre-advice-withdrawal-on-behalf',
    grant: 'INBOUND_PRE_ADVICE_RECORD_ON_BEHALF',
    snapshotHint:
      '填 preAdviceId 与 evidenceReference。不要带 ownerAccount 或 decisionId；收货中与已完结的预报不能撤回。' +
      noActorNote,
  },
  {
    id: 'pre-advice-close',
    label: '完结预报',
    title: '完结入库预报并计算少收',
    endpoint: '/warehouse-fulfillment-inbound-pre-advice-closures',
    grant: 'INBOUND_PRE_ADVICE_CLOSE',
    snapshotHint: '填 preAdviceId，可选 note。不要带 ownerAccount 或 decisionId；只完结收货中的预报。' + noActorNote,
  },
  {
    id: 'discrepancy-conclusion',
    label: '代录收货差异结论',
    title: '代录收货差异结论',
    endpoint: '/warehouse-fulfillment-receipt-discrepancy-conclusion-on-behalf',
    grant: 'RECEIPT_DISCREPANCY_RECORD_ON_BEHALF',
    snapshotHint: '填 discrepancyId、conclusion、evidenceReference，可选 note。不要带 ownerAccount 或 decisionId。' + noActorNote,
  },
  {
    id: 'unclaimed-claim',
    label: '认领待认领',
    title: '认领待认领到货',
    endpoint: '/warehouse-fulfillment-unclaimed-arrival-claims',
    grant: 'UNCLAIMED_ARRIVAL_CLAIM',
    snapshotHint: '填 decisionId、arrivalId、ownerAccount；入库到货需带 preAdviceId，退件只带货主。' + noActorNote,
  },
  {
    id: 'unclaimed-revoke',
    label: '撤销认领',
    title: '撤销待认领到货认领',
    endpoint: '/warehouse-fulfillment-unclaimed-arrival-claim-revocations',
    grant: 'UNCLAIMED_ARRIVAL_CLAIM',
    snapshotHint: '填 decisionId、arrivalId，可选 reason。已被入账引用的认领不能撤销。' + noActorNote,
  },
  {
    id: 'unclaimed-dispose',
    label: '处置待认领',
    title: '按规则处置待认领到货',
    endpoint: '/warehouse-fulfillment-unclaimed-arrival-dispositions',
    grant: 'UNCLAIMED_ARRIVAL_DISPOSITION',
    snapshotHint: '填 arrivalId、method，可选 note。该仓须已登记处置规则且超过认领期限。' + noActorNote,
  },
  {
    id: 'disposal-rule',
    label: '登记处置规则',
    title: '登记待认领处置规则修订',
    endpoint: '/warehouse-fulfillment-unclaimed-disposal-rule-registrations',
    grant: 'registry',
    snapshotHint: '填 warehouseCode、revision（从 1 起连续）、claimWindowSeconds（正整数秒）、disposalMethods（方式数组）。' + noActorNote,
  },
  {
    id: 'goods-on-behalf',
    label: '代录商品',
    title: '代录货主商品修订',
    endpoint: '/warehouse-fulfillment-goods-on-behalf',
    grant: 'GOODS_RECORD_ON_BEHALF',
    snapshotHint:
      '填 ownerAccount、decisionId、evidenceReference、goodsCode、name、baseUnit、trackBatch、trackExpiry、trackSerial，改资料再填 basisRevision（当前修订号），以及可选包装层级、别名、组合与申报资料。首版不带 basisRevision。' +
      noActorNote,
  },
  {
    id: 'return-pre-advice',
    label: '代录退货预报',
    title: '代录退货预报',
    endpoint: '/warehouse-fulfillment-return-pre-advices',
    grant: 'RETURN_RECORD_ON_BEHALF',
    snapshotHint: '填 ownerAccount、basis、warehouse、trackingNumber、lines（goodsCode、unit、quantity）。' + noActorNote,
  },
  {
    id: 'return-disposition',
    label: '代录退货处置',
    title: '代录退货回库处置',
    endpoint: '/warehouse-fulfillment-return-dispositions',
    grant: 'RETURN_RECORD_ON_BEHALF',
    snapshotHint: '填 basis、factId、destination（GOOD_PUTAWAY / DEFECTIVE / DESTROY / RETURN_TO_OWNER）。不要带 ownerAccount。' + noActorNote,
  },
];

/** 库内作业与盘点调整页的写动作。 */
export const internalOpsActionSpecs: readonly WarehouseActionSpec[] = [
  {
    id: 'condition-restore',
    label: '次品转良品',
    title: '次品转良品',
    endpoint: '/warehouse-fulfillment-condition-restores',
    grant: 'CONDITION_RESTORE',
    snapshotHint:
      '填 decisionId、warehouse、ownerAccount、locationCode（收货或退货暂存）、goodsCode、unit、stockState、quantity、reason，可选批号、效期、贴标与入库批次。' +
      noActorNote,
  },
  {
    id: 'processing-instruction',
    label: '发起加工指令',
    title: '运营发起加工或拆分指令',
    endpoint: '/warehouse-fulfillment-processing-instructions',
    grant: 'PROCESSING_INSTRUCTION',
    snapshotHint: '填 decisionId、ownerAccount、warehouse、goodsCode、quantity、reason，可选 disassemble（拆分填 true）、goodsRevision、basis。' + noActorNote,
  },
  {
    id: 'processing-void',
    label: '作废加工指令',
    title: '作废加工或拆分指令',
    endpoint: '/warehouse-fulfillment-processing-instruction-voids',
    grant: 'PROCESSING_INSTRUCTION',
    snapshotHint: '填 instructionId 与 reason。已执行的指令不能作废。' + noActorNote,
  },
  {
    id: 'relabel-instruction',
    label: '发起换标指令',
    title: '运营发起商品换标指令',
    endpoint: '/warehouse-fulfillment-relabel-instructions',
    grant: 'RELABEL_INSTRUCTION',
    snapshotHint: '填 decisionId、ownerAccount、warehouse、goodsCode、unit、quantity、toAlias、reason，可选 fromAlias、batchNumber、expiryDate、basis。' + noActorNote,
  },
  {
    id: 'relabel-void',
    label: '作废换标指令',
    title: '作废商品换标指令',
    endpoint: '/warehouse-fulfillment-relabel-instruction-voids',
    grant: 'RELABEL_INSTRUCTION',
    snapshotHint: '填 instructionId 与 reason。' + noActorNote,
  },
  {
    id: 'stock-freeze',
    label: '冻结库存',
    title: '冻结商品格或整箱',
    endpoint: '/warehouse-fulfillment-stock-freezes',
    grant: 'STOCK_FREEZE',
    snapshotHint:
      '填 decisionId、ownerAccount、warehouse、reason；整箱只加 cartonNumber，商品加 locationCode、goodsCode、unit、stockState、inboundBatchId、quantity。' +
      noActorNote,
  },
  {
    id: 'stock-unfreeze',
    label: '解冻库存',
    title: '整份解冻库存',
    endpoint: '/warehouse-fulfillment-stock-unfreezes',
    grant: 'STOCK_FREEZE',
    snapshotHint: '填 freezeId，可选 note。不要带 ownerAccount。' + noActorNote,
  },
  {
    id: 'count-create',
    label: '创建盘点',
    title: '创建盘点任务',
    endpoint: '/warehouse-fulfillment-stock-counts',
    grant: 'STOCK_COUNT',
    snapshotHint: '填 decisionId、warehouse、locations（库位数组），可选 ownerAccount、goods、note。' + noActorNote,
  },
  {
    id: 'count-start',
    label: '开始盘点',
    title: '开始盘点',
    endpoint: '/warehouse-fulfillment-stock-count-starts',
    grant: 'STOCK_COUNT',
    snapshotHint: '填 countId。开始后范围进入作业锁定。' + noActorNote,
  },
  {
    id: 'count-complete',
    label: '完成盘点',
    title: '完成盘点并生成差额',
    endpoint: '/warehouse-fulfillment-stock-count-completions',
    grant: 'STOCK_COUNT',
    snapshotHint: '填 countId。' + noActorNote,
  },
  {
    id: 'count-void',
    label: '作废盘点',
    title: '作废盘点任务',
    endpoint: '/warehouse-fulfillment-stock-count-voids',
    grant: 'STOCK_COUNT',
    snapshotHint: '填 countId，可选 reason。' + noActorNote,
  },
  {
    id: 'count-approve',
    label: '批准盘点差额',
    title: '批准盘点差额入账',
    endpoint: '/warehouse-fulfillment-count-approvals',
    grant: 'STOCK_ADJUSTMENT_APPROVAL',
    snapshotHint: '填 differenceId，可选 note。不要带 ownerAccount；没登记批准职责规则时答未配置。' + noActorNote,
  },
  {
    id: 'count-reject',
    label: '驳回盘点差额',
    title: '驳回盘点差额',
    endpoint: '/warehouse-fulfillment-count-rejections',
    grant: 'STOCK_ADJUSTMENT_APPROVAL',
    snapshotHint: '填 differenceId 与 reason。不要带 ownerAccount 或 note。' + noActorNote,
  },
  {
    id: 'job-discrepancy-approve',
    label: '批准作业差异',
    title: '批准库内作业差异入账',
    endpoint: '/warehouse-fulfillment-job-discrepancy-approvals',
    grant: 'STOCK_ADJUSTMENT_APPROVAL',
    snapshotHint: '填 discrepancyId，可选 note。上架、形态/状态转换、短少与整箱缺箱均走此口；没登记批准职责规则时答未配置。' + noActorNote,
  },
  {
    id: 'job-discrepancy-reject',
    label: '驳回作业差异',
    title: '驳回库内作业差异',
    endpoint: '/warehouse-fulfillment-job-discrepancy-rejections',
    grant: 'STOCK_ADJUSTMENT_APPROVAL',
    snapshotHint: '填 discrepancyId 与 reason。驳回不改账。' + noActorNote,
  },
  {
    id: 'adjustment-submit',
    label: '提交库存调整',
    title: '提交库存调整',
    endpoint: '/warehouse-fulfillment-stock-adjustments',
    grant: 'STOCK_ADJUSTMENT_SUBMISSION',
    snapshotHint:
      '填 adjustmentId、ownerAccount、warehouse、reason、lines（kind 为 INCREASE / DECREASE / DESTRUCTION，含 locationCode、goodsCode、unit、stockState、quantity）。提交不改账。' +
      noActorNote,
  },
  {
    id: 'adjustment-approve',
    label: '批准库存调整',
    title: '批准库存调整入账',
    endpoint: '/warehouse-fulfillment-stock-adjustment-approvals',
    grant: 'STOCK_ADJUSTMENT_APPROVAL',
    snapshotHint: '填 adjustmentId，可选 note。不要带 ownerAccount。' + noActorNote,
  },
  {
    id: 'adjustment-reject',
    label: '驳回库存调整',
    title: '驳回库存调整',
    endpoint: '/warehouse-fulfillment-stock-adjustment-rejections',
    grant: 'STOCK_ADJUSTMENT_APPROVAL',
    snapshotHint: '填 adjustmentId 与 reason。不要带 ownerAccount。' + noActorNote,
  },
  {
    id: 'approval-duty',
    label: '登记批准职责规则',
    title: '登记库存调整批准职责规则',
    endpoint: '/warehouse-fulfillment-approval-duty-rules',
    grant: 'registry',
    snapshotHint: '填 revision（从 1 起连续）与 distinctSubjects（批准人是否须与提交人为不同主体，布尔值）。' + noActorNote,
  },
];

/** 出库、波次与代理仓页的写动作。 */
export const outboundWaveActionSpecs: readonly WarehouseActionSpec[] = [
  {
    id: 'outbound-request',
    label: '代录出库请求',
    title: '代录出库请求',
    endpoint: '/warehouse-fulfillment-outbound-requests',
    grant: 'OUTBOUND_REQUEST_RECORD_ON_BEHALF',
    snapshotHint:
      '填 decisionId、ownerAccount、basis、warehouse、dispatch（PRODUCT_NETWORK / OWNER_LABEL / PICKUP）、recipient、address、lines（goodsCode、unit、stockState、quantity）；本产品网络必填 service，平台备货可选 platformStocking: true；货主对这一张拣不齐时怎么发另有交代时带 partialShipment（SHIP_COMPLETE / SHORT_SHIP / SPLIT_SHIP），没交代就不带。' +
      noActorNote,
  },
  {
    id: 'outbound-split',
    label: '代录拆单',
    title: '代录拆开尚未分配的出库请求',
    endpoint: '/warehouse-fulfillment-outbound-request-splits',
    grant: 'OUTBOUND_REQUEST_RECORD_ON_BEHALF',
    snapshotHint:
      '填 requestId 与 parts（每项含 requestKey、lines）。拆开后原预占释放，新请求各自预占。已有货拣出来的单不能这样拆，它拣不齐时照货主的部分发货规则办。' +
      noActorNote,
  },
  {
    id: 'outbound-cutoff',
    label: '代录截单',
    title: '代录截单取消出库请求或单个包裹',
    endpoint: '/warehouse-fulfillment-outbound-cancellations',
    grant: 'OUTBOUND_REQUEST_RECORD_ON_BEHALF',
    snapshotHint: '填 requestId、basis，按包裹截单时另填 parcelSequence。不要带 ownerAccount。' + noActorNote,
  },
  {
    id: 'partial-shipment-consent',
    label: '代录部分发货同意',
    title: '代录货主对一张出库单拣不齐时怎么发的同意',
    endpoint: '/warehouse-fulfillment-outbound-partial-shipment-consents',
    grant: 'OUTBOUND_REQUEST_RECORD_ON_BEHALF',
    snapshotHint:
      '填 requestId、partialShipment（SHIP_COMPLETE 整单发 / SHORT_SHIP 少发 / SPLIT_SHIP 拆单发）、basis（货主那句话的出处）。' +
      '它盖过这张单提交时写的与货主登记的默认，不动预占与拣货任务。不要带 ownerAccount。' +
      noActorNote,
  },
  {
    id: 'owner-label',
    label: '登记货主面单',
    title: '为自备面单包裹登记面单号',
    endpoint: '/warehouse-fulfillment-owner-labels',
    grant: 'OUTBOUND_REQUEST_RECORD_ON_BEHALF',
    snapshotHint: '填 requestId、parcelSequence、externalNumber。不要带 ownerAccount。' + noActorNote,
  },
  {
    id: 'wave-release',
    label: '放波次',
    title: '释放波次并分配拣货任务',
    endpoint: '/warehouse-fulfillment-waves',
    grant: 'WAVE_RELEASE',
    snapshotHint:
      '填 waveId、warehouse、requests（出库请求标识数组），可选 batchPick（是否合并拣货）、partial（放行能放的，跳过不行的）、prefetch（是否预取估算委托）与 packagingMaterial（预取时选用的包材编码）。未采用分配策略时答未配置。' +
      noActorNote,
  },
  {
    id: 'wave-withdraw',
    label: '撤单',
    title: '把出库单从已放出的波次里撤出',
    endpoint: '/warehouse-fulfillment-wave-withdrawals',
    grant: 'WAVE_RELEASE',
    snapshotHint:
      '填 waveId 与 requests（要撤出的出库请求标识数组）。只撤还没拣过的单：撤出后它在这个波次里的拣货任务作废，出库单回到已预占，可以再放进别的波次；点名的几张里有一张撤不了，就一张都不撤。' +
      noActorNote,
  },
  {
    id: 'short-shipment',
    label: '按已拣数量发货',
    title: '拣不齐的出库单按已拣数量发货',
    endpoint: '/warehouse-fulfillment-outbound-short-shipments',
    grant: 'WAVE_RELEASE',
    snapshotHint:
      '填 requestId。只办停在「拣不齐、等处置」的单：任务都有了结果、已有货拣进待出库、还有没拣到的预占。'
      + '货主许了少发才成立（这张单提交时写的、受理后代录的同意或货主登记的默认）；成立后没拣到的不再发，这一步撤不回。' +
      noActorNote,
  },
  {
    id: 'shortage-split',
    label: '拆出缺货部分',
    title: '拣不齐的出库单拆出缺货部分',
    endpoint: '/warehouse-fulfillment-outbound-shortage-splits',
    grant: 'WAVE_RELEASE',
    snapshotHint:
      '填 requestId。只办停在「拣不齐、等处置」的单，货主许了拆单发才成立。拆多少不用填：没拣到的预占有多少就拆多少，按基本单位另成一张出库单重新受理、重新预占；'
      + '原单进已拣货，按已拣数量往下走。这一步撤不回。' +
      noActorNote,
  },
  {
    id: 'tote-register',
    label: '登记周转箱',
    title: '给一个仓登记一批周转箱',
    endpoint: '/warehouse-fulfillment-tote-registrations',
    grant: 'registry',
    snapshotHint:
      '填 warehouse 与 toteCodes（箱号数组，一次最多 500 个）。已经在册的箱号照旧，停用过的恢复在用。拣货车上的格口也按箱登记。只给自营仓登。' +
      noActorNote,
  },
  {
    id: 'tote-retire',
    label: '停用周转箱',
    title: '停用一个周转箱',
    endpoint: '/warehouse-fulfillment-tote-retirements',
    grant: 'registry',
    snapshotHint: '填 warehouse 与 toteCode。箱上还绑着出库单时不停。停用的箱留在名册上，再登记同一个箱号就恢复在用。' + noActorNote,
  },
  {
    id: 'tote-requirement',
    label: '登记拣货必须用箱',
    title: '登记一个仓拣货是否必须用周转箱',
    endpoint: '/warehouse-fulfillment-tote-requirements',
    grant: 'registry',
    snapshotHint:
      '填 warehouse、revision（从 1 起连续）、required（true 必须用箱 / false 不要求）。登记成必须之后，不带箱号的拣货不成立。没登记的仓带不带箱都收。' +
      noActorNote,
  },
  {
    id: 'wave-assign',
    label: '指派拣货人',
    title: '把一个波次派给一个人拣，或取消指派',
    endpoint: '/warehouse-fulfillment-wave-assignments',
    grant: 'WAVE_RELEASE',
    snapshotHint:
      '填 waveId 与 assignee（拣货人的操作者标识，「发行方#sub」，可从「操作者授予」页或别的波次的拣货人一格抄）；不填 assignee 是取消指派。' +
      '派了之后，别人拣这个波次里的任务后端不收；拣货的人也可以在拣货工位上自己领。已完成、已取消的波次不能派。' +
      noActorNote,
  },
  {
    id: 'wave-rule',
    label: '登记成波规则',
    title: '登记一个仓的一条成波规则',
    endpoint: '/warehouse-fulfillment-wave-rules',
    grant: 'registry',
    snapshotHint:
      '填 warehouse、ruleCode、revision（从 1 起连续，要改或停用就登下一号）、name、maxRequests（一个波次最多几张，必填），' +
      '可选 releaseTimes（每天到点放的时点 HH:MM 数组，不填是只由人点）、maxLines、maxUnits（一个波次最多几行、几件，不填是不限）、ownerAccount、dispatch、profile（SINGLE_UNIT / MULTI_UNIT，不填是不限）、' +
      'batchPick、deactivated。三个圈单条件不填就是不限。' +
      noActorNote,
  },
  {
    id: 'tote-release',
    label: '放开周转箱',
    title: '由人放开一个周转箱',
    endpoint: '/warehouse-fulfillment-tote-releases',
    grant: 'WAVE_RELEASE',
    snapshotHint:
      '填 warehouse 与 toteCode。箱平时由系统放开（出库单打完包，或截单的货都回了库）；箱被拿错、被清空而系统不知道时才由人放。'
      + '它只解开箱与出库单的绑定，不动出库单、拣货任务与库存。' +
      noActorNote,
  },
  {
    id: 'allocation-strategy',
    label: '采用分配策略',
    title: '采用内置参考分配策略',
    endpoint: '/warehouse-fulfillment-allocation-strategy-adoptions',
    grant: 'registry',
    snapshotHint: '填 revision（从 1 起连续）。登记后放波次才会按先到期先出 / 先进先出与拣选优先分配库位。' + noActorNote,
  },
  {
    id: 'shortage-policy',
    label: '登记缺货处置',
    title: '登记货主缺货处置规则',
    endpoint: '/warehouse-fulfillment-shortage-policies',
    grant: 'registry',
    snapshotHint:
      '填 ownerAccount、revision（从 1 起连续），再至少填一件：kind（受理时库存不够怎么办：PARTIAL 部分预占 / REJECT 不足拒单）、' +
      'partialShipment（拣不齐时怎么发的默认：SHIP_COMPLETE 整单发 / SHORT_SHIP 少发 / SPLIT_SHIP 拆单发）。每一版是完整的一份，没填的那件就是没登记。' +
      noActorNote,
  },
  {
    id: 'confirmation-correction',
    label: '更正出库确认',
    title: '更正出库确认时点或判不成立',
    endpoint: '/warehouse-fulfillment-outbound-confirmation-corrections',
    grant: 'OUTBOUND_CONFIRMATION_CORRECTION',
    snapshotHint: '填 factId，二选一：改时点填 occurredAt（RFC 3339），判不成立填 invalidate: true。' + noActorNote,
  },
  {
    id: 'confirmation-replenishment',
    label: '批准确认回补',
    title: '批准判不成立出库确认的库存回补',
    endpoint: '/warehouse-fulfillment-outbound-confirmation-replenishments',
    grant: 'STOCK_ADJUSTMENT_APPROVAL',
    snapshotHint: '填 factId。批准后按原消耗的待出库库位与入库批次调增回补；没登记批准职责规则时答未配置。' + noActorNote,
  },
  {
    id: 'agent-connector',
    label: '登记代理仓连接器',
    title: '登记代理仓采用模拟源',
    endpoint: '/warehouse-fulfillment-agent-connectors',
    grant: 'registry',
    snapshotHint: '填 warehouse（已登记的代理仓编码）。凭据不放进报文。' + noActorNote,
  },
  {
    id: 'agent-statement',
    label: '记代理仓库存声明',
    title: '登记代理仓报来的库存声明',
    endpoint: '/warehouse-fulfillment-agent-stock-statements',
    grant: 'registry',
    snapshotHint: '填 source、statedAt、validUntil（RFC 3339）、warehouse、ownerAccount、goodsCode、unit、quantity。不进自营库存账。' + noActorNote,
  },
  {
    id: 'agent-result',
    label: '转出或记代理仓结果',
    title: '向代理仓转出请求或登记代理仓回传结果',
    endpoint: '/warehouse-fulfillment-agent-outbound-results',
    grant: 'registry',
    snapshotHint:
      '填 requestId 与 kind（FORWARDED 转出 / PACKED 打包 / HANDED_OVER 交接 / REJECTED 拒单 / SHORT 缺货）；非转出时填 resultId，打包另填 weight、weightUnit、length、width、height、dimension。' +
      noActorNote,
  },
];

/** 仓储计价与收费配置页的写动作。 */
export const pricingActionSpecs: readonly WarehouseActionSpec[] = [
  {
    id: 'warehouse-plan',
    label: '登记仓储计费方案',
    title: '登记仓储计费方案',
    endpoint: '/pricing-warehouse-plans',
    grant: 'registry',
    snapshotHint:
      '填 id、version、scope、currency、declaredValue（REFUSE / ALLOW）、effectiveFrom（RFC 3339）、groups 与 rules（单价 unitPriceMinor、基费 baseFeeMinor 为最小货币单位整数）。需部署开启较新规范化开关。' +
      noActorNote,
  },
  {
    id: 'charge-adoption',
    label: '登记仓储计费采用',
    title: '登记货主与仓的仓储计费采用',
    endpoint: '/warehouse-fulfillment-charge-adoptions',
    grant: 'registry',
    snapshotHint:
      '填 ownerAccount、warehouse、revision（从 1 起连续）、effectiveAt（RFC 3339）、account（已登记结算账户）、pricingScope、basis。金额不在这里。' +
      noActorNote,
  },
  {
    id: 'charge-units',
    label: '登记重量体积单位',
    title: '登记仓的收费重量与体积单位',
    endpoint: '/warehouse-fulfillment-charge-measure-units',
    grant: 'registry',
    snapshotHint: '填 warehouse、weightUnit、volumeUnit。没登记时带重量体积的收费发生项不形成。' + noActorNote,
  },
  {
    id: 'charge-aging',
    label: '登记库龄口径',
    title: '登记仓的仓租库龄口径',
    endpoint: '/warehouse-fulfillment-charge-aging-bases',
    grant: 'registry',
    snapshotHint: '填 warehouse 与 basis（ACTUAL_BATCH 按实批 / THEORETICAL_FIFO 按理论先进先出）。' + noActorNote,
  },
  {
    id: 'charge-project',
    label: '登记增值服务项目',
    title: '登记仓的增值服务项目名',
    endpoint: '/warehouse-fulfillment-charge-service-projects',
    grant: 'registry',
    snapshotHint: '填 warehouse 与 name。' + noActorNote,
  },
  {
    id: 'storage-day-close',
    label: '仓租日结',
    title: '按自然日触发一次仓租发生项日结',
    endpoint: '/warehouse-fulfillment-storage-day-closes',
    grant: 'registry',
    snapshotHint: '填 ownerAccount、warehouse、day（YYYY-MM-DD）。仓须已登记业务时区与库龄口径，否则答未配置。' + noActorNote,
  },
];
