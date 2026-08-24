import { useEffect, useState } from 'react';
import {
  DetailPageTemplate,
  type DetailField,
  type DetailSection,
  type TemplateViewState,
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
import {
  requestStateLabels,
  acceptanceTaskStateLabels,
  decisionKindLabels,
  problemNote,
  withCode,
} from './presentation';
import {
  findShipmentRequestView,
  REQUEST_NOT_VISIBLE_CODE,
  type ApiResult,
  type ShipmentRequestDetail,
  type ViewDetailResponseBody,
} from './api';

// 委托详情，对 GET /shipment-request-views?shipmentRequestId=… 真实端点取数。
//
// 字段与区块跟已落地的查询契约走（api.ts 的 ShipmentRequestDetail 镜像），读模型
// 没有的东西（客户委托参考、寄收件关系、接受基线快照）不虚构；那些查阅面扩进读
// 模型时，先扩 ports 的记录与端点响应，再回来加区块。
//
// 「统一不可见结果」（CONTEXT.md）：单份查阅的 404 SHIPMENT_REQUEST_NOT_VISIBLE
// 是终局业务答案，不存在、越权与其他租户对象同一语义——本页对它只说「不可见」，
// 不按取数失败原因分辨呈现「不存在」与「无权查看」，也不提供重试。

function buildBasicFields(view: ShipmentRequestDetail): DetailField[] {
  return [
    { label: '委托标识', value: <span className="font-mono">{view.shipmentRequestId}</span> },
    { label: '客户账户', value: <span className="font-mono">{view.customerAccountId}</span> },
    {
      label: '委托状态',
      value: withCode(requestStateLabels[view.state], view.state),
    },
    { label: '提交批次', value: <span className="font-mono">{view.batchId}</span> },
    { label: '来源', value: <span className="font-mono">{view.source}</span> },
    { label: '来源请求键', value: <span className="font-mono">{view.sourceRequestKey}</span> },
    { label: '当前提交版本', value: <span className="font-mono">{view.submissionVersionId}</span> },
    // 此前版本数是「同一委托边界内纠错/补充」的痕迹计数，0 表示首版即当前版。
    { label: '此前版本数', value: String(view.priorVersionCount) },
    { label: '提交时间', value: <span className="font-mono">{view.submittedAt}</span> },
    { label: '来源发生时间', value: <span className="font-mono">{view.occurredAt}</span> },
    { label: '系统接收时间', value: <span className="font-mono">{view.receivedAt}</span> },
  ];
}

function buildSections(view: ShipmentRequestDetail): DetailSection[] {
  const sections: DetailSection[] = [
    {
      id: 'declared-parcels',
      title: '声明包裹',
      description:
        '客户声明成员（读模型粒度：内部包裹标识与声明测量，值原样保全不规范化）；' +
        '接受后成员基线冻结，增删走取消或关联新委托。',
      content: (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>包裹标识</TableHead>
              <TableHead>声明毛重</TableHead>
              <TableHead>声明外廓</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {view.declaredParcels.map((parcel) => (
              <TableRow key={parcel.parcelId}>
                <TableCell>
                  <span className="font-mono text-[12px]">{parcel.parcelId}</span>
                </TableCell>
                <TableCell>
                  {parcel.declaredWeightValue
                    ? `${parcel.declaredWeightValue} ${parcel.declaredWeightUnit ?? ''}`
                    : '未声明'}
                </TableCell>
                <TableCell>
                  {parcel.dimensions
                    ? `${parcel.dimensions.length} × ${parcel.dimensions.width} × ${parcel.dimensions.height} ${parcel.dimensions.unit}`
                    : '未声明'}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ),
    },
    {
      id: 'acceptance-task',
      title: '接受判断任务',
      description:
        '任务阶段与最近一次没能推进的处理记录都保留——只留成功判断的话，' +
        '一份卡了十轮的委托看起来会和刚建单的一模一样。',
      content: (
        <dl className="space-y-1">
          <FieldRow
            label="任务状态"
            value={withCode(acceptanceTaskStateLabels[view.acceptanceTask.state], view.acceptanceTask.state)}
          />
          {view.acceptanceTask.lastAttemptReason !== undefined && (
            <FieldRow label="最近处理结果" value={view.acceptanceTask.lastAttemptReason} />
          )}
          {view.acceptanceTask.lastAttemptContinuation !== undefined && (
            <FieldRow label="续办引用" value={view.acceptanceTask.lastAttemptContinuation} mono />
          )}
          {view.acceptanceTask.lastAttemptedAt !== undefined && (
            <FieldRow label="最近处理时间" value={view.acceptanceTask.lastAttemptedAt} mono />
          )}
        </dl>
      ),
    },
  ];

  if (view.decision) {
    sections.push({
      id: 'decision',
      title: '接受/拒绝决定',
      description: '已形成的决定不可覆盖；决定后的撤回不再可用，后续走对应的决定后入口。',
      content: (
        <dl className="space-y-1">
          <FieldRow label="决定标识" value={view.decision.decisionId} mono />
          <FieldRow
            label="决定种类"
            value={withCode(decisionKindLabels[view.decision.kind], view.decision.kind)}
          />
          <FieldRow label="决定时间" value={view.decision.decidedAt} mono />
        </dl>
      ),
    });
  }

  return sections;
}

/** 区块内的键值行，排版对齐 DetailPageTemplate 基本信息区。 */
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

// 取数答案 → 模板四态。不可见走空态（终局答案、无重试）；其余错误格与列表页同款。
function viewStateOf(
  answer: ApiResult<ViewDetailResponseBody> | null,
  retry: () => void,
): TemplateViewState {
  if (answer === null) return { kind: 'loading' };
  switch (answer.kind) {
    case 'outcome':
      return { kind: 'ready' };
    case 'unconfigured':
      return {
        kind: 'unconfigured',
        title: '接入渠道未配置',
        description:
          '查询端点已建立，但接入渠道未配置（403 ACCESS_CHANNEL_NOT_CONFIGURED）。' +
          '恢复动作是提供渠道参数（PAR-INT-01），改请求或重试不会改变结果。',
      };
    case 'callerProblem':
      if (answer.code === REQUEST_NOT_VISIBLE_CODE) {
        return {
          kind: 'empty',
          title: '委托不可见',
          description:
            '在当前授权查询作用域内查不到该委托。统一不可见结果：不存在、越权与' +
            '其他租户对象同一语义，本页不作区分。',
        };
      }
      return {
        kind: 'error',
        title: `调用方式问题（HTTP ${answer.status}）`,
        description: problemNote(answer.code),
      };
    case 'noAnswer':
      return {
        kind: 'error',
        title: `服务端未形成答案（HTTP ${answer.status}）`,
        description: problemNote(answer.code),
        onRetry: retry,
      };
    case 'transport':
      return {
        kind: 'error',
        title: '请求未到达 parcel-api',
        description: `${answer.message}；请确认代理与服务端在运行（约定走 /api 经代理转发）。`,
        onRetry: retry,
      };
  }
}

export function ShipmentRequestDetailPage({
  shipmentRequestId,
  onBack,
}: {
  /** 要查阅的委托标识；本页自取数，标识只定位对象，不单独证明查询权限。 */
  shipmentRequestId: string;
  onBack?: () => void;
}) {
  const [answer, setAnswer] = useState<ApiResult<ViewDetailResponseBody> | null>(null);
  const [reloadToken, setReloadToken] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void findShipmentRequestView(shipmentRequestId).then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [shipmentRequestId, reloadToken]);

  const view = answer?.kind === 'outcome' ? answer.body.request : undefined;
  const stateWord = view ? requestStateLabels[view.state] : undefined;

  return (
    <DetailPageTemplate
      title="委托详情"
      identifier={view?.shipmentRequestId ?? shipmentRequestId}
      status={stateWord ? <StatusBadgeFor status={stateWord as DomainStatus} /> : undefined}
      description="委托的服务身份、承诺与商业生命周期；全程运营主状态归 visibility-exception，本页不呈现。"
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
      viewState={viewStateOf(answer, () => setReloadToken((token) => token + 1))}
    />
  );
}
