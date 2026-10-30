import { useEffect, useState, type ReactNode } from 'react';
import { isOperatorRefusal } from '../../auth/access-refusal';
import { StatusPill } from '../../components/StatusPill';
import { OperatorRefusalNote, SectionError, TechnicalDetails, httpRows } from '../../components/states';
import type { ApiResult } from '../catalogue-api';
import { formatInstant } from '../moment';
import {
  resolveLegalEntityProfile,
  type LegalEntityProfileResolutionResponseBody,
} from './api';
import { problemNote, profileRevisionLabel } from './presentation';
import {
  legalEntityProfileStatusOf,
  profileContentLines,
} from './legal-entity-profile';

/** 法人首屏要能判断“现在能不能拿来开票”：这里只读当前时点，不把完整资料表单塞进概要。 */
export function LegalEntityProfileSummary({
  legalEntityId,
  refreshKey = 0,
}: {
  legalEntityId: string;
  refreshKey?: number;
}) {
  const [answer, setAnswer] = useState<ApiResult<LegalEntityProfileResolutionResponseBody> | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    const at = new Date().toISOString();
    let cancelled = false;
    setAnswer(null);
    void resolveLegalEntityProfile(legalEntityId, at).then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [legalEntityId, refreshKey, reloadKey]);

  if (answer === null) {
    return <SummaryShell><p className="text-[12px] text-idpxyz-breadcrumb">正在读取当前法人资料…</p></SummaryShell>;
  }
  if (answer.kind === 'unconfigured') {
    return (
      <SummaryShell>
        <SummaryHeading label="暂时无法读取" tone="warn" />
        <p className="mt-2 text-[12px] text-idpxyz-breadcrumb">法人资料读取渠道尚未配置，这不等同于资料不全。</p>
      </SummaryShell>
    );
  }
  if (isOperatorRefusal(answer)) {
    return (
      <SummaryShell>
        <OperatorRefusalNote kind={answer.kind} what="法人资料摘要" className="text-[12px]" />
      </SummaryShell>
    );
  }
  if (answer.kind === 'callerProblem') {
    return (
      <SummaryShell>
        <p className="text-[12px] text-idpxyz-breadcrumb">调用方式问题：{problemNote(answer.code)}</p>
        <TechnicalDetails className="mt-2" rows={httpRows(answer.status, answer.code)} />
      </SummaryShell>
    );
  }
  if (answer.kind === 'noAnswer') {
    return (
      <SummaryShell>
        <SectionError
          title="法人资料读取失败"
          description={problemNote(answer.code)}
          technical={httpRows(answer.status, answer.code)}
          onRetry={() => setReloadKey((value) => value + 1)}
        />
      </SummaryShell>
    );
  }
  if (answer.kind === 'transport') {
    return (
      <SummaryShell>
        <SectionError title="无法连接主数据读取服务" description={answer.message} onRetry={() => setReloadKey((value) => value + 1)} />
      </SummaryShell>
    );
  }

  const body = answer.body;
  if (body.legalEntityId !== legalEntityId) {
    return (
      <SummaryShell>
        <p className="text-[12px] text-idpxyz-breadcrumb">答案回显的法人与当前页面不符，已丢弃。</p>
      </SummaryShell>
    );
  }

  const status = legalEntityProfileStatusOf(body);
  const effective = body.effectiveRevision;
  return (
    <SummaryShell>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-3">
          <h3 className="text-[14px] font-semibold leading-5 text-idpxyz-textBright">当前法人资料</h3>
          <SummaryHeading label={status.label} tone={status.tone} />
        </div>
        <span className="text-[12px] text-idpxyz-breadcrumb">按当前时点 · {formatInstant(body.at)}</span>
      </div>
      <p className="mt-2 text-[13px] leading-5 text-idpxyz-breadcrumb">
        {status.description}
      </p>
      <dl className="mt-4 grid grid-cols-[repeat(auto-fit,minmax(min(100%,26rem),1fr))] gap-x-8">
        <SummaryField label="资料修订" value={effective ? profileRevisionLabel(effective.revision) : '未配置'} mono={Boolean(effective)} />
        <SummaryField label="生效时间" value={effective ? formatInstant(effective.effectiveFrom) : '未配置'} mono={Boolean(effective)} />
        <SummaryField label="资料依据" value={effective?.basis ?? '未配置'} mono={Boolean(effective)} />
        {(effective ? profileContentLines(effective) : emptyProfileLines()).map((line) => (
          <SummaryField key={line.label} label={line.label} value={line.value} mono={line.label === '税务登记号'} emphasized={line.label === '开票抬头'} />
        ))}
      </dl>
    </SummaryShell>
  );
}

function emptyProfileLines() {
  return [
    { label: '注册地址', value: '未配置' },
    { label: '税务登记号', value: '未配置' },
    { label: '开票抬头', value: '未配置' },
    { label: '联系人', value: '未配置' },
  ];
}

function SummaryShell({ children }: { children: ReactNode }) {
  return <section>{children}</section>;
}

function SummaryHeading({ label, tone }: { label: string; tone: 'good' | 'info' | 'warn' | 'bad' | 'muted' }) {
  return <StatusPill label={`资料状态：${label}`} tone={tone} />;
}

function SummaryField({ label, value, mono = false, emphasized = false }: { label: string; value: string; mono?: boolean; emphasized?: boolean }) {
  return (
    <div className="grid min-w-0 grid-cols-[5.5rem_minmax(0,1fr)] items-start gap-x-3 border-b border-idpxyz-border py-2.5 last:border-b-0">
      <dt className="text-[12px] leading-5 text-idpxyz-breadcrumb">{label}</dt>
      <dd
        className={`min-w-0 break-words text-[13px] leading-5 ${
          value === '未配置' ? 'text-idpxyz-breadcrumb' : 'text-idpxyz-text'
        } ${mono ? 'font-mono' : ''} ${emphasized ? 'font-medium' : ''}`}
      >
        {value}
      </dd>
    </div>
  );
}
