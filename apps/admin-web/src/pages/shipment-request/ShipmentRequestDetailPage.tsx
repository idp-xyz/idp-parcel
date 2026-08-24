import {
  DetailPageTemplate,
  type DetailField,
  type DetailSection,
} from '../../templates';
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from '@idpxyz/ui-primitives';
import { StatusBadgeFor, type DomainStatus } from '../../domain/status';
import { requestStateLabels, withCode } from './presentation';

// 委托详情骨架。字段与区块取 parcel-shipment CONTEXT.md 与 UC-PS-001 的原词;
// 查询端点未建,view 现阶段恒缺席,内容区如实呈现「未配置」态。builder 按 view
// 组装是接线路径:查询契约落地后调用方喂入 view 即转 ready,骨架不用重搭。
//
// 「统一不可见结果」(CONTEXT.md)约束接线后的取数:不存在、越权与其他租户对象
// 同一语义,本页不得按取数失败原因分辨呈现「不存在」与「无权查看」。

/** 声明包裹一行(UC-PS-001 声明包裹组:客户侧引用、声明测量、货物和服务资料)。 */
export interface DeclaredParcelView {
  customerParcelReference: string;
  /** 声明毛重:值与单位是客户引用原样保全,不规范化(与 api.ts 草案同一态度)。 */
  declaredWeightValue: string;
  declaredWeightUnit: string;
  goodsDescription?: string;
}

/** 当前提交版本(CONTEXT「委托提交版本」:原版本、成员、原因、提交主体必须保留)。 */
export interface SubmissionVersionView {
  versionId: string;
  submittedBy: string;
  /** 形成原因:首次提交,或同一委托边界内的纠错/补充。 */
  reason: string;
}

/** 接受判断任务(CONTEXT 原词:判断阶段、尚缺的权威结果、采用版本、最近处理结果、续办关系)。 */
export interface AcceptanceTaskView {
  stage: string;
  missingAuthorities: string[];
  adoptedVersion: string;
  lastResult: string;
  continuation: string;
}

/** 委托接受基线(CONTEXT:接受时形成的不可覆盖快照)与预计承诺(接受时形成)。 */
export interface AcceptanceBaselineView {
  acceptedAt: string;
  /** 客户声明的包裹成员(基线固定的成员引用)。 */
  memberReferences: string[];
  /** 客户与责任法人。 */
  responsibleLegalEntity: string;
  /** 合同与服务产品依据。 */
  contractAndProduct: string;
  /** 服务要求快照引用。 */
  serviceRequirements: string;
  /** 适用商业依据引用。 */
  commercialBasis: string;
  /** 预计承诺(CONTEXT 客户承诺:委托接受 → 预计承诺形成)。 */
  estimatedCommitment: string;
}

/**
 * 详情视图形状。查询契约未建,这是页面侧暂定;真契约落地时以它为准重谈,
 * 不得反过来把这里当已发布的查询 Schema。
 */
export interface ShipmentRequestDetailView {
  shipmentRequestId: string;
  /** 提交批次(CONTEXT:客户一次提交或导入多份委托的处理归组,不取得服务责任)。 */
  batchId: string;
  state: string;
  customerShipmentReference: string;
  requestedServiceProduct: string;
  destinationServiceScope: string;
  senderRelation: string;
  recipientRelation: string;
  /**
   * 客户请求生效时间 requestEffectiveAt:值与缺失/显式存在状态都进入内容摘要
   * (CONTEXT),因此缺席时要呈现「未声明」这一事实本身,不能默认补齐。
   */
  requestEffectiveAt?: string;
  /** 来源发生时间 occurredAt(来源信封元数据,不进入内容摘要)。 */
  occurredAt: string;
  /** 系统接收时间 receivedAt(来源信封元数据,不进入内容摘要)。 */
  receivedAt: string;
  parcels: DeclaredParcelView[];
  currentVersion?: SubmissionVersionView;
  /** 委托仍为「已提交」时在场:任务未完成时委托保持已提交(CONTEXT 接受判断任务)。 */
  acceptanceTask?: AcceptanceTaskView;
  /** 委托「已接受」后在场:后续变化不得静默覆盖(CONTEXT 委托接受基线)。 */
  acceptanceBaseline?: AcceptanceBaselineView;
}

function buildBasicFields(view: ShipmentRequestDetailView): DetailField[] {
  return [
    { label: '委托标识', value: <span className="font-mono">{view.shipmentRequestId}</span> },
    { label: '提交批次', value: <span className="font-mono">{view.batchId}</span> },
    {
      label: '委托状态',
      value: withCode(requestStateLabels[view.state], view.state),
    },
    { label: '客户委托参考', value: <span className="font-mono">{view.customerShipmentReference}</span> },
    { label: '请求的服务产品', value: view.requestedServiceProduct },
    { label: '目的服务范围', value: view.destinationServiceScope },
    { label: '寄件关系', value: view.senderRelation },
    { label: '收件关系', value: view.recipientRelation },
    {
      label: '客户请求生效时间',
      // 缺失与显式存在是两种要分别呈现的事实,不是空串。
      value: view.requestEffectiveAt ?? '未声明',
    },
    { label: '来源发生时间', value: <span className="font-mono">{view.occurredAt}</span> },
    { label: '系统接收时间', value: <span className="font-mono">{view.receivedAt}</span> },
  ];
}

function buildSections(view: ShipmentRequestDetailView): DetailSection[] {
  const sections: DetailSection[] = [
    {
      id: 'declared-parcels',
      title: '声明包裹',
      description:
        '客户声明成员及各自客户侧引用、声明测量与货物资料;接受后成员基线冻结,增删走取消或关联新委托。',
      content: (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>客户侧包裹引用</TableHead>
              <TableHead>声明毛重</TableHead>
              <TableHead>品名</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {view.parcels.map((parcel) => (
              <TableRow key={parcel.customerParcelReference}>
                <TableCell>
                  <span className="font-mono text-[12px]">{parcel.customerParcelReference}</span>
                </TableCell>
                <TableCell>
                  {parcel.declaredWeightValue} {parcel.declaredWeightUnit}
                </TableCell>
                <TableCell>{parcel.goodsDescription ?? '—'}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ),
    },
  ];

  if (view.currentVersion) {
    sections.push({
      id: 'current-submission-version',
      title: '当前提交版本',
      description:
        '同一委托同一时刻只有一个待判断的当前提交版本;纠错或补充形成新版本,原版本与判断历史保留。',
      content: (
        <dl className="space-y-1">
          <FieldRow label="版本标识" value={view.currentVersion.versionId} mono />
          <FieldRow label="提交主体" value={view.currentVersion.submittedBy} />
          <FieldRow label="形成原因" value={view.currentVersion.reason} />
        </dl>
      ),
    });
  }

  if (view.acceptanceTask) {
    sections.push({
      id: 'acceptance-task',
      title: '接受判断任务',
      description: '任务未完成时委托仍为「已提交」;它可续办,但不是委托的新领域状态。',
      content: (
        <dl className="space-y-1">
          <FieldRow label="判断阶段" value={view.acceptanceTask.stage} />
          <FieldRow
            label="尚缺的权威结果"
            value={
              view.acceptanceTask.missingAuthorities.length > 0
                ? view.acceptanceTask.missingAuthorities.join('、')
                : '无'
            }
          />
          <FieldRow label="采用版本" value={view.acceptanceTask.adoptedVersion} mono />
          <FieldRow label="最近处理结果" value={view.acceptanceTask.lastResult} />
          <FieldRow label="续办关系" value={view.acceptanceTask.continuation} mono />
        </dl>
      ),
    });
  }

  if (view.acceptanceBaseline) {
    sections.push({
      id: 'acceptance-baseline',
      title: '委托接受基线',
      description:
        '接受时形成的不可覆盖快照;后续取消、更正或物理身份演化不能改写该基线。',
      content: (
        <dl className="space-y-1">
          <FieldRow label="接受时间" value={view.acceptanceBaseline.acceptedAt} mono />
          <FieldRow
            label="客户声明的包裹成员"
            value={view.acceptanceBaseline.memberReferences.join('、')}
            mono
          />
          <FieldRow label="客户与责任法人" value={view.acceptanceBaseline.responsibleLegalEntity} />
          <FieldRow label="合同与服务产品" value={view.acceptanceBaseline.contractAndProduct} />
          <FieldRow label="服务要求" value={view.acceptanceBaseline.serviceRequirements} />
          <FieldRow label="适用商业依据" value={view.acceptanceBaseline.commercialBasis} mono />
          <FieldRow label="预计承诺" value={view.acceptanceBaseline.estimatedCommitment} />
        </dl>
      ),
    });
  }

  return sections;
}

/** 区块内的键值行,排版对齐 DetailPageTemplate 基本信息区。 */
function FieldRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex gap-3 text-[12px] leading-5">
      <dt className="w-[128px] shrink-0 text-idpxyz-textMuted">{label}</dt>
      <dd className={`min-w-0 flex-1 break-words text-idpxyz-text ${mono ? 'font-mono' : ''}`}>
        {value}
      </dd>
    </div>
  );
}

export function ShipmentRequestDetailPage({
  shipmentRequestId,
  view,
  onBack,
}: {
  /** 从列表钻取时携带的委托标识;查询未接线时仅用于页头示名。 */
  shipmentRequestId?: string;
  /** 查询接线后由调用方喂入;当前恒缺席。 */
  view?: ShipmentRequestDetailView;
  onBack?: () => void;
}) {
  const stateWord = view ? requestStateLabels[view.state] : undefined;

  return (
    <DetailPageTemplate
      title="委托详情"
      identifier={view?.shipmentRequestId ?? shipmentRequestId}
      status={stateWord ? <StatusBadgeFor status={stateWord as DomainStatus} /> : undefined}
      description="委托的服务身份、承诺与商业生命周期;全程运营主状态归 visibility-exception,本页不呈现。"
      headerActions={
        onBack ? (
          <button
            type="button"
            onClick={onBack}
            className="rounded border border-idpxyz-border px-3 py-1.5 text-[12px] text-idpxyz-textMuted hover:bg-idpxyz-hover"
          >
            返回列表
          </button>
        ) : undefined
      }
      basicFields={view ? buildBasicFields(view) : []}
      sections={view ? buildSections(view) : undefined}
      viewState={
        view
          ? { kind: 'ready' }
          : {
              kind: 'unconfigured',
              title: '委托查询端点尚未建立',
              description:
                '详情数据待查询契约建立后接线;区块骨架(声明包裹、当前提交版本、接受判断任务、' +
                '委托接受基线)已按 parcel-shipment CONTEXT 原词搭好。本页不发请求、不含合成数据。',
            }
      }
    />
  );
}
