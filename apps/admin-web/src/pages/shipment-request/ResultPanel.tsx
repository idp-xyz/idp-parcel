import type { ReactNode } from 'react';
import { OperatorRefusalState, UnconfiguredState } from '../../components/states';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import type { ApiResult } from './api';
import { answerTechnicalRows, problemMessage } from './words';
import { NoticeCard } from './controls';

// ApiResult 各格的统一呈现(格的划分见 api.ts)。业务答案(2xx)长什么样由页面
// 各自渲染——提交与撤回的封闭词汇表不同,合并渲染就得开「其他」格,那正是
// ADR-0022 不允许的口子;答案之外的各格在各页语义相同,集中在这里。
// 正文只说下一步做什么;状态码与问题码收进「技术详情」。
export function ResultPanel<Body>({
  result,
  pending,
  renderOutcome,
}: {
  result: ApiResult<Body> | null;
  pending: boolean;
  renderOutcome: (body: Body, status: number) => ReactNode;
}) {
  if (pending) {
    return (
      <div className="rounded border border-idpxyz-border p-4 text-[13px] text-idpxyz-textMuted">
        正在提交…
      </div>
    );
  }
  if (!result) return null;

  switch (result.kind) {
    case 'outcome':
      return <>{renderOutcome(result.body, result.status)}</>;

    case 'unconfigured':
      // 403 + ACCESS_CHANNEL_NOT_CONFIGURED:改请求或重试都不会好,要去配置接入
      // 渠道。组件契约归 components/states,这里只按名使用。
      return <UnconfiguredState />;

    case 'loginRequired':
    case 'notGranted':
      // 401 要重新登录、403 OPERATOR_NOT_GRANTED 要登记授予:同样不是重发能好的事,与上一格同属接入那一格。
      return <OperatorRefusalState kind={result.kind} />;

    case 'callerProblem':
      // 调用方的错(4xx):答案没形成,但重发同样内容不会改变结果——交给人改,
      // 呈现为可改正的陈述,不是系统故障。
      return (
        <NoticeCard title="没有受理">
          <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-2">{problemMessage(result.code)}</p>
          <TechnicalDetails rows={answerTechnicalRows(result)} />
        </NoticeCard>
      );

    case 'noAnswer':
      return (
        <NoticeCard title="这次没有处理成功">
          <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-2">
            {problemMessage(result.code)} 这次没有形成任何业务决定,重试是安全的,不会重复建单。
          </p>
          <TechnicalDetails rows={answerTechnicalRows(result)} />
        </NoticeCard>
      );

    case 'transport':
      return (
        <NoticeCard title="没有连上服务器">
          <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-2">请求没有送出去。请检查网络后再试;一直连不上请联系管理员。</p>
          <TechnicalDetails rows={answerTechnicalRows(result)} />
        </NoticeCard>
      );
  }
}
