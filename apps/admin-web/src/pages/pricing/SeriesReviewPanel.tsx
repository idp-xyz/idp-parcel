import { useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Textarea } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import type { ApiResult } from '../catalogue-api';
import type { RegistrationResponseBody } from '../../components/registration';
import { OperatorRefusalNote } from '../../components/states';
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
 * **表单上没有「复核责任方」这一格，那不是漏做。** 租户的参考复核职责规则比的就是复核
 * 责任方与他在信封里持有的授予（ADR-0262 决定一），从浏览器收一个上去就是自报身份；传输层
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
  /** 登记责任方，只用于在面板上提示复核职责规则——不随请求送出。 */
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
          复核 <span className="font-mono type-value">{target.seriesId}</span>
          <span className="type-value text-idpxyz-textMuted"> 版本 {target.seriesVersion}</span>
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {/* 复核责任方与授予由接入渠道给，表单不收也不送——从浏览器收一个上去就是自报身份（ADR-0262 决定一）。 */}
        <p className="type-body leading-4 text-idpxyz-textMuted">
          核对这一版的每期数值与外部来源公布的是否一致。通过后，这一版从现在起参与计价；退回后，登记人要更正再登一版。
          复核只是另记一条结论，原来登记的数值不会被改。
          <br />
          这一版的登记人是 <span className="font-mono">{target.registrant}</span>。贵公司如果规定登记人不能自己复核，
          请换一位同事来做。
        </p>

        <div className="flex items-center gap-2">
          <span className="type-body leading-4 text-idpxyz-textMuted">结论</span>
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
          placeholder="理由（必填）：例如「已与来源公布的数值逐期核对一致」"
          rows={4}
          className="type-body leading-4"
        />

        <div className="flex items-center gap-3">
          <Button
            variant="default"
            className={primaryActionClass}
            onClick={send}
            disabled={!ready || state.kind === 'submitting'}
          >
            {state.kind === 'submitting' ? '提交中…' : '提交复核结论'}
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
      // 未收录的 outcome 原样示出原名：新增一格时宁可显示原名，也不把它归进某个既有中文说法——那会让一种新答案冒充另一种。
      return <p className="type-body leading-4 text-idpxyz-textMuted">{seriesReviewOutcomeLabels[outcome] ?? outcome}</p>;
    }
    case 'unconfigured':
      // 渠道未配置是诚实答案不是尚未实现（ADR-0055）：改请求或重试都不会变，续办在配置接入渠道。
      return (
        <p className="type-body leading-4 text-idpxyz-textMuted">
          访问通道尚未配置，这次复核没有记下；重试也不会变。请管理员先配置访问通道。
        </p>
      );
    case 'loginRequired':
    case 'notGranted':
      return <OperatorRefusalNote kind={answer.kind} what="复核" />;
    case 'callerProblem':
      return (
        <p className="type-body leading-4 text-idpxyz-danger">复核没有记下：{problemNote(answer.code)}</p>
      );
    case 'noAnswer':
      return (
        <p className="type-body leading-4 text-idpxyz-danger">
          这次没处理完，复核记没记上还不确定；稍后刷新列表确认，再决定要不要重提。（问题码 {answer.code}）
        </p>
      );
    case 'transport':
      return <p className="type-body leading-4 text-idpxyz-danger">连不上系统，复核没有提交（{answer.message}）；检查网络后重试。</p>;
  }
}
