import { useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Textarea } from '@idpxyz/ui-primitives';
import type { ApiResult } from '../catalogue-api';
import type { RegistrationResponseBody } from '../../components/registration';
import {
  reviewReferenceSeries,
  seriesReviewOutcomeLabels,
  type SeriesReviewRequest,
} from './api';
import { problemNote } from './presentation';

/**
 * 序列版本复核的行动作面板（ADR-0099 决定二；票 pricing-reference-series-operations/04
 * 切片 04b）。
 *
 * **为什么是逐字段表单而不是 JSON 快照口。** [ADR-0101](../../../../docs/adr/0101-operator-facing-registration-payload-shape-is-product-defined.md)
 * 决定一把「载荷形状属渠道接入契约」的适用场景收窄为**客户渠道载荷**，并明定运营操作者面
 * 的形状由产品定义、各册按「登记频次 × 操作者角色 × 载荷结构」自裁；决定八要求每册在自己
 * 的实施票里写明选了哪一形。复核是**低频、两格、治理性质**的动作，逐字段表单是这三条上的
 * 直接读数——它不像价卡那样是上千格的矩阵（那一册按决定二走模板导入）。
 *
 * **表单上没有「复核责任方」这一格，那不是漏做。** 复核责任方是四眼门的一半（领域拒绝
 * 复核责任方等于登记责任方），从浏览器收一个上去就是自报身份；传输层
 * `ReferenceSeriesReviewIntake` 的注释原话是「从请求内容里铸一个出来就等于把那道门拆了」。
 * 正当出处是 ADR-0100 的 `OperatorEnvelope`，由接入渠道给。今天渠道未配置，所以提交必然
 * 答 403——**页面因此要说「接入渠道未配置」而不是「尚未实现」**：机制在，墙也在，两者
 * 不是一回事。
 *
 * 复核时刻同样不在表单上：复核就是复核责任方此刻作出的确认，时刻取服务端时钟。
 */
export interface SeriesReviewTarget {
  seriesId: string;
  seriesVersion: string;
  /** 登记责任方，只用于在面板上提示四眼门——不随请求送出。 */
  registrant: string;
}

type PanelState =
  | { kind: 'idle' }
  | { kind: 'submitting' }
  | { kind: 'answered'; answer: ApiResult<RegistrationResponseBody> };

export function SeriesReviewPanel({
  target,
  onClose,
}: {
  target: SeriesReviewTarget;
  onClose: () => void;
}) {
  const [decision, setDecision] = useState<SeriesReviewRequest['decision'] | null>(null);
  const [basis, setBasis] = useState('');
  const [state, setState] = useState<PanelState>({ kind: 'idle' });

  // 两件都齐才让提交：结论是封闭两格、依据是硬句（库上 CHECK 与领域构造门各守一道）。
  // 本地拦住空依据不是替服务端判断，是不把一个必然被拒的请求送上去——那个 400/拒绝会与
  // 治理答案挤在同一格里，而两者的续办动作不同。
  const ready = decision !== null && basis.trim() !== '';

  const send = () => {
    if (decision === null) return;
    setState({ kind: 'submitting' });
    void reviewReferenceSeries({
      seriesId: target.seriesId,
      seriesVersion: target.seriesVersion,
      decision,
      basis: basis.trim(),
    }).then((answer) => setState({ kind: 'answered', answer }));
  };

  return (
    <Card className="m-4">
      <CardHeader>
        <CardTitle>
          复核 <span className="font-mono text-[13px]">{target.seriesId}</span>
          <span className="font-mono text-[13px] text-idpxyz-textMuted">
            @{target.seriesVersion}
          </span>
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <p className="text-xs text-idpxyz-textMuted">
          复核是追加在版本之旁的独立事实，不改原登记一列；结论为通过时，该版本自复核时刻起
          在用（ADR-0099 决定二、三）。
          <br />
          复核责任方由接入渠道给，本表单不收也不送——它是四眼门的一半，自报即失效。该版本的
          登记责任方是 <span className="font-mono">{target.registrant}</span>，
          <strong>同一个人复核会被领域拒绝</strong>。
        </p>

        <div className="flex items-center gap-2">
          <span className="text-xs text-idpxyz-textMuted">结论</span>
          <Button
            variant={decision === 'APPROVED' ? 'default' : 'outline'}
            onClick={() => setDecision('APPROVED')}
          >
            通过
          </Button>
          <Button
            variant={decision === 'RETURNED' ? 'default' : 'outline'}
            onClick={() => setDecision('RETURNED')}
          >
            退回
          </Button>
        </div>

        <Textarea
          value={basis}
          onChange={(event) => setBasis(event.target.value)}
          placeholder="复核依据：凭什么作出这个结论（必填）"
          rows={4}
          className="text-xs"
        />

        <div className="flex items-center gap-3">
          <Button onClick={send} disabled={!ready || state.kind === 'submitting'}>
            {state.kind === 'submitting' ? '提交中…' : '提交复核'}
          </Button>
          <Button variant="outline" onClick={onClose}>
            收起
          </Button>
          <ReviewAnswerNote state={state} />
        </div>
      </CardContent>
    </Card>
  );
}

function ReviewAnswerNote({ state }: { state: PanelState }) {
  if (state.kind === 'idle' || state.kind === 'submitting') return null;

  const answer = state.answer;
  switch (answer.kind) {
    case 'outcome': {
      const outcome = answer.body.outcome;
      // 未收录的 outcome 原样示出英文原名：服务端新增一格时，宁可显示原名也不把它归进某个
      // 既有中文说法——那会让一种新答案冒充另一种。
      return (
        <p className="text-xs text-idpxyz-textMuted">
          复核册答复：<span className="font-mono">{outcome}</span> ——{' '}
          {seriesReviewOutcomeLabels[outcome] ?? outcome}
        </p>
      );
    }
    case 'unconfigured':
      return (
        <p className="text-xs text-idpxyz-textMuted">
          接入渠道未配置（403 ACCESS_CHANNEL_NOT_CONFIGURED）。复核端点已建立并装配，但操作者
          身份的接入渠道尚未登记，服务端按 ADR-0055 如实拒绝——<strong>这是诚实答案不是尚未
          实现</strong>，改请求或重试都不会改变结果。登记接入渠道认证参数（PAR-INT-01，实例
          半边）后由装配侧换上真 Intake 即放行；此前复核仍走受控登记 CLI。
        </p>
      );
    case 'callerProblem':
      return (
        <p className="text-xs text-idpxyz-danger">
          调用方式问题（HTTP {answer.status}）：{problemNote(answer.code)}
        </p>
      );
    case 'noAnswer':
      return (
        <p className="text-xs text-idpxyz-danger">
          服务端未形成答案（HTTP {answer.status}）：{problemNote(answer.code)}；复核记没记上
          未知，可稍后重试。
        </p>
      );
    case 'transport':
      return <p className="text-xs text-idpxyz-danger">请求未到达 parcel-api：{answer.message}</p>;
  }
}
