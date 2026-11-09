import { useEffect, useState } from 'react';
import { X } from 'lucide-react';
import { Button, Dialog, DialogContent, Input } from '@idpxyz/ui-primitives';
import { WorkbenchPageTemplate, type WorkbenchColumn } from '../../templates';
import type { TemplateViewState } from '../../templates/state-slot';
import { useAddressKeyword } from '../../templates/use-address-keyword';
import { useOwnAddressChange } from '../../templates/use-own-address-change';
import { usePushedAddress } from '../../templates/use-pushed-address';
import { hashQueryValue, hashWithQueryValue } from '../../templates/address-query';
import { moduleInfoById } from '../../navigation';
import { primaryActionClass } from '../../components/primary-action';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { ensureSession } from '../../auth/oidc';
import type { ApiResult } from '../catalogue-api';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { CursorPager, pastFirstPage } from '../shipment-request/cursor-pager';
import { currentDisplayTimeZone, wallTimeOf, wallTimeToRfc3339 } from '../moment';
import { useOperatorAccessGrant } from '../use-operator-grants';
import { DetailRow } from '../party/detail-primitives';
import { RegisterView } from '../party/RegisterView';
import { selectClass } from '../party/PublicationFormFields';
import {
  listOperators,
  registerOperator,
  registerOperatorGrant,
  revokeOperatorGrant,
  type OperatorGrantRecord,
  type OperatorListResponseBody,
  type OperatorRecord,
  type OperatorWriteResponseBody,
} from './api';
import {
  effectiveGrantCount,
  faceLabel,
  grantQualifier,
  grantStandingAt,
  grantStandingLabels,
  operatorWriteOutcomeLabel,
  shortSubject,
  workScopeKindLabels,
  type GrantStanding,
} from './presentation';

// 主责出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['operator-access'];
const MODULE_ID = 'operator-access';
const VIEW_PARAM = 'tab';

const standingTone: Record<GrantStanding, 'good' | 'info' | 'warn' | 'muted'> = {
  effective: 'good',
  pending: 'info',
  expired: 'muted',
  revoked: 'warn',
};

const columns: WorkbenchColumn<OperatorRecord>[] = [
  {
    key: 'subject',
    header: '主体标识',
    render: (row) => (
      <span className="font-mono" title={row.subject}>
        {shortSubject(row.subject)}
      </span>
    ),
  },
  { key: 'basis', header: '登记依据', render: (row) => <span className="font-mono type-body">{row.basis}</span> },
  {
    key: 'grants',
    header: '生效中的授予',
    render: (row) => {
      const effective = row.grants.filter((grant) => grantStandingAt(grant, new Date()) === 'effective');
      return effective.length === 0 ? (
        <span className="text-idpxyz-textMuted">无</span>
      ) : (
        <span className="type-body">{Array.from(new Set(effective.map((grant) => faceLabel(grant.capabilityFace)))).join('、')}</span>
      );
    },
  },
  {
    key: 'recorded-at',
    header: '登记时间',
    hideWhenDetailOpen: true,
    render: (row) => <span className="font-mono type-body">{formatInstant(row.recordedAt)}</span>,
  },
];

/** 写口答复的一行：业务答案按代数译成中文；传输层各格照 ApiResult 的种类说续办。 */
function WriteAnswerLine({ answer }: { answer: ApiResult<OperatorWriteResponseBody> | null }) {
  if (answer === null) return null;
  switch (answer.kind) {
    case 'outcome':
      return (
        <p className={`type-body ${answer.body.outcome === 'RECORDED' ? 'text-idpxyz-text' : 'text-idpxyz-textMuted'}`}>
          {operatorWriteOutcomeLabel(answer.body.outcome)}
        </p>
      );
    case 'callerProblem':
      return <p className="type-body text-idpxyz-danger">请求的格式不对（{answer.code}）{answer.detail ? `：${answer.detail}` : ''}</p>;
    case 'notGranted':
      return <p className="type-body text-idpxyz-danger">你的账号没有操作者接入管理授予。</p>;
    case 'loginRequired':
      return <p className="type-body text-idpxyz-danger">登录已失效，请重新登录。</p>;
    case 'unconfigured':
      return <p className="type-body text-idpxyz-danger">操作者渠道尚未配置。</p>;
    default:
      return <p className="type-body text-idpxyz-danger">答案未形成，请稍后重试。</p>;
  }
}

/** 本地时刻输入：datetime-local 的值按显示时区折成 RFC3339；折不动（空或畸形）交回 null。 */
function WallTimeInput({ label, value, onChange, note }: { label: string; value: string; onChange: (value: string) => void; note?: string }) {
  return (
    <label className="flex flex-col gap-1 type-body">
      <span className="text-idpxyz-textMuted">{label}</span>
      <Input type="datetime-local" value={value} onChange={(event) => onChange(event.target.value)} className="font-mono type-body" />
      {note ? <span className="type-caption text-idpxyz-breadcrumb">{note}</span> : null}
    </label>
  );
}

function TextInput({ label, value, onChange, placeholder, mono = true }: { label: string; value: string; onChange: (value: string) => void; placeholder?: string; mono?: boolean }) {
  return (
    <label className="flex flex-col gap-1 type-body">
      <span className="text-idpxyz-textMuted">{label}</span>
      <Input value={value} onChange={(event) => onChange(event.target.value)} placeholder={placeholder} className={mono ? 'font-mono type-body' : 'type-body'} />
    </label>
  );
}

/**
 * 「授予」就地表单（ADR-0363 决定五）：面从读口交回的产品词表里选；运营决定要决定种类、作业事实要作业范围，别的面没有细分。
 * 一格不代填：起点、依据、授予标识都要人给；终点留空即不设终点。本组件不裁任何门——撞键、主体不在册、区间倒置由服务端答。
 */
function GrantForm({
  operator,
  vocabulary,
  onRecorded,
  onClose,
}: {
  operator: OperatorRecord;
  vocabulary: Pick<OperatorListResponseBody, 'faces' | 'decisionKinds' | 'workScopeKinds'>;
  onRecorded: () => void;
  onClose: () => void;
}) {
  const timeZone = currentDisplayTimeZone();
  const [face, setFace] = useState(vocabulary.faces[0] ?? '');
  const [decisionKind, setDecisionKind] = useState(vocabulary.decisionKinds[0] ?? '');
  const [scopeKind, setScopeKind] = useState(vocabulary.workScopeKinds[0] ?? '');
  const [scopeReference, setScopeReference] = useState('');
  const [grantId, setGrantId] = useState('');
  const [startsAt, setStartsAt] = useState(() => wallTimeOf(new Date(), timeZone));
  const [endsAt, setEndsAt] = useState('');
  const [basis, setBasis] = useState('');
  const [busy, setBusy] = useState(false);
  const [answer, setAnswer] = useState<ApiResult<OperatorWriteResponseBody> | null>(null);
  const startsRfc = wallTimeToRfc3339(startsAt, timeZone);
  const endsRfc = endsAt ? wallTimeToRfc3339(endsAt, timeZone) : null;
  const complete =
    grantId.trim() !== '' &&
    basis.trim() !== '' &&
    startsRfc !== null &&
    (endsAt === '' || endsRfc !== null) &&
    (face !== 'OPERATION_DECISION' || decisionKind !== '') &&
    (face !== 'WORK_FACT_REGISTRATION' || (scopeKind !== '' && scopeReference.trim() !== ''));
  const submit = () => {
    if (!complete || startsRfc === null) return;
    setBusy(true);
    void registerOperatorGrant({
      grantId: grantId.trim(),
      issuer: operator.issuer,
      subject: operator.subject,
      capabilityFace: face,
      ...(face === 'OPERATION_DECISION' ? { decisionKind } : {}),
      ...(face === 'WORK_FACT_REGISTRATION' ? { workScope: { kind: scopeKind, reference: scopeReference.trim() } } : {}),
      effectiveStartsAt: startsRfc,
      ...(endsRfc ? { effectiveEndsAt: endsRfc } : {}),
      basis: basis.trim(),
    }).then((next) => {
      setBusy(false);
      setAnswer(next);
      if (next.kind === 'outcome' && next.body.outcome === 'RECORDED') onRecorded();
    });
  };
  return (
    <section className="flex flex-col gap-3 rounded border border-idpxyz-border bg-idpxyz-inputBg p-3">
      <h3 className="type-value font-medium text-idpxyz-text">授予能力面</h3>
      <label className="flex flex-col gap-1 type-body">
        <span className="text-idpxyz-textMuted">能力面</span>
        <select className={selectClass} value={face} onChange={(event) => setFace(event.target.value)}>
          {vocabulary.faces.map((candidate) => (
            <option key={candidate} value={candidate}>
              {faceLabel(candidate)}
            </option>
          ))}
        </select>
      </label>
      {face === 'OPERATION_DECISION' ? (
        <label className="flex flex-col gap-1 type-body">
          <span className="text-idpxyz-textMuted">决定种类</span>
          <select className={selectClass} value={decisionKind} onChange={(event) => setDecisionKind(event.target.value)}>
            {vocabulary.decisionKinds.map((candidate) => (
              <option key={candidate} value={candidate}>
                {candidate}
              </option>
            ))}
          </select>
        </label>
      ) : null}
      {face === 'WORK_FACT_REGISTRATION' ? (
        <div className="grid grid-cols-2 gap-3">
          <label className="flex flex-col gap-1 type-body">
            <span className="text-idpxyz-textMuted">作业范围种类</span>
            <select className={selectClass} value={scopeKind} onChange={(event) => setScopeKind(event.target.value)}>
              {vocabulary.workScopeKinds.map((candidate) => (
                <option key={candidate} value={candidate}>
                  {workScopeKindLabels[candidate] ?? candidate}
                </option>
              ))}
            </select>
          </label>
          <TextInput label="作业范围引用" value={scopeReference} onChange={setScopeReference} placeholder="网点或场外作业范围标识" />
        </div>
      ) : null}
      <div className="grid grid-cols-2 gap-3">
        <TextInput label="授予标识" value={grantId} onChange={setGrantId} placeholder="本租户内唯一" />
        <TextInput label="依据" value={basis} onChange={setBasis} placeholder="批准单据或依据引用" />
        <WallTimeInput label="生效起点" value={startsAt} onChange={setStartsAt} note={`按本地时刻（${timeZone}）填。`} />
        <WallTimeInput label="生效终点" value={endsAt} onChange={setEndsAt} note="留空即不设终点。" />
      </div>
      <WriteAnswerLine answer={answer} />
      <div className="flex items-center justify-end gap-2">
        <Button variant="outline" size="sm" className="type-body" onClick={onClose}>
          取消
        </Button>
        <Button variant="default" size="sm" className={`${primaryActionClass} type-body`} disabled={!complete || busy} onClick={submit}>
          {busy ? '登记中…' : '登记授予'}
        </Button>
      </div>
    </section>
  );
}

/** 「撤销」对话框：撤销时刻与依据都要人给；答复原样示在框里，落册后页面重取。 */
function RevokeDialog({ grant, onClose, onRevoked }: { grant: OperatorGrantRecord | null; onClose: () => void; onRevoked: () => void }) {
  const timeZone = currentDisplayTimeZone();
  const [revokedAt, setRevokedAt] = useState('');
  const [basis, setBasis] = useState('');
  const [busy, setBusy] = useState(false);
  const [answer, setAnswer] = useState<ApiResult<OperatorWriteResponseBody> | null>(null);
  // 换一条、或关掉再开：时刻、依据、答复与在途标记都回到初态，上一条的东西不留。
  useEffect(() => {
    setRevokedAt(wallTimeOf(new Date(), timeZone));
    setBasis('');
    setAnswer(null);
    setBusy(false);
  }, [grant?.grantId]);
  const revokedRfc = wallTimeToRfc3339(revokedAt, timeZone);
  const complete = grant !== null && basis.trim() !== '' && revokedRfc !== null;
  const submit = () => {
    if (!complete || grant === null || revokedRfc === null) return;
    setBusy(true);
    void revokeOperatorGrant({ grantId: grant.grantId, revokedAt: revokedRfc, basis: basis.trim() }).then((next) => {
      setBusy(false);
      setAnswer(next);
      if (next.kind === 'outcome' && next.body.outcome === 'RECORDED') onRevoked();
    });
  };
  return (
    <Dialog open={grant !== null} onOpenChange={(open) => (open ? undefined : onClose())}>
      <DialogContent size="md" className="p-0" aria-label="撤销授予">
        {grant ? (
          <div className="flex flex-col gap-3 p-5">
            <h2 className="type-title font-semibold text-idpxyz-textBright">撤销授予 {grant.grantId}？</h2>
            <p className="type-body text-idpxyz-textMuted">
              {faceLabel(grant.capabilityFace)}
              {grantQualifier(grant) ? ` · ${grantQualifier(grant)}` : ''}。撤销自撤销时刻起生效，写入后没有撤回的入口。
            </p>
            <WallTimeInput label="撤销时刻" value={revokedAt} onChange={setRevokedAt} note={`默认是打开这个窗口的时刻；按本地时刻（${timeZone}）填。`} />
            <TextInput label="依据" value={basis} onChange={setBasis} placeholder="撤销的单据或依据引用" />
            <WriteAnswerLine answer={answer} />
            <div className="flex items-center justify-end gap-2">
              <Button variant="outline" size="sm" className="type-body" onClick={onClose}>
                再想想
              </Button>
              <Button variant="default" size="sm" className={`${primaryActionClass} type-body`} disabled={!complete || busy} onClick={submit}>
                {busy ? '撤销中…' : '确认撤销'}
              </Button>
            </div>
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

/**
 * 右栏：绑定事实、授予表、「授予…」与各条「撤销…」。没有管理授予（readOnly）时只看。落册后经 onWritten 重取列表当前这一页：
 * 列表按游标翻页、不经取数层（ADR-0360），没有缓存可供写口作废，只能由页面自己重取。
 */
function OperatorDetailPanel({
  operator,
  vocabulary,
  readOnly,
  onClose,
  onWritten,
}: {
  operator: OperatorRecord;
  vocabulary: Pick<OperatorListResponseBody, 'faces' | 'decisionKinds' | 'workScopeKinds'>;
  readOnly: boolean;
  onClose: () => void;
  onWritten: () => void;
}) {
  const [granting, setGranting] = useState(false);
  const [revoking, setRevoking] = useState<OperatorGrantRecord | null>(null);
  const now = new Date();
  return (
    <div className="flex h-full w-full flex-col bg-idpxyz-bg" aria-label={`操作者 ${operator.subject}`}>
      <div className="space-y-2 border-b border-idpxyz-border px-5 pb-3 pt-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 className="break-all font-mono type-title font-semibold text-idpxyz-textBright">{operator.subject}</h2>
            <p className="mt-0.5 type-body text-idpxyz-textMuted">操作者 · 生效中的授予 {effectiveGrantCount(operator, now)} 条</p>
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            {!readOnly && !granting ? (
              <Button variant="outline" size="sm" className="type-body" onClick={() => setGranting(true)}>
                授予…
              </Button>
            ) : null}
            <Button className="text-idpxyz-breadcrumb" variant="ghost" size="icon" onClick={onClose} aria-label="关闭详情" title="关闭（Esc）">
              <X className="h-4 w-4" aria-hidden="true" />
            </Button>
          </div>
        </div>
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-5 py-3">
        <dl>
          <DetailRow label="发行方" mono>
            {operator.issuer}
          </DetailRow>
          <DetailRow label="登记依据" mono>
            {operator.basis}
          </DetailRow>
          <DetailRow label="登记时间" mono>
            {formatInstant(operator.recordedAt)}
          </DetailRow>
        </dl>
        {granting ? (
          <GrantForm
            operator={operator}
            vocabulary={vocabulary}
            onRecorded={() => {
              setGranting(false);
              onWritten();
            }}
            onClose={() => setGranting(false)}
          />
        ) : null}
        <section className="flex flex-col gap-2">
          <h3 className="type-body font-medium text-idpxyz-text">授予</h3>
          {operator.grants.length === 0 ? (
            <p className="type-body text-idpxyz-textMuted">还没有任何授予：这个操作者登录后什么也做不了。</p>
          ) : (
            <ul className="flex flex-col gap-2">
              {operator.grants.map((grant) => {
                const standing = grantStandingAt(grant, now);
                return (
                  <li key={grant.grantId} className="flex flex-col gap-1 rounded border border-idpxyz-border p-3 type-body">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-medium text-idpxyz-text">{faceLabel(grant.capabilityFace)}</span>
                        {grantQualifier(grant) ? <span className="font-mono text-idpxyz-textMuted">{grantQualifier(grant)}</span> : null}
                        <StatusPill label={grantStandingLabels[standing]} tone={standingTone[standing]} />
                      </div>
                      {!readOnly && (standing === 'effective' || standing === 'pending') ? (
                        <Button variant="outline" size="sm" className="type-body" onClick={() => setRevoking(grant)}>
                          撤销…
                        </Button>
                      ) : null}
                    </div>
                    <p className="font-mono text-idpxyz-textMuted">
                      {formatInstant(grant.effectiveStartsAt)} → {grant.effectiveEndsAt ? formatInstant(grant.effectiveEndsAt) : '不设终点'}
                    </p>
                    {grant.revocation ? (
                      <p className="font-mono text-idpxyz-textMuted">
                        撤销于 {formatInstant(grant.revocation.revokedAt)} · 依据 {grant.revocation.basis}
                      </p>
                    ) : null}
                    <TechnicalDetails
                      rows={[
                        { label: '授予标识', value: grant.grantId },
                        { label: '依据', value: grant.basis },
                        { label: '登记时间', value: grant.recordedAt },
                      ]}
                    />
                  </li>
                );
              })}
            </ul>
          )}
        </section>
      </div>
      <RevokeDialog
        grant={revoking}
        onClose={() => setRevoking(null)}
        onRevoked={() => {
          setRevoking(null);
          onWritten();
        }}
      />
    </div>
  );
}

/** 「新增操作者」登记视图里的表单：发行方从读口预填，主体标识与依据要人给；落册后经 onRecorded 记下目录要重取，回到目录由人决定。 */
function OperatorRegistrationForm({ issuer, onRecorded }: { issuer: string; onRecorded: () => void }) {
  const [subject, setSubject] = useState('');
  const [basis, setBasis] = useState('');
  const [busy, setBusy] = useState(false);
  const [answer, setAnswer] = useState<ApiResult<OperatorWriteResponseBody> | null>(null);
  const complete = subject.trim() !== '' && basis.trim() !== '' && issuer !== '';
  const submit = () => {
    if (!complete) return;
    setBusy(true);
    void registerOperator({ issuer, subject: subject.trim(), basis: basis.trim() }).then((next) => {
      setBusy(false);
      setAnswer(next);
      if (next.kind === 'outcome' && next.body.outcome === 'RECORDED') onRecorded();
    });
  };
  return (
    <div className="flex-1 overflow-auto p-4">
      <section className="mx-auto flex max-w-3xl flex-col gap-3 rounded border border-idpxyz-border bg-idpxyz-bg p-4">
        <p className="type-body text-idpxyz-textMuted">
          主体标识是发行方给这个账号的 <span className="font-mono">sub</span>，不是邮箱：让对方先登录一次，在本页的拒绝提示里抄给你。登记后还要给授予，
          否则登录了也什么都做不了。
        </p>
        <label className="flex flex-col gap-1 type-body">
          <span className="text-idpxyz-textMuted">发行方</span>
          <Input value={issuer} readOnly className="font-mono type-body" />
        </label>
        <TextInput label="主体标识" value={subject} onChange={setSubject} placeholder="发行方签发的 sub" />
        <TextInput label="登记依据" value={basis} onChange={setBasis} placeholder="入职单、授权函等依据引用" />
        <WriteAnswerLine answer={answer} />
        <div className="flex items-center justify-end">
          <Button variant="default" size="sm" className={`${primaryActionClass} type-body`} disabled={!complete || busy} onClick={submit}>
            {busy ? '登记中…' : '登记操作者'}
          </Button>
        </div>
      </section>
    </div>
  );
}

/** 当前会话的发行方与主体标识：从 ID token 的声明解出，只显示、不上送。 */
function useOwnSubject(): { issuer: string; subject: string } | null {
  const [own, setOwn] = useState<{ issuer: string; subject: string } | null>(null);
  useEffect(() => {
    let cancelled = false;
    void ensureSession().then((session) => {
      if (cancelled || !session) return;
      const issuer = typeof session.claims.iss === 'string' ? session.claims.iss : '';
      const subject = typeof session.claims.sub === 'string' ? session.claims.sub : '';
      setOwn({ issuer, subject });
    });
    return () => {
      cancelled = true;
    };
  }, []);
  return own;
}

/**
 * 操作者与授予（ADR-0363 决定五）：工作台列本租户的操作者，右栏显绑定与授予、就地授予、对话框撤销；「新增操作者」是
 * `?tab=register` 的登记视图。没有操作者接入管理授予的人不摆写入口（ADR-0315 同款）；读口答未授予时，状态区把本人的发行方
 * 与主体标识显出来——未在册的人靠它把标识交给管理员登记。
 *
 * 列表按游标翻页（ADR-0144，票 catalogue-read-pagination-rollout/14），次序是读口的缺省序（登记先后）。检索词（地址 `?q=`）停稳后
 * 下推成 q，读口只在主体标识与绑定依据上找：授予的标识与能力面挂在各个操作者名下、不是这一册的列（accessidentity 的
 * OperatorCataloguePages）。这一页原先在页内也按授予与能力面找，迁过来就只搜这两格——检索框的提示照实只写这两格，收窄在框上
 * 看得见；页内不再补搜，补搜只对这一页成立，会让人以为整册都按授予找过了。
 */
export function OperatorAccessPage() {
  const grant = useOperatorAccessGrant();
  const mayWrite = grant.mayAdminister;
  const [hash, setHash] = useState(() => window.location.hash);
  useOwnAddressChange(() => setHash(window.location.hash));
  const requested = hashQueryValue(hash, VIEW_PARAM) === 'register' ? 'register' : 'catalogue';
  const view = mayWrite ? requested : 'catalogue';
  useEffect(() => {
    if (requested === 'register' && grant.settled && !grant.mayAdminister) {
      const next = hashWithQueryValue(window.location.hash, VIEW_PARAM, '');
      window.location.replace(next);
      setHash(next);
    }
  }, [requested, grant.settled, grant.mayAdminister]);
  const pushed = usePushedAddress();
  const openRegister = () => pushed.push(hashWithQueryValue(window.location.hash, VIEW_PARAM, 'register'));
  const leaveRegister = () => {
    if (pushed.back()) return;
    const next = hashWithQueryValue(window.location.hash, VIEW_PARAM, '');
    window.location.replace(next);
    setHash(next);
  };

  const [search, setSearch] = useAddressKeyword();
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const conditions = { q: useSettledKeyword(search) };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listOperators);
  const own = useOwnSubject();
  const body = answer?.kind === 'outcome' ? answer.body : null;
  const operators = body?.operators ?? [];
  const keyOf = (operator: OperatorRecord) => `${operator.issuer}|${operator.subject}`;
  let viewState: TemplateViewState = catalogueViewState(answer, operators.length, retry, {
    module: info,
    endpoint: 'GET /operators',
    emptyTitle: '还没有任何操作者',
    emptyDescription: '租户的第一个操作者与它的操作者接入管理授予由实施方经受控命令行登记；之后在这里新增。',
    narrowed: catalogueConditionsNarrow(conditions) || pastFirstPage(pagination),
  });
  // 登记视图里落了册，回到目录时才重取当前这一页：当场重取的话，登记表单上取自答复的发行方一格会随重取闪空。
  const [registeredAway, setRegisteredAway] = useState(false);
  useEffect(() => {
    if (view !== 'catalogue' || !registeredAway) return;
    setRegisteredAway(false);
    retry();
    // retry 每次渲染是新函数，只认视图与落册标记。
  }, [view, registeredAway]);
  if (viewState.kind === 'notGranted' && own) {
    viewState = {
      ...viewState,
      description: `这一页只对持操作者接入管理授予的操作者开放。你的发行方：${own.issuer}；主体标识：${own.subject}——交给管理员即可登记或授予。`,
    };
  }

  if (view === 'register') {
    return (
      <RegisterView title="新增操作者" subtitle="把一个已在发行方登录过的账号登进本租户的操作者册；授予在目录的右栏里给" onBack={leaveRegister}>
        <OperatorRegistrationForm issuer={body?.issuer ?? ''} onRecorded={() => setRegisteredAway(true)} />
      </RegisterView>
    );
  }

  return (
    <WorkbenchPageTemplate<OperatorRecord>
      title={info.title}
      viewStateKey="operator-access"
      subtitle={info.summary}
      onRefresh={retry}
      refreshing={answer === null}
      primaryAction={
        mayWrite ? (
          <Button variant="default" className={primaryActionClass} onClick={openRegister}>
            新增操作者
          </Button>
        ) : undefined
      }
      search={{ value: search, onChange: setSearch, placeholder: '搜主体标识、登记依据' }}
      toolbarRight={pagination ? <CursorPager pagination={pagination} /> : undefined}
      columns={columns}
      rows={operators}
      rowKey={keyOf}
      selectedKey={selectedKey}
      onSelect={setSelectedKey}
      noMatchText="没有匹配的操作者"
      onClearFilters={() => setSearch('')}
      renderDetail={(key) => {
        const operator = operators.find((candidate) => keyOf(candidate) === key);
        if (!operator || !body) return null;
        return (
          <OperatorDetailPanel
            key={key}
            operator={operator}
            vocabulary={body}
            readOnly={!mayWrite}
            onClose={() => setSelectedKey(null)}
            onWritten={retry}
          />
        );
      }}
      viewState={viewState}
    />
  );
}

