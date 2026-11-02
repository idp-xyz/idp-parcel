import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { isOperatorRefusal } from '../../auth/access-refusal';
import { OperatorRefusalNote, SectionError, TechnicalDetails, httpRows } from '../../components/states';
import type { ApiResult } from '../catalogue-api';
import { resolveLegalEntityProfile, type LegalEntityProfileResolutionResponseBody } from './api';
import { problemNote } from './presentation';

export interface CurrentLegalEntityProfileQuery {
  legalEntityId?: string;
  answer: ApiResult<LegalEntityProfileResolutionResponseBody> | null;
  retry: () => void;
}

/** 计数、概要与联系人共用当前答案；新一轮读取不能沿用旧主体或旧修订的计数。 */
export function useCurrentLegalEntityProfile(legalEntityId: string | undefined, refreshKey: string | number = 0): CurrentLegalEntityProfileQuery {
  const [reloadKey, setReloadKey] = useState(0);
  const retry = useCallback(() => setReloadKey((value) => value + 1), []);
  const request = useMemo(() => ({ legalEntityId, refreshKey, reloadKey }), [legalEntityId, refreshKey, reloadKey]);
  const [result, setResult] = useState<{
    request: typeof request;
    answer: ApiResult<LegalEntityProfileResolutionResponseBody>;
  } | null>(null);

  useEffect(() => {
    if (!request.legalEntityId) return;
    let cancelled = false;
    void resolveLegalEntityProfile(request.legalEntityId, new Date().toISOString()).then((next) => {
      if (!cancelled) setResult({ request, answer: next });
    });
    return () => { cancelled = true; };
  }, [request]);

  const answer = result?.request === request
    ? result.answer
    : null;
  return { legalEntityId, answer, retry };
}

/** 读失败不能当成「未配置联系人」；读口的拒绝和未知状态原样区分。 */
export function CurrentLegalEntityProfile({ query, children }: {
  query: CurrentLegalEntityProfileQuery;
  children: (body: LegalEntityProfileResolutionResponseBody) => ReactNode;
}) {
  const { legalEntityId, answer, retry } = query;
  if (answer === null) return <p className="text-[12px] text-idpxyz-breadcrumb">正在读取当前法人资料…</p>;
  if (answer.kind === 'unconfigured') {
    return <p className="text-[13px] text-idpxyz-breadcrumb">法人资料读取渠道尚未配置，暂时无法读取。</p>;
  }
  if (isOperatorRefusal(answer)) {
    return <OperatorRefusalNote kind={answer.kind} what="法人资料" className="text-[12px]" />;
  }
  if (answer.kind === 'callerProblem') {
    return (
      <div>
        <p className="text-[12px] text-idpxyz-breadcrumb">调用方式问题：{problemNote(answer.code)}</p>
        <TechnicalDetails className="mt-2" rows={httpRows(answer.status, answer.code)} />
      </div>
    );
  }
  if (answer.kind === 'noAnswer') {
    return (
      <SectionError
        title="法人资料读取失败"
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
    return <p className="text-[12px] text-idpxyz-breadcrumb">答案回显的法人与当前页面不符，已丢弃。</p>;
  }
  return <>{children(answer.body)}</>;
}
