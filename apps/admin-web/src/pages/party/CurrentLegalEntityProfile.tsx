import type { ReactNode } from 'react';
import { isOperatorRefusal } from '../../auth/access-refusal';
import { OperatorRefusalNote, SectionError, TechnicalDetails, httpRows } from '../../components/states';
import { revealClass, useDelayedReveal } from '../../components/use-delayed-reveal';
import type { ApiResult } from '../catalogue-api';
import type { LegalEntityProfileResolutionResponseBody } from './api';
import { useLegalEntityRead } from './legal-entity-reads';
import { problemNote } from './presentation';

export interface CurrentLegalEntityProfileQuery {
  legalEntityId?: string;
  answer: ApiResult<LegalEntityProfileResolutionResponseBody> | null;
  retry: () => void;
}

/**
 * 计数、概要与联系人共用当前答案；换了公司或身份版本就是另一把键，不沿用旧主体或旧修订的那份（legal-entity-reads.ts）。
 * 资料更新落册后整域作废（party-writes.ts），这里不另拧钥匙。不给 `version` 的（资料表单要拿此刻的当前资料预填）单占一格，
 * 并带 `fresh`：打开表单必现问，问到之前不拿缓存那份预填。
 */
export function useCurrentLegalEntityProfile(
  legalEntityId: string | undefined,
  version?: string,
  options?: { fresh?: boolean },
): CurrentLegalEntityProfileQuery {
  const { answer, retry } = useLegalEntityRead('profile', legalEntityId, version, options);
  return { legalEntityId, answer, retry };
}

/**
 * 还没答到时的占位：与读出同样的三块卡轮廓，高度相近。只摆一行「正在读取…」时，右栏先塌成一行、答案到了再整块撑开，
 * 看着是一闪（用户 2026-11-03 指出）。读屏只念一句状态。答得快时不露（components/use-delayed-reveal.ts），位置照占。
 */
function ProfileReadoutSkeleton() {
  const revealed = useDelayedReveal();
  return (
    <div role="status" aria-label="正在读取当前公司资料" className={`mt-3 flex animate-pulse flex-col gap-3 ${revealClass(revealed)}`}>
      {[4, 3, 2].map((lines, index) => (
        <div key={index} className="overflow-hidden rounded-md border border-idpxyz-border">
          <div className="h-9 border-b border-idpxyz-border bg-idpxyz-editor" />
          <div className="flex flex-col gap-2.5 px-4 py-3">
            {Array.from({ length: lines }, (_, line) => (
              <div key={line} className="flex gap-4">
                <div className="h-3.5 w-24 shrink-0 rounded bg-idpxyz-hover" />
                <div className="h-3.5 flex-1 rounded bg-idpxyz-hover" style={{ maxWidth: `${60 - line * 10}%` }} />
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

/** 读失败不能当成「未配置联系人」；读口的拒绝和未知状态原样区分。 */
export function CurrentLegalEntityProfile({ query, children }: {
  query: CurrentLegalEntityProfileQuery;
  children: (body: LegalEntityProfileResolutionResponseBody) => ReactNode;
}) {
  const { legalEntityId, answer, retry } = query;
  if (answer === null) return <ProfileReadoutSkeleton />;
  if (answer.kind === 'unconfigured') {
    return <p className="type-value text-idpxyz-breadcrumb">公司资料读取渠道尚未配置，暂时无法读取。</p>;
  }
  if (isOperatorRefusal(answer)) {
    return <OperatorRefusalNote kind={answer.kind} what="公司资料" className="type-body" />;
  }
  if (answer.kind === 'callerProblem') {
    return (
      <div>
        <p className="type-body text-idpxyz-breadcrumb">调用方式问题：{problemNote(answer.code)}</p>
        <TechnicalDetails className="mt-2" rows={httpRows(answer.status, answer.code)} />
      </div>
    );
  }
  if (answer.kind === 'noAnswer') {
    return (
      <SectionError
        title="公司资料读取失败"
        description={problemNote(answer.code)}
        technical={httpRows(answer.status, answer.code)}
        onRetry={retry}
      />
    );
  }
  if (answer.kind === 'transport') {
    return (
      <SectionError
        title="无法连接主数据读取服务"
        description={answer.message}
        onRetry={retry}
      />
    );
  }
  if (answer.body.legalEntityId !== legalEntityId) {
    return <p className="type-body text-idpxyz-breadcrumb">答案回显的公司主体与当前页面不符，已丢弃。</p>;
  }
  return <>{children(answer.body)}</>;
}
