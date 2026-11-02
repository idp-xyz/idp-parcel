import { useEffect, useMemo, useState } from 'react';
import { isOperatorRefusal } from '../../auth/access-refusal';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@idpxyz/ui-primitives';
import { OperatorRefusalNote, SectionError, TechnicalDetails, httpRows } from '../../components/states';
import type { ApiResult } from '../catalogue-api';
import { formatInstant } from '../moment';
import { addressText, contactPurposeLabels, representativeLabel } from './legal-entity-profile';
import {
  compareLegalEntityProfiles, orderedProfileRevisions, profileComparisonSummary,
  profileRevisionState, selectedProfileRevision, type ProfileChangeKind, type ProfileFieldChange,
} from './legal-entity-profile-history';
import {
  listLegalEntityProfileRevisions, resolveLegalEntityProfile,
  type LegalEntityProfileResolutionResponseBody, type LegalEntityProfileRevisionListResponseBody,
  type LegalEntityProfileRevisionRecord,
} from './api';
import { problemNote } from './presentation';

const note = 'text-[12px] text-idpxyz-breadcrumb';
const kindLabels: Record<ProfileChangeKind, string> = {
  added: '新增', modified: '已修改', removed: '移除', unchanged: '未变化',
};
const kindClasses: Record<ProfileChangeKind, string> = {
  added: 'text-emerald-700 dark:text-emerald-300',
  modified: 'font-medium text-idpxyz-accent',
  removed: 'text-idpxyz-breadcrumb',
  unchanged: 'text-idpxyz-breadcrumb',
};

export function LegalEntityProfileHistory({ legalEntityId, revision }: { legalEntityId: string; revision: number }) {
  const [reloadKey, setReloadKey] = useState(0);
  const [selectedNumber, setSelectedNumber] = useState<number | null>(null);
  const [baselineNumber, setBaselineNumber] = useState<number | null | undefined>(undefined);
  const [changedOnly, setChangedOnly] = useState(true);
  const requestKey = `${legalEntityId}\u0000${revision}\u0000${reloadKey}`;
  const [historyResult, setHistoryResult] = useState<{ key: string; answer: ApiResult<LegalEntityProfileRevisionListResponseBody> }>();
  const [currentResult, setCurrentResult] = useState<{ key: string; answer: ApiResult<LegalEntityProfileResolutionResponseBody> }>();
  const history = historyResult?.key === requestKey ? historyResult.answer : null;
  const current = currentResult?.key === requestKey ? currentResult.answer : null;

  useEffect(() => {
    let cancelled = false;
    void listLegalEntityProfileRevisions(legalEntityId).then((answer) => {
      if (!cancelled) setHistoryResult({ key: requestKey, answer });
    });
    return () => { cancelled = true; };
  }, [legalEntityId, requestKey]);

  useEffect(() => {
    let cancelled = false;
    void resolveLegalEntityProfile(legalEntityId, new Date().toISOString()).then((answer) => {
      if (!cancelled) setCurrentResult({ key: requestKey, answer });
    });
    return () => { cancelled = true; };
  }, [legalEntityId, requestKey]);

  const retry = () => setReloadKey((value) => value + 1);
  const historyBody = history?.kind === 'outcome' && history.body.legalEntityId === legalEntityId
    ? history.body
    : undefined;
  const revisions = useMemo(() => orderedProfileRevisions(historyBody?.revisions ?? []), [historyBody]);
  const currentBody = current?.kind === 'outcome' && current.body.legalEntityId === legalEntityId
    ? current.body
    : undefined;
  const currentRevision = currentBody?.effectiveRevision?.revision;
  const selected = selectedProfileRevision(revisions, selectedNumber, currentRevision);

  if (history === null) return <p role="status" className={note}>正在读取资料版本…</p>;
  if (isOperatorRefusal(history)) return <OperatorRefusalNote kind={history.kind} what="资料版本" className={note} />;
  if (history.kind === 'unconfigured') return <p className={note}>资料版本读取渠道尚未配置，暂时无法查看。</p>;
  if (history.kind === 'callerProblem') return <div className={note}><p>读取方式问题：{problemNote(history.code)}</p><TechnicalDetails rows={httpRows(history.status, history.code)} /></div>;
  if (history.kind === 'noAnswer') return <SectionError title="资料版本读取失败" description={problemNote(history.code)} technical={httpRows(history.status, history.code)} onRetry={retry} />;
  if (history.kind === 'transport') return <SectionError title="无法连接主数据读取服务" description={history.message} onRetry={retry} />;
  if (history.kind === 'outcome' && history.body.legalEntityId !== legalEntityId) return <SectionError title="返回的法人资料与当前对象不符" description="为避免展示其他对象的资料，已丢弃这次答案。" onRetry={retry} />;
  if (revisions.length === 0) return <p className={note}>该法人尚无资料修订。</p>;

  const selectedIndex = selected ? revisions.findIndex((row) => row.revision === selected.revision) : -1;
  const defaultBaseline = selectedIndex >= 0 ? revisions[selectedIndex + 1] : undefined;
  const baseline = baselineNumber === undefined
    ? defaultBaseline
    : revisions.find((row) => row.revision === baselineNumber);
  const comparison = selected && baseline ? compareLegalEntityProfiles(baseline, selected) : undefined;
  const differences = comparison?.fields ?? [];
  const visibleDifferences = changedOnly ? differences.filter((field) => field.kind !== 'unchanged') : differences;
  const resolutionIsUsable = currentBody !== undefined && ['RESOLVED', 'PROFILE_INCOMPLETE'].includes(currentBody.outcome);
  const currentReadProblem = current === null ? null
    : current === undefined ? '正在判读当前生效版本…'
    : current.kind === 'outcome' && current.body.legalEntityId !== legalEntityId ? '返回的法人资料与当前对象不符，版本状态未判读。'
    : current.kind === 'outcome' ? current.body.outcome === 'RESOLVED' || current.body.outcome === 'PROFILE_INCOMPLETE' ? null : '当前生效状态暂不可判读。'
    : current.kind === 'transport' ? '无法连接当前资料读取服务，版本状态暂不可判读。'
    : current.kind === 'noAnswer' ? `当前生效状态读取失败：${problemNote(current.code)}`
    : current.kind === 'callerProblem' ? `当前生效状态读取受拒：${problemNote(current.code)}`
    : current.kind === 'unconfigured' ? '当前资料读取渠道尚未配置，版本状态暂不可判读。'
    : '当前资料读取权限不足，版本状态暂不可判读。';

  const selectRevision = (number: number) => {
    setSelectedNumber(number);
    setBaselineNumber(undefined);
  };
  return (
    <div className="flex min-w-0 flex-col gap-4">
      {currentReadProblem ? <p role="status" className={note}>{currentReadProblem}</p> : null}
      <div className="grid min-w-0 gap-4 rounded-lg border border-idpxyz-border bg-idpxyz-panel p-4 md:grid-cols-[minmax(11rem,0.7fr)_minmax(0,1.3fr)]">
        <div className="min-w-0">
          <label htmlFor="legal-entity-profile-revision" className="text-[12px] font-medium text-idpxyz-text">选择资料版本</label>
          <select
            id="legal-entity-profile-revision"
            value={selected?.revision ?? ''}
            onChange={(event) => selectRevision(Number(event.target.value))}
            className="mt-1.5 h-9 w-full min-w-0 rounded-md border border-idpxyz-border bg-idpxyz-bg px-2 text-[13px] text-idpxyz-text focus-visible:outline focus-visible:outline-2 focus-visible:outline-idpxyz-accent"
          >
            {revisions.map((row, index) => {
              const rowState = profileRevisionState(row, resolutionIsUsable ? currentBody : undefined);
              const summary = index === revisions.length - 1 ? '首次登记' : profileComparisonSummary(compareLegalEntityProfiles(revisions[index + 1], row));
              return <option key={row.revision} value={row.revision}>资料 r{row.revision} · {rowState.label} · {summary}</option>;
            })}
          </select>
          <ol aria-label="资料版本登记顺序" className="mt-3 space-y-1.5">
            {revisions.map((row, index) => {
              const rowState = profileRevisionState(row, resolutionIsUsable ? currentBody : undefined);
              const active = selected?.revision === row.revision;
              const summary = index === revisions.length - 1 ? '首次登记' : profileComparisonSummary(compareLegalEntityProfiles(revisions[index + 1], row));
              return (
                <li key={row.revision}>
                  <button
                    type="button"
                    aria-current={active ? 'true' : undefined}
                    aria-label={`资料 r${row.revision}，${rowState.label}，${summary}`}
                    onClick={() => selectRevision(row.revision)}
                    className={`w-full rounded-md border px-3 py-2 text-left transition-colors motion-reduce:transition-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-idpxyz-accent ${active ? 'border-idpxyz-accent bg-idpxyz-activeItem' : 'border-transparent hover:bg-idpxyz-hoverItem'}`}
                  >
                    <span className="flex items-center justify-between gap-2">
                      <span className="text-[13px] font-semibold text-idpxyz-text">资料 r{row.revision}</span>
                      <span className={`rounded-full bg-idpxyz-bg px-2 py-0.5 text-[11px] ${rowState.tone === 'current' ? 'font-medium text-idpxyz-accent' : 'text-idpxyz-breadcrumb'}`}>{rowState.label}</span>
                    </span>
                    <span className="mt-1 block truncate text-[12px] text-idpxyz-breadcrumb">{summary}</span>
                    <span className="mt-1 block text-[11px] text-idpxyz-breadcrumb">生效 {formatInstant(row.effectiveFrom)}</span>
                  </button>
                </li>
              );
            })}
          </ol>
        </div>
        {selected ? (
          <RevisionDetails
            selected={selected}
            baseline={baseline}
            baselineNumber={baselineNumber}
            baselineOptions={revisions.filter((row) => row.revision !== selected.revision)}
            comparison={comparison}
            fields={visibleDifferences}
            changedOnly={changedOnly}
            onBaselineChange={(value) => setBaselineNumber(value === '' ? null : Number(value))}
            onChangedOnlyChange={setChangedOnly}
          />
        ) : null}
      </div>
    </div>
  );
}

function RevisionDetails({
  selected, baseline, baselineNumber, baselineOptions, comparison, fields, changedOnly,
  onBaselineChange, onChangedOnlyChange,
}: {
  selected: LegalEntityProfileRevisionRecord;
  baseline?: LegalEntityProfileRevisionRecord;
  baselineNumber: number | null | undefined;
  baselineOptions: LegalEntityProfileRevisionRecord[];
  comparison?: ReturnType<typeof compareLegalEntityProfiles>;
  fields: ProfileFieldChange[];
  changedOnly: boolean;
  onBaselineChange: (value: string) => void;
  onChangedOnlyChange: (value: boolean) => void;
}) {
  return (
    <section aria-label={`资料 r${selected.revision} 详情`} className="min-w-0 border-t border-idpxyz-border pt-4 md:border-l md:border-t-0 md:pl-4 md:pt-0">
      <div className="flex min-w-0 flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <h5 className="text-[14px] font-semibold text-idpxyz-text">资料 r{selected.revision}</h5>
          <p className="mt-1 break-words text-[12px] text-idpxyz-breadcrumb">{baseline ? profileComparisonSummary(comparison) : '首次登记，无前一版本可比较'}</p>
        </div>
        <span className="shrink-0 rounded-md bg-idpxyz-bg px-2 py-1 text-[11px] text-idpxyz-breadcrumb">{formatInstant(selected.registeredAt)}</span>
      </div>
      <dl className="mt-3 grid min-w-0 grid-cols-1 gap-x-4 gap-y-2 text-[12px] sm:grid-cols-2">
        <Metadata label="生效时间" value={formatInstant(selected.effectiveFrom)} />
        <Metadata label="登记时间" value={formatInstant(selected.registeredAt)} />
        <Metadata label="登记依据" value={selected.basis} wide />
      </dl>

      {baseline ? (
        <div className="mt-4 flex min-w-0 flex-wrap items-end gap-3 border-t border-idpxyz-border pt-3">
          <label className="min-w-0 flex-1 text-[12px] text-idpxyz-breadcrumb">
            比较基准
            <select
              value={baselineNumber === undefined ? baseline.revision : baselineNumber ?? ''}
              onChange={(event) => onBaselineChange(event.target.value)}
              aria-label="选择比较基准版本"
              className="mt-1 block h-8 w-full rounded-md border border-idpxyz-border bg-idpxyz-bg px-2 text-idpxyz-text focus-visible:outline focus-visible:outline-2 focus-visible:outline-idpxyz-accent"
            >
              {baselineOptions.map((row) => <option key={row.revision} value={row.revision}>资料 r{row.revision} · {formatInstant(row.effectiveFrom)}</option>)}
              <option value="">不比较</option>
            </select>
          </label>
          <label className="inline-flex min-h-8 items-center gap-2 pb-1 text-[12px] text-idpxyz-text">
            <input type="checkbox" checked={changedOnly} onChange={(event) => onChangedOnlyChange(event.target.checked)} className="size-4 accent-idpxyz-accent focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-idpxyz-accent" />
            只看变化
          </label>
        </div>
      ) : null}

      {baseline ? (
        <div className="mt-2 min-w-0">
          {comparison?.contactMatchingLimited ? <p role="note" className="mb-2 rounded-md bg-idpxyz-bg px-2.5 py-2 text-[11px] text-idpxyz-breadcrumb">资料中没有联系人的稳定标识；无法唯一对应的联系人按移除和新增列出，不推测为同一人。</p> : null}
          {fields.length ? (
            <div className="max-h-[min(45vh,26rem)] overflow-auto rounded-md border border-idpxyz-border">
              <Table className="min-w-[34rem] text-left text-[12px]">
                <TableHeader className="sticky top-0 z-10 bg-idpxyz-bg">
                  <TableRow><TableHead>项目</TableHead><TableHead>上一版 · r{baseline.revision}</TableHead><TableHead>当前选择 · r{selected.revision}</TableHead></TableRow>
                </TableHeader>
                <TableBody>{fields.map((field) => <ChangeRow key={field.id} field={field} />)}</TableBody>
              </Table>
            </div>
          ) : <p role="status" className="py-3 text-[12px] text-idpxyz-breadcrumb">{changedOnly ? '这两个版本之间没有资料变化。关闭「只看变化」可查看全部项目。' : '没有可显示的资料项目。'}</p>}
        </div>
      ) : null}

      <details className="mt-3 border-t border-idpxyz-border pt-3">
        <summary className="cursor-pointer text-[12px] font-medium text-idpxyz-text focus-visible:outline focus-visible:outline-2 focus-visible:outline-idpxyz-accent">查看完整资料快照</summary>
        <Snapshot revision={selected} />
      </details>
    </section>
  );
}

function ChangeRow({ field }: { field: ProfileFieldChange }) {
  const label = field.contactKey ? `${field.subject}${field.detail ? `（${field.detail}）` : ''} · ${field.field}` : field.field;
  return (
    <TableRow>
      <TableCell className="min-w-28 align-top">
        <span>{label}</span>
        {field.kind !== 'unchanged' ? <span className={`mt-0.5 block ${kindClasses[field.kind]}`}>{kindLabels[field.kind]}</span> : null}
      </TableCell>
      <TableCell className={`max-w-64 whitespace-pre-wrap [overflow-wrap:anywhere] align-top ${field.kind === 'removed' || field.kind === 'modified' ? 'line-through decoration-idpxyz-border' : 'text-idpxyz-breadcrumb'}`}>{field.before || '—'}</TableCell>
      <TableCell className={`max-w-64 whitespace-pre-wrap [overflow-wrap:anywhere] align-top ${field.kind === 'added' || field.kind === 'modified' ? 'font-medium text-idpxyz-text' : 'text-idpxyz-breadcrumb'}`}>{field.after || '—'}</TableCell>
    </TableRow>
  );
}

function Metadata({ label, value, wide = false }: { label: string; value: string; wide?: boolean }) {
  return <div className={`min-w-0 ${wide ? 'sm:col-span-2' : ''}`}><dt className="text-[11px] text-idpxyz-breadcrumb">{label}</dt><dd className="mt-0.5 break-words text-idpxyz-text">{value}</dd></div>;
}

function Snapshot({ revision }: { revision: LegalEntityProfileRevisionRecord }) {
  const representative = revision.representative ? `${representativeLabel(revision.representative.kind)} · ${revision.representative.name}` : '未配置';
  const contacts = [...revision.contacts].sort((a, b) => a.name.localeCompare(b.name));
  return (
    <div className="mt-3 max-h-[min(50vh,30rem)] space-y-4 overflow-auto pr-1 text-[12px]">
      <section><h6 className="font-medium text-idpxyz-text">代表人信息</h6><p className="mt-1 break-words text-idpxyz-breadcrumb">{representative}</p></section>
      <section><h6 className="font-medium text-idpxyz-text">地址信息</h6><dl className="mt-1 grid gap-2 sm:grid-cols-2"><Metadata label="注册地址" value={addressText(revision.registeredAddress)} /><Metadata label="办公地址" value={addressText(revision.officeAddress)} /><Metadata label="通信地址" value={addressText(revision.correspondenceAddress)} /></dl></section>
      <section><h6 className="font-medium text-idpxyz-text">税务与开票</h6><dl className="mt-1 grid gap-2 sm:grid-cols-2"><Metadata label="税务登记号" value={revision.taxRegistrationNumbers.map((row) => `${row.typeCode} ${row.number}`).join('；') || '未配置'} /><Metadata label="开票资料" value={revision.invoicingRegistered ? '已登记' : '未登记'} /><Metadata label="开票抬头" value={revision.invoiceTitle || '未配置'} /></dl></section>
      <section><h6 className="font-medium text-idpxyz-text">联系人 · {contacts.length}</h6>{contacts.length ? <ul className="mt-2 space-y-2">{contacts.map((contact, index) => <li key={`${contact.name}-${contact.email ?? ''}-${contact.phone ?? ''}-${index}`} className="rounded-md border border-idpxyz-border p-2.5"><p className="font-medium text-idpxyz-text">{contact.name}</p><p className="mt-1 break-words text-idpxyz-breadcrumb">{[contact.department, contact.position, contact.email, contact.phone, (contact.purposes ?? []).map((purpose) => contactPurposeLabels[purpose] ?? purpose).join('、'), contact.primaryFor?.length ? `主要联系人：${contact.primaryFor.map((purpose) => contactPurposeLabels[purpose] ?? purpose).join('、')}` : ''].filter(Boolean).join(' · ') || '未配置其他联系资料'}</p>{contact.socialAccounts?.length ? <ul className="mt-2 space-y-1">{contact.socialAccounts.map((account, accountIndex) => <li key={`${account.platform}-${account.account}-${accountIndex}`} className="break-all"><span className="text-idpxyz-breadcrumb">{account.platform}　</span><span className="text-idpxyz-text">{account.account}</span></li>)}</ul> : null}</li>)}</ul> : <p className="mt-1 text-idpxyz-breadcrumb">未配置联系人</p>}</section>
    </div>
  );
}
