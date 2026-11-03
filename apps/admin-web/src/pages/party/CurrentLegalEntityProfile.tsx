import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import { isOperatorRefusal } from '../../auth/access-refusal';
import { OperatorRefusalNote, SectionError, TechnicalDetails, httpRows } from '../../components/states';
import { revealClass, useDelayedReveal } from '../../components/use-delayed-reveal';
import type { ApiResult } from '../catalogue-api';
import { resolveLegalEntityProfile, type LegalEntityProfileResolutionResponseBody } from './api';
import { cachedLegalEntityRead, loadLegalEntityRead } from './legal-entity-read-cache';
import { problemNote } from './presentation';

export interface CurrentLegalEntityProfileQuery {
  legalEntityId?: string;
  answer: ApiResult<LegalEntityProfileResolutionResponseBody> | null;
  retry: () => void;
}

/**
 * 计数、概要与联系人共用当前答案；新一轮读取不能沿用旧主体或旧修订的计数。
 *
 * 给了 `version`（身份版本，`legalEntityReadVersion`）就走页面内缓存：这家公司这一版读过的那份先显出来，同时照常再问一遍、
 * 答到了再替换——左边换行时右栏不再先清空成「正在读取…」再撑开（legal-entity-read-cache.ts 文件头）。不给就每次现问，
 * 如资料表单要拿最新的那一版预填。
 */
export function useCurrentLegalEntityProfile(
  legalEntityId: string | undefined,
  refreshKey: string | number = 0,
  version?: string,
): CurrentLegalEntityProfileQuery {
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
    const load =
      version === undefined
        ? resolveLegalEntityProfile(request.legalEntityId, new Date().toISOString())
        : loadLegalEntityRead('profile', request.legalEntityId, version);
    void load.then((next) => {
      if (!cancelled) setResult({ request, answer: next });
    });
    return () => { cancelled = true; };
  }, [request, version]);

  const cached = legalEntityId !== undefined && version !== undefined ? cachedLegalEntityRead('profile', legalEntityId, version) : undefined;
  const answer = result?.request === request ? result.answer : (cached ?? null);
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
    return <p className="text-[13px] text-idpxyz-breadcrumb">公司资料读取渠道尚未配置，暂时无法读取。</p>;
  }
  if (isOperatorRefusal(answer)) {
    return <OperatorRefusalNote kind={answer.kind} what="公司资料" className="text-[12px]" />;
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
    return <p className="text-[12px] text-idpxyz-breadcrumb">答案回显的公司主体与当前页面不符，已丢弃。</p>;
  }
  return <>{children(answer.body)}</>;
}
