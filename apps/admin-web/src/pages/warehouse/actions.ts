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
    snapshotHint: '代货主登一张入库预报。到货方式、入库方式和预计到达要填；按商品入库填商品行，按箱入库填箱。',
  },
  {
    id: 'pre-advice-notice',
    label: '代录预报告知',
    title: '代录更新入库预报告知',
    endpoint: '/warehouse-fulfillment-inbound-pre-advice-notice-on-behalf',
    grant: 'INBOUND_PRE_ADVICE_RECORD_ON_BEHALF',
    snapshotHint: '改这张预报的预计到达。到货线索和货主参考号留空就是清空。',
  },
  {
    id: 'pre-advice-withdrawal',
    label: '代录撤回预报',
    title: '代录撤回入库预报',
    endpoint: '/warehouse-fulfillment-inbound-pre-advice-withdrawal-on-behalf',
    grant: 'INBOUND_PRE_ADVICE_RECORD_ON_BEHALF',
    snapshotHint: '撤回这张还没开始收的预报，写明依据。',
  },
  {
    id: 'pre-advice-close',
    label: '完结预报',
    title: '完结入库预报并计算少收',
    endpoint: '/warehouse-fulfillment-inbound-pre-advice-closures',
    grant: 'INBOUND_PRE_ADVICE_CLOSE',
    snapshotHint: '完结这张收货中的预报，还没收到的按少收记下。说明可以不填。',
  },
  {
    id: 'discrepancy-conclusion',
    label: '代录收货差异结论',
    title: '代录收货差异结论',
    endpoint: '/warehouse-fulfillment-receipt-discrepancy-conclusion-on-behalf',
    grant: 'RECEIPT_DISCREPANCY_RECORD_ON_BEHALF',
    snapshotHint: '给这张收货差异写下结论和依据。说明可以不填。',
  },
  {
    id: 'unclaimed-claim',
    label: '认领待认领',
    title: '认领待认领到货',
    endpoint: '/warehouse-fulfillment-unclaimed-arrival-claims',
    grant: 'UNCLAIMED_ARRIVAL_CLAIM',
    snapshotHint: '入库到货填单据号，页面换成预报标识再送出；手里已经是预报标识也可以。退件留空。',
  },
  {
    id: 'unclaimed-revoke',
    label: '撤销认领',
    title: '撤销待认领到货认领',
    endpoint: '/warehouse-fulfillment-unclaimed-arrival-claim-revocations',
    grant: 'UNCLAIMED_ARRIVAL_CLAIM',
    snapshotHint: '撤销这一笔认领。已经入账的不能撤。原因可以不填。',
  },
  {
    id: 'unclaimed-dispose',
    label: '处置待认领',
    title: '按规则处置待认领到货',
    endpoint: '/warehouse-fulfillment-unclaimed-arrival-dispositions',
    grant: 'UNCLAIMED_ARRIVAL_DISPOSITION',
    snapshotHint: '按这个仓的处置规则处理这一笔超过认领期限的到货。说明可以不填。还没登记规则，或还没过期限，送不出去。',
  },
  {
    id: 'disposal-rule',
    label: '登记处置规则',
    title: '登记待认领处置规则修订',
    endpoint: '/warehouse-fulfillment-unclaimed-disposal-rule-registrations',
    grant: 'registry',
    snapshotHint: '给这个仓登记下一版待认领规则：认领期限按秒计，以及允许的处置方式。修订号从 1 起连续。',
  },
  {
    id: 'goods-on-behalf',
    label: '代录商品',
    title: '代录货主商品修订',
    endpoint: '/warehouse-fulfillment-goods-on-behalf',
    grant: 'GOODS_RECORD_ON_BEHALF',
    snapshotHint: '代货主登记商品。第一次登记不填所依据的修订；改已有商品时填当前修订号。',
  },
  {
    id: 'return-pre-advice',
    label: '代录退货预报',
    title: '代录退货预报',
    endpoint: '/warehouse-fulfillment-return-pre-advices',
    grant: 'RETURN_RECORD_ON_BEHALF',
    snapshotHint: '代货主登一张退货预报：哪个仓、运单号，以及要退的货。',
  },
  {
    id: 'return-disposition',
    label: '代录退货处置',
    title: '代录退货回库处置',
    endpoint: '/warehouse-fulfillment-return-dispositions',
    grant: 'RETURN_RECORD_ON_BEHALF',
    snapshotHint: '给这一笔已点清的退货到仓记下回库去向：上架良品、次品、销毁或退回货主。货还在退货暂存，移出要用上架与移库。',
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
    snapshotHint: '把收货或退货暂存里的次品转成良品。批号、效期、贴标和入库批次认得出就填。',
  },
  {
    id: 'processing-instruction',
    label: '发起加工指令',
    title: '运营发起加工或拆分指令',
    endpoint: '/warehouse-fulfillment-processing-instructions',
    grant: 'PROCESSING_INSTRUCTION',
    snapshotHint: '发起加工或拆分。要拆开时勾上拆分。依据和商品修订可以不填。',
  },
  {
    id: 'processing-void',
    label: '作废加工指令',
    title: '作废加工或拆分指令',
    endpoint: '/warehouse-fulfillment-processing-instruction-voids',
    grant: 'PROCESSING_INSTRUCTION',
    snapshotHint: '作废还没执行的加工或拆分指令，写明原因。已经做了的不能作废。',
  },
  {
    id: 'relabel-instruction',
    label: '发起换标指令',
    title: '运营发起商品换标指令',
    endpoint: '/warehouse-fulfillment-relabel-instructions',
    grant: 'RELABEL_INSTRUCTION',
    snapshotHint: '发起换标：换成哪个贴标，以及原因。原来的贴标、批号、效期和依据可以不填。',
  },
  {
    id: 'relabel-void',
    label: '作废换标指令',
    title: '作废商品换标指令',
    endpoint: '/warehouse-fulfillment-relabel-instruction-voids',
    grant: 'RELABEL_INSTRUCTION',
    snapshotHint: '作废还没执行的换标指令，写明原因。',
  },
  {
    id: 'stock-freeze',
    label: '冻结库存',
    title: '冻结商品格或整箱',
    endpoint: '/warehouse-fulfillment-stock-freezes',
    grant: 'STOCK_FREEZE',
    snapshotHint: '冻结一格商品或一整箱。整箱只填箱号；商品要填库位、商品、单位、库存状态、入库批次和数量。',
  },
  {
    id: 'stock-unfreeze',
    label: '解冻库存',
    title: '整份解冻库存',
    endpoint: '/warehouse-fulfillment-stock-unfreezes',
    grant: 'STOCK_FREEZE',
    snapshotHint: '把这一份冻结整份解开。说明可以不填。',
  },
  {
    id: 'count-create',
    label: '创建盘点',
    title: '创建盘点任务',
    endpoint: '/warehouse-fulfillment-stock-counts',
    grant: 'STOCK_COUNT',
    snapshotHint: '建一张盘点：哪个仓、要盘哪些库位。货主、商品和说明可以不填，不填就是这个范围里都盘。',
  },
  {
    id: 'count-start',
    label: '开始盘点',
    title: '开始盘点',
    endpoint: '/warehouse-fulfillment-stock-count-starts',
    grant: 'STOCK_COUNT',
    snapshotHint: '开始这张盘点。开始之后这个范围锁住，别人不能动。',
  },
  {
    id: 'count-complete',
    label: '完成盘点',
    title: '完成盘点并生成差额',
    endpoint: '/warehouse-fulfillment-stock-count-completions',
    grant: 'STOCK_COUNT',
    snapshotHint: '完成这张盘点，按实点与账面的差生成差额。',
  },
  {
    id: 'count-void',
    label: '作废盘点',
    title: '作废盘点任务',
    endpoint: '/warehouse-fulfillment-stock-count-voids',
    grant: 'STOCK_COUNT',
    snapshotHint: '作废这张盘点。原因可以不填。',
  },
  {
    id: 'count-approve',
    label: '批准盘点差额',
    title: '批准盘点差额入账',
    endpoint: '/warehouse-fulfillment-count-approvals',
    grant: 'STOCK_ADJUSTMENT_APPROVAL',
    snapshotHint: '批准这一笔盘点差额入账。说明可以不填。还没登记批准职责规则时送不出去。',
  },
  {
    id: 'count-reject',
    label: '驳回盘点差额',
    title: '驳回盘点差额',
    endpoint: '/warehouse-fulfillment-count-rejections',
    grant: 'STOCK_ADJUSTMENT_APPROVAL',
    snapshotHint: '驳回这一笔盘点差额，写明原因。',
  },
  {
    id: 'job-discrepancy-approve',
    label: '批准作业差异',
    title: '批准库内作业差异入账',
    endpoint: '/warehouse-fulfillment-job-discrepancy-approvals',
    grant: 'STOCK_ADJUSTMENT_APPROVAL',
    snapshotHint: '批准这一笔库内作业差异入账。说明可以不填。上架、形态或状态转换、短少和整箱缺箱都走这里。还没登记批准职责规则时送不出去。',
  },
  {
    id: 'job-discrepancy-reject',
    label: '驳回作业差异',
    title: '驳回库内作业差异',
    endpoint: '/warehouse-fulfillment-job-discrepancy-rejections',
    grant: 'STOCK_ADJUSTMENT_APPROVAL',
    snapshotHint: '驳回这一笔作业差异，写明原因。账不动。',
  },
  {
    id: 'adjustment-submit',
    label: '提交库存调整',
    title: '提交库存调整',
    endpoint: '/warehouse-fulfillment-stock-adjustments',
    grant: 'STOCK_ADJUSTMENT_SUBMISSION',
    snapshotHint: '提交一张库存调整。每一行写增加、减少或销毁，以及库位、商品和数量。提交这一步还不改账。',
  },
  {
    id: 'adjustment-approve',
    label: '批准库存调整',
    title: '批准库存调整入账',
    endpoint: '/warehouse-fulfillment-stock-adjustment-approvals',
    grant: 'STOCK_ADJUSTMENT_APPROVAL',
    snapshotHint: '批准这一张库存调整入账。说明可以不填。',
  },
  {
    id: 'adjustment-reject',
    label: '驳回库存调整',
    title: '驳回库存调整',
    endpoint: '/warehouse-fulfillment-stock-adjustment-rejections',
    grant: 'STOCK_ADJUSTMENT_APPROVAL',
    snapshotHint: '驳回这一张库存调整，写明原因。',
  },
  {
    id: 'approval-duty',
    label: '登记批准职责规则',
    title: '登记库存调整批准职责规则',
    endpoint: '/warehouse-fulfillment-approval-duty-rules',
    grant: 'registry',
    snapshotHint: '登记下一版批准职责：批准人要不要和提交人不是同一个主体。修订号从 1 起连续。',
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
    snapshotHint: '代货主登一张出库单。发运方式、收件人和要出的货要填。走本产品网络时还要填服务。货主对这一张拣不齐时怎么发另有交代才填，没交代就不填。',
  },
  {
    id: 'outbound-split',
    label: '代录拆单',
    title: '代录拆开尚未分配的出库请求',
    endpoint: '/warehouse-fulfillment-outbound-request-splits',
    grant: 'OUTBOUND_REQUEST_RECORD_ON_BEHALF',
    snapshotHint: '把还没分配的出库单拆成几张，每张写自己的货。拆开后原来的预占放开，新的各自再预占。已经拣出货的单不能这样拆。',
  },
  {
    id: 'outbound-cutoff',
    label: '代录截单',
    title: '代录截单取消出库请求或单个包裹',
    endpoint: '/warehouse-fulfillment-outbound-cancellations',
    grant: 'OUTBOUND_REQUEST_RECORD_ON_BEHALF',
    snapshotHint: '截掉这张出库单，或只截其中一个包裹。写明依据。',
  },
  {
    id: 'partial-shipment-consent',
    label: '代录部分发货同意',
    title: '代录货主对一张出库单拣不齐时怎么发的同意',
    endpoint: '/warehouse-fulfillment-outbound-partial-shipment-consents',
    grant: 'OUTBOUND_REQUEST_RECORD_ON_BEHALF',
    snapshotHint: '记下货主对这一张拣不齐时怎么发：整单发、少发或拆单发，并写明那句话的出处。它盖过提交时写的和货主的默认，不动已经预占的货和拣货任务。',
  },
  {
    id: 'owner-label',
    label: '登记货主面单',
    title: '为自备面单包裹登记面单号',
    endpoint: '/warehouse-fulfillment-owner-labels',
    grant: 'OUTBOUND_REQUEST_RECORD_ON_BEHALF',
    snapshotHint: '给自备面单的一个包裹登记面单号。',
  },
  {
    id: 'wave-release',
    label: '放波次',
    title: '释放波次并分配拣货任务',
    endpoint: '/warehouse-fulfillment-waves',
    grant: 'WAVE_RELEASE',
    snapshotHint: '放出一个波次并分配拣货任务。可以选合并拣货、只放放得下的，以及预取估算委托。这个仓还没采用分配策略时放不了。',
  },
  {
    id: 'wave-withdraw',
    label: '撤单',
    title: '把出库单从已放出的波次里撤出',
    endpoint: '/warehouse-fulfillment-wave-withdrawals',
    grant: 'WAVE_RELEASE',
    snapshotHint: '把还没拣过的出库单撤出这个波次。撤出后拣货任务作废，出库单回到已预占，可以再放进别的波次。点名的几张里有一张撤不了，就一张都不撤。',
  },
  {
    id: 'short-shipment',
    label: '按已拣数量发货',
    title: '拣不齐的出库单按已拣数量发货',
    endpoint: '/warehouse-fulfillment-outbound-short-shipments',
    grant: 'WAVE_RELEASE',
    snapshotHint: '这张拣不齐、等处置的单按已拣数量发。货主许了少发才成立。没拣到的不再发，这一步撤不回。',
  },
  {
    id: 'shortage-split',
    label: '拆出缺货部分',
    title: '拣不齐的出库单拆出缺货部分',
    endpoint: '/warehouse-fulfillment-outbound-shortage-splits',
    grant: 'WAVE_RELEASE',
    snapshotHint: '这张拣不齐、等处置的单把没拣到的拆成另一张出库单。拆多少按没拣到的预占，不用填。原单按已拣数量往下走。这一步撤不回。',
  },
  {
    id: 'tote-register',
    label: '登记周转箱',
    title: '给一个仓登记一批周转箱',
    endpoint: '/warehouse-fulfillment-tote-registrations',
    grant: 'registry',
    snapshotHint: '给这个自营仓登记一批周转箱。已经在册的照旧，停用过的恢复在用。一次最多五百个。拣货车上的格口也按箱登记。',
  },
  {
    id: 'tote-retire',
    label: '停用周转箱',
    title: '停用一个周转箱',
    endpoint: '/warehouse-fulfillment-tote-retirements',
    grant: 'registry',
    snapshotHint: '停用这个周转箱。箱上还绑着出库单时不停。再登记同一个箱号就恢复在用。',
  },
  {
    id: 'tote-requirement',
    label: '登记拣货必须用箱',
    title: '登记一个仓拣货是否必须用周转箱',
    endpoint: '/warehouse-fulfillment-tote-requirements',
    grant: 'registry',
    snapshotHint: '登记这个仓拣货要不要必须用周转箱。修订号从 1 起连续。登记成必须之后，不带箱号的拣货不成立。没登记的仓带不带箱都收。',
  },
  {
    id: 'wave-assign',
    label: '指派拣货人',
    title: '把一个波次派给一个人拣，或取消指派',
    endpoint: '/warehouse-fulfillment-wave-assignments',
    grant: 'WAVE_RELEASE',
    snapshotHint: '把这个波次派给一个人拣，或留空取消指派。拣货人从操作者名册里选。派了之后别人拣不了；拣货的人也可以在工位上自己领。已完成、已取消的波次不能派。',
  },
  {
    id: 'wave-rule',
    label: '登记成波规则',
    title: '登记一个仓的一条成波规则',
    endpoint: '/warehouse-fulfillment-wave-rules',
    grant: 'registry',
    snapshotHint: '给这个仓登记一条成波规则，或改下一版。一个波次最多几张要填。每天几点放、最多几行几件、货主、发运方式、件型和是否合并拣货都可以不填，不填就是不限。',
  },
  {
    id: 'tote-release',
    label: '放开周转箱',
    title: '由人放开一个周转箱',
    endpoint: '/warehouse-fulfillment-tote-releases',
    grant: 'WAVE_RELEASE',
    snapshotHint: '由人放开这个周转箱。平时在打完包或截单回库后由系统放开；箱被拿错、被清空而系统不知道时才用这一下。只解开箱和出库单的绑定。',
  },
  {
    id: 'allocation-strategy',
    label: '采用分配策略',
    title: '采用内置参考分配策略',
    endpoint: '/warehouse-fulfillment-allocation-strategy-adoptions',
    grant: 'registry',
    snapshotHint: '采用内置的分配策略。修订号从 1 起连续。登记之后，放波次才按先到期先出、先进先出，并且优先拣选库位。',
  },
  {
    id: 'shortage-policy',
    label: '登记缺货处置',
    title: '登记货主缺货处置规则',
    endpoint: '/warehouse-fulfillment-shortage-policies',
    grant: 'registry',
    snapshotHint: '登记这个货主的下一版缺货处置。至少写一件：受理时库存不够是部分预占还是拒单，以及拣不齐时默认整单发、少发还是拆单发。没填的那一件就是没登记。',
  },
  {
    id: 'confirmation-correction',
    label: '更正出库确认',
    title: '更正出库确认时点或判不成立',
    endpoint: '/warehouse-fulfillment-outbound-confirmation-corrections',
    grant: 'OUTBOUND_CONFIRMATION_CORRECTION',
    snapshotHint: '更正这一笔出库确认：改发生时刻，或判它不成立。两件里做一件。',
  },
  {
    id: 'confirmation-replenishment',
    label: '批准确认回补',
    title: '批准判不成立出库确认的库存回补',
    endpoint: '/warehouse-fulfillment-outbound-confirmation-replenishments',
    grant: 'STOCK_ADJUSTMENT_APPROVAL',
    snapshotHint: '批准把判不成立的出库确认所消耗的货补回去。还没登记批准职责规则时送不出去。',
  },
  {
    id: 'agent-connector',
    label: '登记代理仓连接器',
    title: '登记代理仓采用模拟源',
    endpoint: '/warehouse-fulfillment-agent-connectors',
    grant: 'registry',
    snapshotHint: '这个代理仓改用模拟源。连接用的凭据不写在这里。',
  },
  {
    id: 'agent-statement',
    label: '记代理仓库存声明',
    title: '登记代理仓报来的库存声明',
    endpoint: '/warehouse-fulfillment-agent-stock-statements',
    grant: 'registry',
    snapshotHint: '记下代理仓报来的一份库存声明：何时说的、有效到何时、哪种货有多少。它不进自营库存账。',
  },
  {
    id: 'agent-result',
    label: '转出或记代理仓结果',
    title: '向代理仓转出请求或登记代理仓回传结果',
    endpoint: '/warehouse-fulfillment-agent-outbound-results',
    grant: 'registry',
    snapshotHint: '把出库请求转给代理仓，或记下代理仓回传的结果：已打包、已交接、拒单或缺货。不是转出时要有结果号；打包还要写重量和尺寸。',
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
    snapshotHint: '登记一份仓储计费方案。单价和基费按最小货币单位的整数填。这个部署没开较新的规范化时登不了。',
  },
  {
    id: 'charge-adoption',
    label: '登记仓储计费采用',
    title: '登记货主与仓的仓储计费采用',
    endpoint: '/warehouse-fulfillment-charge-adoptions',
    grant: 'registry',
    snapshotHint: '这个货主在这个仓从何时起用哪一份仓储计费。结算账户和计价范围要填。金额不在这里。修订号从 1 起连续。',
  },
  {
    id: 'charge-units',
    label: '登记重量体积单位',
    title: '登记仓的收费重量与体积单位',
    endpoint: '/warehouse-fulfillment-charge-measure-units',
    grant: 'registry',
    snapshotHint: '登记这个仓收费用的重量单位和体积单位。没登记时，带重量或体积的收费发生项形成不了。',
  },
  {
    id: 'charge-aging',
    label: '登记库龄口径',
    title: '登记仓的仓租库龄口径',
    endpoint: '/warehouse-fulfillment-charge-aging-bases',
    grant: 'registry',
    snapshotHint: '登记这个仓的仓租按实批算库龄，还是按理论先进先出。',
  },
  {
    id: 'charge-project',
    label: '登记增值服务项目',
    title: '登记仓的增值服务项目名',
    endpoint: '/warehouse-fulfillment-charge-service-projects',
    grant: 'registry',
    snapshotHint: '登记这个仓的一个增值服务项目名。',
  },
  {
    id: 'storage-day-close',
    label: '仓租日结',
    title: '按自然日触发一次仓租发生项日结',
    endpoint: '/warehouse-fulfillment-storage-day-closes',
    grant: 'registry',
    snapshotHint: '给这个货主在这个仓结一天的仓租。日子按自然日填。仓还没登记业务时区或库龄口径时结不了。',
  },
];
