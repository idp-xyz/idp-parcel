import { useEffect, useRef, useState } from 'react';
import { Button, Card, CardContent, Input, Textarea } from '@idpxyz/ui-primitives';
import { isOperatorRefusal } from '../../auth/access-refusal';
import { moduleInfoById } from '../../navigation';
import { primaryActionClass } from '../../components/primary-action';
import { RegistrationAnswerNote } from '../../components/registration';
import { OperatorRefusalNote, SectionError, TechnicalDetails, httpRows } from '../../components/states';
import type { ApiResult } from '../catalogue-api';
import { currentDisplayTimeZone, formatInstant } from '../moment';
import {
  legalEntityProfileOutcomeLabels,
  listLegalEntityProfileRevisions,
  resolveLegalEntityProfile,
  type LegalEntityProfileResolutionResponseBody,
  type LegalEntityProfileRevisionListResponseBody,
  type LegalEntityProfileRevisionRecord,
} from './api';
import {
  labelOf,
  problemNote,
  profileIncompleteCauseLabels,
  profileResolutionOutcomeLabels,
  profileRevisionLabel,
} from './presentation';
import { Field, Problems, fieldLabel } from './PublicationFormFields';
import { AdvancedGroup, FormGroup, RevisionField, WallTimeField, useRegistrationForm } from './party-registration-fields';
import { advancedGroupOpen } from './registration-form';
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
import { useMayWriteRegistry } from '../use-operator-grants';

// 法人对象页概要中的「开票资料」区（票 legal-entity-profile/04 第 2 项；ADR-0145 决定三、五、六）：资料修订历史与登记新修订的表单。
// 当前时点摘要由 LegalEntityProfileSummary 负责；判读与载荷在 legal-entity-profile.ts（纯函数，node:test 钉着），这里只摆。
//
// 调用方按法人给 key：换一个法人就整块重挂，表单草稿里钉的法人标识不会停在上一个法人身上。

const info = moduleInfoById['group-legal-entities'];
const kind = 'legal-entity-profile';
const note = 'mt-1 text-[12px] text-idpxyz-breadcrumb';
// 资料表单收在高级组里的那一格；组里有问题时组一律展开。
const profileAdvancedPaths = ['profiles[0].revision'];

const profileRevisionHistory: RevisionHistoryRegister<LegalEntityProfileRevisionListResponseBody> = {
  subject: '法人',
  endpoint: 'GET /commercial-group-legal-entities/{legalEntityId}/profile-revisions',
  load: listLegalEntityProfileRevisions,
  echoedSubjectId: (body) => body.legalEntityId,
  timelineOf: (body) => profileRevisionTimeline(body.revisions, (iso) => formatInstant(iso)),
  noteOf: profileRevisionHistoryNote,
};

export function LegalEntityProfileSection({
  legalEntityId,
  formOpenRequest = 0,
  collapseCurrentProfile = false,
  onProfileUpdated,
}: {
  legalEntityId: string;
  /** 调用方每要一次「直接打开登记表」就加一（工作台详情栏的主动作）；0 即没要过。 */
  formOpenRequest?: number;
  /** 对象页已有当前资料摘要时折叠时点查询，仍保留历史与未来时点的查看入口。 */
  collapseCurrentProfile?: boolean;
  onProfileUpdated?: () => void;
}) {
  // 登记落册一次加一：当前有效区与历史区都按它重取，新登的那笔立刻可见。
  const [landed, setLanded] = useState(0);
  const [lookupVisited, setLookupVisited] = useState(false);
  // 没有登记册配置写授予的操作者不摆登记表与打开它的按钮（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  const [formOpen, setFormOpen] = useState(formOpenRequest > 0);
  const formRef = useRef<HTMLElement>(null);
  // 展开与滚动分两拍（票 group-legal-entities-uiux/07）：同一拍里先 setFormOpen 再滚，滚的时候表单还没渲染，内容不够高，
  // 滚动距离被截断，表单停在视野底边外。只有区块挂载时就带着「已请求」初值那一回碰巧对。这里请求只记一笔「要滚」，
  // 等展开那次提交之后再由下一个 effect 去滚。
  const [scrollRequest, setScrollRequest] = useState(0);
  useEffect(() => {
    if (formOpenRequest === 0) return;
    setFormOpen(true);
    setScrollRequest(formOpenRequest);
  }, [formOpenRequest]);
  useEffect(() => {
    if (scrollRequest === 0) return;
    formRef.current?.scrollIntoView({ block: 'start' });
  }, [scrollRequest]);
  return (
    <div className="flex flex-col gap-5">
      {collapseCurrentProfile ? (
        <details
          onToggle={(event) => {
            if (event.currentTarget.open) setLookupVisited(true);
          }}
        >
          <summary className="cursor-pointer text-[13px] font-medium text-idpxyz-text focus-visible:outline focus-visible:outline-2 focus-visible:outline-idpxyz-accent">
            按时点查看法人资料
          </summary>
          {lookupVisited ? <CurrentProfile legalEntityId={legalEntityId} landed={landed} /> : null}
        </details>
      ) : (
        <section>
          <div className="flex items-start justify-between gap-3">
            <div>
              <h4 className="text-[13px] font-semibold text-idpxyz-text">当前有效资料</h4>
              <p className="mt-1 text-xs text-idpxyz-breadcrumb">开票时按当时生效的那一版资料；可以提前登记未来生效的资料。</p>
            </div>
          </div>
          <CurrentProfile legalEntityId={legalEntityId} landed={landed} />
        </section>
      )}
      <section className="border-t border-idpxyz-border pt-5">
        <div>
          <h4 className="text-[13px] font-semibold text-idpxyz-text">资料修订历史</h4>
          <p className="mt-1 text-xs text-idpxyz-breadcrumb">每次更新都留一版，旧的不会被改掉。</p>
        </div>
        <RevisionHistorySection register={profileRevisionHistory} subjectId={legalEntityId} revision={landed} />
      </section>
      {mayWrite ? (
        <section ref={formRef} className="border-t border-idpxyz-border pt-5">
          {formOpen ? (
            <ProfileRegistrationForm
              legalEntityId={legalEntityId}
              onLanded={() => {
                setLanded((count) => count + 1);
                onProfileUpdated?.();
              }}
            />
          ) : (
            <Button className="text-idpxyz-breadcrumb" variant="outline" onClick={() => setFormOpen(true)}>
              更新开票资料
            </Button>
          )}
        </section>
      ) : null}
    </div>
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
      <div className="mt-3 max-w-80 rounded-md border border-idpxyz-border bg-idpxyz-sidebar p-3">
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
        暂时读不到法人资料：接入渠道还没有配置。这不代表资料不全；配置好之后重新打开本页即可。
      </p>
    );
  }
  if (isOperatorRefusal(answer)) {
    return <OperatorRefusalNote kind={answer.kind} what="法人资料按时点解析" className={note} />;
  }
  if (answer.kind === 'callerProblem') {
    return (
      <div className={note}>
        <p>调用方式问题：{problemNote(answer.code)}</p>
        <TechnicalDetails rows={httpRows(answer.status, answer.code)} />
      </div>
    );
  }
  if (answer.kind === 'noAnswer') {
    return (
      <SectionError
        title="没有得到答案"
        description={problemNote(answer.code)}
        technical={httpRows(answer.status, answer.code)}
        onRetry={retry}
      />
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
        <span className="ml-2 text-idpxyz-breadcrumb">问的时刻 {formatInstant(body.at)}</span>
      </p>
      {effective !== undefined ? (
        <dl className="mt-2 grid grid-cols-[6rem_1fr] gap-x-3 gap-y-1">
          <dt className="text-idpxyz-breadcrumb">生效修订</dt>
          <dd className="font-mono">
            {profileRevisionLabel(effective.revision)} · 生效自 {formatInstant(effective.effectiveFrom)} · 依据 {effective.basis}
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
      <dt className="text-idpxyz-breadcrumb">{label}</dt>
      <dd className="font-mono">{value}</dd>
    </>
  );
}

/**
 * 登记新修订的逐字段表单。本地只拦编码层的三件（legalEntityProfileLocalProblems）；地址国家与身份对不对得上、税号合不合
 * 目录、修订连不连续都由服务端答，答复原样示出。修订号建议取这个法人已登的资料修订，落册后重取。
 *
 * 分组与措辞同登记责任法人那张表单（票 group-legal-entities-uiux/03）：资料内容 / 登记 / 高级三组，资料修订号有建议值，
 * 收进高级组；这里法人是钉死的，没有「在册对象的下一修订」可判，高级组只在组里有问题时自己展开。
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

  const [advancedChoice, setAdvancedChoice] = useState<boolean | null>(null);
  const advancedOpen = advancedGroupOpen(advancedChoice, problems, profileAdvancedPaths, false);
  const advancedSummary = `资料修订号 ${form.revisionField.value.trim() || '未填'}${
    form.revisionField.edited ? '（手填）' : '（建议值）'
  }`;

  return (
    <Card>
      <CardContent className="flex flex-col gap-5 pt-4">
        <div className="rounded-md border border-idpxyz-border bg-idpxyz-sidebar p-3">
          <p className="text-[13px] font-medium text-idpxyz-text">登记说明</p>
          <ul className="mt-1 list-disc pl-4 text-xs leading-5 text-idpxyz-breadcrumb">
            <li>每次更新都留一版，新的一版从它的生效时间起取代上一版；可以提前登记未来生效的资料。</li>
            <li>注册地址的国家 / 地区须与身份上的注册国家 / 地区一致。</li>
            <li>没填开票抬头的那一版生效后，开票会因「资料不全」被拒。</li>
          </ul>
        </div>

        <FormGroup title="资料内容" description="对外开票、签约与通知用的注册地址、开票抬头、税务登记号与联系人。">
          <Field label="注册地址国家 / 地区 *" path="profiles[0].registeredAddress" problems={problems}>
            <Input
              value={draft.addressCountry}
              readOnly={locked}
              className="font-mono text-[13px]"
              placeholder="如 CN、SG"
              onChange={(event) => patch({ addressCountry: event.target.value })}
            />
          </Field>
          <Field label="开票抬头" path="profiles[0].invoiceTitle" problems={problems}>
            <Input
              value={draft.invoiceTitle}
              readOnly={locked}
              placeholder="开票用的法人名称"
              onChange={(event) => patch({ invoiceTitle: event.target.value })}
            />
            <span className="block text-[12px] text-idpxyz-breadcrumb mt-1">留空即这一笔不带开票资料。</span>
          </Field>
          <div className="md:col-span-2">
            <Field label="注册地址（一行一段） *" path="profiles[0].registeredAddress.lines" problems={problems}>
              <Textarea
                value={draft.addressLines}
                readOnly={locked}
                rows={3}
                onChange={(event) => patch({ addressLines: event.target.value })}
              />
            </Field>
          </div>

          <div className="md:col-span-2">
            <p className={fieldLabel}>税务登记号</p>
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
                  className="text-idpxyz-breadcrumb"
                  variant="ghost"
                  disabled={locked}
                  onClick={() => patch({ taxNumbers: draft.taxNumbers.filter((_, at) => at !== index) })}
                >
                  移除
                </Button>
              </div>
            ))}
            <Problems lines={problems['profiles[0].taxRegistrationNumbers']} />
            <div className="flex items-center gap-3">
              <Button
                className="text-idpxyz-breadcrumb"
                variant="outline"
                disabled={locked}
                onClick={() => patch({ taxNumbers: [...draft.taxNumbers, { typeCode: '', number: '' }] })}
              >
                添加税务登记号
              </Button>
              <span className="text-[12px] text-idpxyz-breadcrumb">类型码取自注册号类型目录的资料层。</span>
            </div>
          </div>

          <div className="md:col-span-2">
            <p className={fieldLabel}>联系人</p>
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
                  className="text-idpxyz-breadcrumb"
                  variant="ghost"
                  disabled={locked}
                  onClick={() => patch({ contacts: draft.contacts.filter((_, at) => at !== index) })}
                >
                  移除
                </Button>
              </div>
            ))}
            <div className="flex items-center gap-3">
              <Button
                className="text-idpxyz-breadcrumb"
                variant="outline"
                disabled={locked}
                onClick={() => patch({ contacts: [...draft.contacts, { name: '', email: '', phone: '' }] })}
              >
                添加联系人
              </Button>
              <span className="text-[12px] text-idpxyz-breadcrumb">联系人可以不登。</span>
            </div>
          </div>
        </FormGroup>

        <FormGroup title="登记" description="这次更新依据哪份单据、从什么时候起生效；生效时间可以在未来。">
          <Field label="来源单据 *" path="profiles[0].basis" problems={problems}>
            <Input
              value={draft.basis}
              readOnly={locked}
              className="font-mono text-[13px]"
              placeholder="营业执照、税务登记或审批单号"
              onChange={(event) => patch({ basis: event.target.value })}
            />
          </Field>
          <WallTimeField
            label="生效时间 *"
            path="profiles[0].effectiveFrom"
            problems={problems}
            value={draft.effectiveFrom}
            locked={locked}
            timeZone={timeZone}
            onChange={(effectiveFrom) => patch({ effectiveFrom })}
          />
        </FormGroup>

        <AdvancedGroup open={advancedOpen} onToggle={() => setAdvancedChoice(!advancedOpen)} summary={advancedSummary}>
          <RevisionField
            label="更新序号"
            path="profiles[0].revision"
            problems={problems}
            {...form.revisionField}
            note="系统按已登记的记录自动算好，一般不用改。"
          />
        </AdvancedGroup>

        <div className="flex items-start gap-3 border-t border-idpxyz-border pt-4">
          <Button
            variant="default"
            className={primaryActionClass}
            onClick={form.send}
            disabled={locked || !form.canSend}
          >
            {locked ? '保存中…' : '保存'}
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
