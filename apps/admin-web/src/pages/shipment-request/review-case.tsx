import type { DetailField } from '../../templates';
import type { TechnicalRow } from '../../components/TechnicalDetails';
import { StatusPill } from '../../components/StatusPill';
import { CodeRef } from '../../components/CodeRef';
import { labelFor } from '../../vocabulary/status';
import { accessRefusalHeadline } from '../../auth/access-refusal';
import { formatInstant } from '../catalogue-view';
import type { AcceptanceReviewCaseResponseBody, ApiResult, ShipmentRequestDetail } from './api';
import { ShipmentNo } from './refs';

// 接受前复核页与授权处置页共用的单份详情呈现:两页审的都是同一份复核案卷(GET /acceptance-review-queue?shipmentRequestId=),
// 分两处写就得在两处约定哪一份是准的。业务行只放运营要看的(运单号、客户、件数、状态、进度);各种标识、版本与
// 续办引用进技术详情。

/** 右栏业务行。状态与进度经词典译;最近一次没推进的原因有过才在场,没有不占一格。 */
export function caseBusinessFields(request: ShipmentRequestDetail): DetailField[] {
  const task = request.acceptanceTask;
  const fields: DetailField[] = [
    { label: '运单号', value: <ShipmentNo shipmentRequestId={request.shipmentRequestId} /> },
    { label: '客户单号', value: <CodeRef value={request.sourceRequestKey} title="客户单号" /> },
    { label: '客户', value: request.customerAccountId },
    { label: '件数', value: `${request.declaredParcelCount} 件` },
    { label: '状态', value: <StatusPill code={request.state} /> },
    { label: '提交时间', value: formatInstant(request.submittedAt) },
    { label: '处理进度', value: <StatusPill code={task.state} /> },
  ];
  if (task.lastAttemptReason) {
    fields.push({ label: '上次没推进', value: labelFor(task.lastAttemptReason) });
  }
  if (request.decision) {
    fields.push({
      label: '已有决定',
      value: (
        <span className="inline-flex items-center gap-2">
          <StatusPill code={request.decision.kind} />
          <span>{formatInstant(request.decision.decidedAt)}</span>
        </span>
      ),
    });
  }
  return fields;
}

/** 右栏折叠区:整串标识、版本、来源主体与续办引用,排查与对协议时才用;调用方再接自己那几行。 */
export function caseTechnicalRows(request: ShipmentRequestDetail, extras: TechnicalRow[] = []): TechnicalRow[] {
  const task = request.acceptanceTask;
  return [
    { label: '委托标识', value: request.shipmentRequestId },
    { label: '客户账户', value: request.customerAccountId },
    { label: '来源主体', value: request.source },
    { label: '来源请求键', value: request.sourceRequestKey },
    { label: '提交版本', value: request.submissionVersionId },
    { label: '委托状态码', value: request.state },
    { label: '判断任务状态', value: task.state },
    { label: '上次未推进原因', value: task.lastAttemptReason ?? '' },
    { label: '续办引用', value: task.lastAttemptContinuation ?? '' },
    { label: '决定标识', value: request.decision?.decisionId ?? '' },
    ...extras,
  ];
}

/** 单份读不回时说清缺在哪一步。渠道未配置与「查不到」不并成一句:恢复动作不同。 */
export function caseFailureText(answer: ApiResult<AcceptanceReviewCaseResponseBody>): string {
  switch (answer.kind) {
    case 'outcome':
      return '';
    case 'unconfigured':
      return '管理台还没接上操作者登录渠道,读不到详情。';
    case 'loginRequired':
    case 'notGranted':
      return `${accessRefusalHeadline(answer.kind)},读不到详情。`;
    case 'transport':
      return '没有连上服务器,读不到详情;请检查网络后刷新。';
    case 'callerProblem':
      return '按这个运单号读不到详情,可能刚被处理掉了;请刷新清单。';
    default:
      return '这次没有读到详情,请稍后刷新。';
  }
}
