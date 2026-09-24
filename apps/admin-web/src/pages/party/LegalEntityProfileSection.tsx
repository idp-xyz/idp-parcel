import { useEffect, useState } from 'react';
import { Button, Card, CardContent, Input, Textarea } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { RegistrationAnswerNote } from '../../components/registration';
import { SectionError } from '../../components/states';
import type { ApiResult } from '../catalogue-api';
import { currentDisplayTimeZone, formatInstant } from '../moment';
import {
  commercialRegistrationEndpoints,
  legalEntityProfileOutcomeLabels,
  listLegalEntityProfileRevisions,
  resolveLegalEntityProfile,
  type LegalEntityProfileResolutionResponseBody,
  type LegalEntityProfileRevisionListResponseBody,
  type LegalEntityProfileRevisionRecord,
} from './api';
import { labelOf, problemNote, profileIncompleteCauseLabels, profileResolutionOutcomeLabels } from './presentation';
import { Field, Problems, fieldLabel } from './PublicationFormFields';
import { RevisionField, WallTimeField, useRegistrationForm } from './party-registration-fields';
import { RevisionHistorySection, type RevisionHistoryRegister } from './RevisionHistorySection';
import {
  emptyLegalEntityProfileDraft,
  legalEntityProfileLocalProblems,
  legalEntityProfilePayloadOf,
  profileContentLines,
  profileRevisionHistoryNote,
  profileRevisionTimeline,
  resolutionInstantOf,
  suggestedProfileRevision,
  type ContactDraft,
  type LegalEntityProfileDraft,
  type TaxNumberDraft,
} from './legal-entity-profile';

// 法人详情抽屉的「法人资料」区（票 legal-entity-profile/04 第 2 项；ADR-0145 决定三、五、六）：当前有效的那一修订、修订历史、
// 登记新修订的表单。判读与载荷在 legal-entity-profile.ts（纯函数，node:test 钉着），这里只摆。
//
// 调用方按法人给 key：换一个法人就整块重挂，表单草稿里钉的法人标识不会停在上一个法人身上。

const info = moduleInfoById['group-legal-entities'];
const kind = 'legal-entity-profile';
const endpoint = `POST ${commercialRegistrationEndpoints[kind]}`;
const note = 'mt-1 text-[12px] text-idpxyz-textMuted';

const profileRevisionHistory: RevisionHistoryRegister<LegalEntityProfileRevisionListResponseBody> = {
  subject: '法人',
  endpoint: 'GET /commercial-group-legal-entities/{legalEntityId}/profile-revisions',
  load: listLegalEntityProfileRevisions,
  echoedSubjectId: (body) => body.legalEntityId,
  timelineOf: (body) => profileRevisionTimeline(body.revisions, (iso) => formatInstant(iso)),
  noteOf: profileRevisionHistoryNote,
};

export function LegalEntityProfileSection({ legalEntityId }: { legalEntityId: string }) {
  // 登记落册一次加一：当前有效区与历史区都按它重取，新登的那笔立刻可见。
  const [landed, setLanded] = useState(0);
  const [formOpen, setFormOpen] = useState(false);
  return (
    <>
      <h4 className="mt-2 text-[12px] font-medium text-idpxyz-text">当前有效</h4>
      <CurrentProfile legalEntityId={legalEntityId} landed={landed} />
      <h4 className="mt-4 text-[12px] font-medium text-idpxyz-text">资料修订历史</h4>
      <RevisionHistorySection register={profileRevisionHistory} subjectId={legalEntityId} revision={landed} />
      <div className="mt-4">
        {formOpen ? (
          <ProfileRegistrationForm legalEntityId={legalEntityId} onLanded={() => setLanded((count) => count + 1)} />
        ) : (
          <Button variant="outline" onClick={() => setFormOpen(true)}>
            登记新的资料修订
          </Button>
        )}
      </div>
    </>
  );
}

/**
 * 当前有效区：按时点解析（留空即此刻）。答资料不全照写原因——那是明确的非成功，开立方据此拒开，这里不拿默认值补。
 * 时点可改到未来，看登记了未来生效的那笔到时会不会接上。
 */
function CurrentProfile({ legalEntityId, landed }: { legalEntityId: string; landed: number }) {
  const timeZone = currentDisplayTimeZone();
  const [at, setAt] = useState('');
  const [answer, setAnswer] = useState<ApiResult<LegalEntityProfileResolutionResponseBody> | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const instant = resolutionInstantOf(at, timeZone, new Date());
  const askable = instant !== null;

  useEffect(() => {
    // 时刻在 effect 里现取：「此刻」每次渲染都不同，放进依赖会一直重取。
    const moment = resolutionInstantOf(at, timeZone, new Date());
    if (moment === null) return;
    let cancelled = false;
    setAnswer(null);
    void resolveLegalEntityProfile(legalEntityId, moment).then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [legalEntityId, at, timeZone, landed, reloadKey]);

  const retry = () => setReloadKey((value) => value + 1);
  return (
    <div className="mt-1 flex flex-col gap-2">
      <div className="max-w-80">
        <WallTimeField
          label="按时点查看（留空即此刻）"
          path="at"
          problems={askable ? {} : { at: ['时点要是可解析的日期时间'] }}
          value={at}
          locked={false}
          timeZone={timeZone}
          onChange={setAt}
        />
      </div>
      {askable ? <ResolutionAnswer answer={answer} legalEntityId={legalEntityId} retry={retry} /> : null}
    </div>
  );
}

function ResolutionAnswer({
  answer,
  legalEntityId,
  retry,
}: {
  answer: ApiResult<LegalEntityProfileResolutionResponseBody> | null;
  legalEntityId: string;
  retry: () => void;
}) {
  if (answer === null) return <p className={note}>正在按时点解析法人资料…</p>;
  if (answer.kind === 'unconfigured') {
    return (
      <p className={note}>
        访问通道尚未配置：法人资料按时点解析读口当前不可用（403）。这不是「资料不全」——今天没有问到；配置该上下文的访问通道后重新打开抽屉。
      </p>
    );
  }
  if (answer.kind === 'callerProblem') {
    return (
      <p className={note}>
        调用方式问题（HTTP {answer.status}）：{problemNote(answer.code)}
      </p>
    );
  }
  if (answer.kind === 'noAnswer') {
    return (
      <SectionError title={`服务端未形成答案（HTTP ${answer.status}）`} description={problemNote(answer.code)} onRetry={retry} />
    );
  }
  if (answer.kind === 'transport') {
    return <SectionError title="无法连接主数据读取服务" description={answer.message} onRetry={retry} />;
  }
  const body = answer.body;
  if (body.legalEntityId !== legalEntityId) {
    return (
      <p className={note}>
        答案回显的法人（{body.legalEntityId}）与所问（{legalEntityId}）不符，已丢弃。
      </p>
    );
  }
  const effective = body.effectiveRevision;
  return (
    <div className="text-[12px]">
      <p>
        {labelOf(profileResolutionOutcomeLabels, body.outcome)}
        {body.incompleteCause !== undefined ? `：${labelOf(profileIncompleteCauseLabels, body.incompleteCause)}` : ''}
        <span className="ml-2 text-idpxyz-textMuted">问的时刻 {formatInstant(body.at)}</span>
      </p>
      {effective !== undefined ? (
        <dl className="mt-2 grid grid-cols-[6rem_1fr] gap-x-3 gap-y-1">
          <dt className="text-idpxyz-textMuted">生效修订</dt>
          <dd className="font-mono">
            r{effective.revision} · 生效自 {formatInstant(effective.effectiveFrom)} · 依据 {effective.basis}
          </dd>
          {profileContentLines(effective).map((line) => (
            <FragmentRow key={line.label} label={line.label} value={line.value} />
          ))}
        </dl>
      ) : null}
    </div>
  );
}

function FragmentRow({ label, value }: { label: string; value: string }) {
  return (
    <>
      <dt className="text-idpxyz-textMuted">{label}</dt>
      <dd className="font-mono">{value}</dd>
    </>
  );
}

/**
 * 登记新修订的逐字段表单。本地只拦编码层的三件（legalEntityProfileLocalProblems）；地址国家与身份对不对得上、税号合不合
 * 目录、修订连不连续都由服务端答，答复原样示出。修订号建议取这个法人已登的资料修订，落册后重取。
 */
function ProfileRegistrationForm({ legalEntityId, onLanded }: { legalEntityId: string; onLanded: () => void }) {
  const [known, setKnown] = useState<LegalEntityProfileRevisionRecord[] | null>(null);
  const [refresh, setRefresh] = useState(0);
  useEffect(() => {
    let cancelled = false;
    void listLegalEntityProfileRevisions(legalEntityId).then((answer) => {
      if (!cancelled) setKnown(answer.kind === 'outcome' ? answer.body.revisions : null);
    });
    return () => {
      cancelled = true;
    };
  }, [legalEntityId, refresh]);

  const form = useRegistrationForm<LegalEntityProfileDraft>({
    kind,
    empty: () => emptyLegalEntityProfileDraft(legalEntityId),
    suggestion: () => suggestedProfileRevision(known, legalEntityId),
    localProblems: legalEntityProfileLocalProblems,
    payloadOf: legalEntityProfilePayloadOf,
    landedOutcome: 'REGISTERED',
    onLanded: () => {
      setRefresh((value) => value + 1);
      onLanded();
    },
  });
  const { draft, patch, problems, locked, timeZone } = form;

  const setTax = (index: number, change: Partial<TaxNumberDraft>) =>
    patch({ taxNumbers: draft.taxNumbers.map((row, at) => (at === index ? { ...row, ...change } : row)) });
  const setContact = (index: number, change: Partial<ContactDraft>) =>
    patch({ contacts: draft.contacts.map((row, at) => (at === index ? { ...row, ...change } : row)) });

  return (
    <Card>
      <CardContent className="flex flex-col gap-3 pt-4">
        <p className="text-xs text-idpxyz-textMuted">
          一笔登记一个修订，修订号连续；新修订自其生效时点起取代前一修订，可以登记未来生效的修订，登过的不被覆盖。
          注册地址的国家 / 地区须与身份上的注册国家 / 地区一致——由服务端判。提交打到 <span className="font-mono">{endpoint}</span>。
        </p>
        <div className="grid grid-cols-2 gap-3">
          <RevisionField
            path="profiles[0].revision"
            problems={problems}
            {...form.revisionField}
            note="建议值取这个法人已登资料修订的最新一笔 + 1（没有为 1）；连续性由服务端按册面判。"
          />
          <Field label="登记依据 *" path="profiles[0].basis" problems={problems}>
            <Input
              value={draft.basis}
              readOnly={locked}
              className="font-mono text-[13px]"
              placeholder="资料登记依据引用"
              onChange={(event) => patch({ basis: event.target.value })}
            />
          </Field>
          <WallTimeField
            label="生效自（可在未来；留空由服务端点名）"
            path="profiles[0].effectiveFrom"
            problems={problems}
            value={draft.effectiveFrom}
            locked={locked}
            timeZone={timeZone}
            onChange={(effectiveFrom) => patch({ effectiveFrom })}
          />
          <Field label="注册地址国家 / 地区 *" path="profiles[0].registeredAddress" problems={problems}>
            <Input
              value={draft.addressCountry}
              readOnly={locked}
              className="font-mono text-[13px]"
              placeholder="如 CN、SG"
              onChange={(event) => patch({ addressCountry: event.target.value })}
            />
          </Field>
          <Field label="注册地址（一行一段） *" path="profiles[0].registeredAddress.lines" problems={problems}>
            <Textarea
              value={draft.addressLines}
              readOnly={locked}
              rows={3}
              onChange={(event) => patch({ addressLines: event.target.value })}
            />
          </Field>
          <Field label="开票抬头" path="profiles[0].invoiceTitle" problems={problems}>
            <Input
              value={draft.invoiceTitle}
              readOnly={locked}
              placeholder="开票用的法人名称；留空即这笔不带开票资料"
              onChange={(event) => patch({ invoiceTitle: event.target.value })}
            />
            <span className="block text-[11px] text-idpxyz-textMuted mt-1">
              不带开票资料的那笔生效后，按时点解析答「资料不全」，开立方据此拒开。
            </span>
          </Field>
        </div>

        <div>
          <p className={fieldLabel}>税务登记号（类型码取注册号类型目录的资料层，由服务端判）</p>
          {draft.taxNumbers.map((row, index) => (
            <div key={index} className="mb-2 grid grid-cols-[1fr_1fr_auto] gap-3">
              <Input
                value={row.typeCode}
                readOnly={locked}
                className="font-mono text-[13px]"
                placeholder="类型码"
                onChange={(event) => setTax(index, { typeCode: event.target.value })}
              />
              <Input
                value={row.number}
                readOnly={locked}
                className="font-mono text-[13px]"
                placeholder="号"
                onChange={(event) => setTax(index, { number: event.target.value })}
              />
              <Button
                variant="ghost"
                disabled={locked}
                onClick={() => patch({ taxNumbers: draft.taxNumbers.filter((_, at) => at !== index) })}
              >
                移除
              </Button>
            </div>
          ))}
          <Problems lines={problems['profiles[0].taxRegistrationNumbers']} />
          <Button
            variant="outline"
            disabled={locked}
            onClick={() => patch({ taxNumbers: [...draft.taxNumbers, { typeCode: '', number: '' }] })}
          >
            添加税务登记号
          </Button>
        </div>

        <div>
          <p className={fieldLabel}>联系人（可空）</p>
          {draft.contacts.map((row, index) => (
            <div key={index} className="mb-2 grid grid-cols-[1fr_1fr_1fr_auto] gap-3">
              <Input
                value={row.name}
                readOnly={locked}
                placeholder="姓名"
                onChange={(event) => setContact(index, { name: event.target.value })}
              />
              <Input
                value={row.email}
                readOnly={locked}
                placeholder="邮箱（可空）"
                onChange={(event) => setContact(index, { email: event.target.value })}
              />
              <Input
                value={row.phone}
                readOnly={locked}
                placeholder="电话（可空）"
                onChange={(event) => setContact(index, { phone: event.target.value })}
              />
              <Button
                variant="ghost"
                disabled={locked}
                onClick={() => patch({ contacts: draft.contacts.filter((_, at) => at !== index) })}
              >
                移除
              </Button>
            </div>
          ))}
          <Button
            variant="outline"
            disabled={locked}
            onClick={() => patch({ contacts: [...draft.contacts, { name: '', email: '', phone: '' }] })}
          >
            添加联系人
          </Button>
        </div>

        <div className="flex items-start gap-3">
          <Button onClick={form.send} disabled={locked || !form.canSend}>
            {locked ? '提交中…' : '提交资料修订'}
          </Button>
          <RegistrationAnswerNote
            state={form.state}
            owner={info.owner}
            outcomeLabels={legalEntityProfileOutcomeLabels}
            problemNote={problemNote}
          />
        </div>
      </CardContent>
    </Card>
  );
}
