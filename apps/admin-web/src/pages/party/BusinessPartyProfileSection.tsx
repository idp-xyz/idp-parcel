import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { FileText, Landmark, MapPin, Receipt, UserPlus } from 'lucide-react';
import { Button, Input } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { statusToneBanner, statusToneText } from '../../components/status-tone';
import { RegistrationAnswerNote } from '../../components/registration';
import { moduleInfoById } from '../../navigation';
import { useAnswer, type AnswerQuery } from '../../query/answers';
import { formatInstant } from '../catalogue-view';
import {
  businessPartyProfileOutcomeLabels,
  listBusinessPartyProfileRevisions,
  listRegistrationNumberTypes,
  type BusinessPartyProfileRevisionListResponseBody,
  type BusinessPartyProfileRevisionRecord,
} from './api';
import {
  businessPartyProfileChanged,
  businessPartyProfileDraftBasedOn,
  businessPartyProfileLocalProblems,
  businessPartyProfileMissingFields,
  businessPartyProfilePayloadOf,
  customerLacksContacts,
  effectiveBusinessPartyProfile,
  suggestedBusinessPartyProfileRevision,
  type BusinessPartyProfileDraft,
} from './business-party-profile';
import { LegalEntityBankAccountsTable } from './LegalEntityProfileContent';
import { LegalEntityContactCards } from './LegalEntityContactCards';
import { FieldAnchor, LegalEntityProfileFields } from './LegalEntityProfileFields';
import {
  ProfileReadoutAddresses,
  ProfileReadoutCard,
  ProfileReadoutField,
  ProfileReadoutGrid,
  ProfileReadoutPairs,
  ReadoutText,
  absentValueMark,
} from './LegalEntityProfileReadout';
import { invoiceTitleRowsOf, languageTagLabel, problemsWithMissing } from './legal-entity-profile';
import { scrollToField, useTypeNameOf } from './legal-entity-profile-form-parts';
import { CopyAside, RefText } from './detail-primitives';
import { AdvancedGroup, FormGroup, RevisionField, WallTimeField, useRegistrationForm } from './party-registration-fields';
import { partyAnswersDomain } from './party-writes';
import { problemNote } from './presentation';
import { Field, Problems, fieldLabel, useLoaded } from './PublicationFormFields';
import { advancedGroupOpen } from './registration-form';

// 往来单位资料（ADR-0369）在对象页上的几块：读出、联系人、版本清单与「更新资料」编辑态。资料的形状与法人资料相同，读出的小组件、
// 字段组件与收款账户表、联系人卡都借法人资料那一套；这里只摆往来单位自己的卡：其他语言名称并进地址卡，没有法定代表人。

const info = moduleInfoById['business-parties'];
const kind = 'business-party-profile';
const fieldGroups = ['addresses', 'invoicing', 'bankAccounts', 'contacts'] as const;
const advancedPaths = ['profiles[0].revision'];

/**
 * 一家往来单位的资料修订链。读写两处共用一个键：写口答到后参与方域整域作废（party-writes.ts），读出与编辑态下次都现问。
 */
export function useBusinessPartyProfiles(
  partyId: string,
  fresh = false,
  enabled = true,
): AnswerQuery<BusinessPartyProfileRevisionListResponseBody> {
  return useAnswer([partyAnswersDomain, 'business-party', partyId, 'profile-revisions'], () => listBusinessPartyProfileRevisions(partyId), {
    fresh,
    enabled,
  });
}

/** 资料修订链答复里此刻有效的那一笔；还在读、读不到或没有一笔生效都是 null。第二个值说的是有没有读到答复。 */
export function effectiveProfileOf(
  query: AnswerQuery<BusinessPartyProfileRevisionListResponseBody>,
  partyId: string,
): { record: BusinessPartyProfileRevisionRecord | null; body: BusinessPartyProfileRevisionListResponseBody | null } {
  const answer = query.answer;
  if (answer?.kind !== 'outcome' || answer.body.partyId !== partyId) return { record: null, body: null };
  return { record: effectiveBusinessPartyProfile(answer.body), body: answer.body };
}

/** 我方公司的往来单位在资料位置上的那句话：它的资料登在公司主体上（ADR-0369 决定二）。对象页与详情栏共用。 */
export const ourCompanyProfileNote = '这是我方公司的往来单位：联系人、地址、税号与收款账户登在公司主体的资料上。';

/**
 * 读出几处共用的资料状态：还在读或读不到只给一句灰字；读到了却没有生效的一版是空状态（登过、都在未来生效的另说一句）；
 * 有生效版就交出那一版。别家的答复（换行时上一家的还没换掉）当作还在读。
 */
type ProfileState =
  | { kind: 'note'; text: string }
  | { kind: 'empty'; scheduled: boolean; body: BusinessPartyProfileRevisionListResponseBody }
  | { kind: 'present'; record: BusinessPartyProfileRevisionRecord; body: BusinessPartyProfileRevisionListResponseBody };

function profileStateOf(query: AnswerQuery<BusinessPartyProfileRevisionListResponseBody>, partyId: string): ProfileState {
  const answer = query.answer;
  if (answer === null) return { kind: 'note', text: '正在读取资料…' };
  if (answer.kind !== 'outcome') return { kind: 'note', text: '资料暂时读不到，稍后刷新再看。' };
  const { record, body } = effectiveProfileOf(query, partyId);
  if (body === null) return { kind: 'note', text: '正在读取资料…' };
  if (record) return { kind: 'present', record, body };
  return { kind: 'empty', scheduled: body.revisions.length > 0, body };
}

const customerEmptyLine = '这家单位是客户，建议登记一位联系人，下单和对账时找得到人。';
const customerHintLine = '这家单位是客户，资料里还没有联系人；建议登记一位，下单和对账时找得到人。';

/**
 * 资料空着时的那一块：说清空在哪、下一步去哪。有写授予的给「更新资料」——描边按钮：页头已有一枚实心的主动作，两枚实心会抢；
 * 查看者只看到话。
 */
function ProfileEmptyState({ title, lines, onUpdate }: { title: string; lines: readonly string[]; onUpdate?: () => void }) {
  return (
    <div className="mt-3 flex items-start gap-3 rounded-md border border-dashed border-idpxyz-border px-4 py-4">
      <FileText className="mt-0.5 h-5 w-5 shrink-0 text-idpxyz-breadcrumb" aria-hidden="true" />
      <div className="min-w-0 flex-1">
        <p className="type-value font-medium text-idpxyz-text">{title}</p>
        {lines.map((line) => (
          <p key={line} className="mt-1 type-body leading-5 text-idpxyz-breadcrumb">
            {line}
          </p>
        ))}
        {onUpdate ? (
          <Button size="sm" variant="outline" className="mt-3 type-body" onClick={onUpdate}>
            更新资料
          </Button>
        ) : null}
      </div>
    </div>
  );
}

/** 「概要」里的资料读出：名称与地址、税务与开票、收款账户三张卡。联系人有自己的分区。 */
export function BusinessPartyProfileReadout({
  partyId,
  query,
  registrationCountry,
  roles = [],
  onUpdate,
}: {
  partyId: string;
  query: AnswerQuery<BusinessPartyProfileRevisionListResponseBody>;
  /** 往来单位登记的注册国家：监管登记号的类型名按它查，没有按注册地址国家。 */
  registrationCountry?: string;
  /** 这家单位此刻持有的角色；是客户而缺联系人时提示一句（customerLacksContacts）。 */
  roles?: readonly string[];
  /** 空状态与提示里的「更新资料」；不给就不摆按钮（查看者）。 */
  onUpdate?: () => void;
}) {
  const typeCatalogue = useLoaded(listRegistrationNumberTypes);
  const typeNameOf = useTypeNameOf(typeCatalogue);
  const state = profileStateOf(query, partyId);
  if (state.kind === 'note') return <p className="type-value leading-6 text-idpxyz-breadcrumb">{state.text}</p>;
  if (state.kind === 'empty') {
    return (
      <ProfileEmptyState
        title={state.scheduled ? '还没有生效的资料' : '还没有登记资料'}
        lines={[
          state.scheduled ? '已登记的版本都在未来生效。' : '联系人、地址、税号与收款账户都在「更新资料」里登记。',
          ...(customerLacksContacts(roles, null) ? [customerEmptyLine] : []),
        ]}
        onUpdate={onUpdate}
      />
    );
  }
  const record = state.record;
  const names = record.names ?? [];
  const addresses = [
    ...(record.registeredAddress ? [{ key: 'registered', label: '注册地址', address: record.registeredAddress }] : []),
    ...(record.officeAddress ? [{ key: 'office', label: '办公地址', address: record.officeAddress }] : []),
    ...(record.correspondenceAddress ? [{ key: 'correspondence', label: '通信地址', address: record.correspondenceAddress }] : []),
  ];
  const invoiceTitleRows = invoiceTitleRowsOf(record);
  const taxCountry = registrationCountry ?? record.registeredAddress?.country ?? '';
  const bankAccounts = record.bankAccounts ?? [];
  return (
    <div className="legal-entity-profile-readout mt-3 flex flex-col gap-3">
      {customerLacksContacts(roles, record) ? (
        <div className={`flex items-center gap-2 rounded border px-3 py-2 type-body ${statusToneBanner.info}`}>
          <UserPlus className={`h-3.5 w-3.5 shrink-0 ${statusToneText.info}`} aria-hidden="true" />
          <span className="min-w-0 flex-1 leading-5">{customerHintLine}</span>
          {onUpdate ? (
            <Button size="sm" variant="outline" className="h-7 shrink-0 type-body" onClick={onUpdate}>
              更新资料
            </Button>
          ) : null}
        </div>
      ) : null}
      <ProfileReadoutCard icon={MapPin} title="名称与地址">
        <ProfileReadoutGrid>
          <ProfileReadoutField
            label="其他语言名称"
            wide
            value={
              names.length ? (
                <ProfileReadoutPairs
                  headNowrap
                  rows={names.map((row) => ({
                    key: row.language,
                    head: languageTagLabel(row.language),
                    value: (
                      <CopyAside value={row.name} name="名称">
                        <ReadoutText text={row.name} />
                      </CopyAside>
                    ),
                  }))}
                />
              ) : undefined
            }
          />
        </ProfileReadoutGrid>
        <div className="mt-3">
          {addresses.length ? (
            <ProfileReadoutAddresses items={addresses} />
          ) : (
            <ProfileReadoutGrid>
              <ProfileReadoutField label="地址" value={undefined} wide />
            </ProfileReadoutGrid>
          )}
        </div>
      </ProfileReadoutCard>
      <ProfileReadoutCard icon={Receipt} title="税务与开票">
        <ProfileReadoutGrid>
          <ProfileReadoutField
            label="开票抬头"
            wide
            value={
              invoiceTitleRows.length ? (
                <ProfileReadoutPairs
                  headNowrap
                  rows={invoiceTitleRows.map((row, index) => ({
                    key: `${row.language}-${index}`,
                    head: row.language ? languageTagLabel(row.language) : absentValueMark,
                    value: (
                      <CopyAside value={row.title} name="开票抬头">
                        <ReadoutText text={row.title} />
                      </CopyAside>
                    ),
                  }))}
                />
              ) : undefined
            }
          />
          <ProfileReadoutField
            label="监管登记号"
            title="税务 / 海关的监管登记号"
            wide
            value={
              record.taxRegistrationNumbers.length ? (
                <ProfileReadoutPairs
                  mono
                  rows={record.taxRegistrationNumbers.map((row, index) => {
                    const typeName = typeNameOf(taxCountry, row.typeCode, 'PROFILE') ?? row.typeCode;
                    return {
                      key: `${row.typeCode}-${row.number}-${index}`,
                      head: typeName,
                      headTitle: row.typeCode,
                      value: <RefText value={row.number} name={typeName} />,
                    };
                  })}
                />
              ) : undefined
            }
          />
        </ProfileReadoutGrid>
      </ProfileReadoutCard>
      <ProfileReadoutCard icon={Landmark} title="收款账户" extra={bankAccounts.length ? `${bankAccounts.length} 个` : undefined}>
        <LegalEntityBankAccountsTable accounts={bankAccounts} />
      </ProfileReadoutCard>
    </div>
  );
}

/** 「联系人」分区：此刻有效那一版的联系人卡；没有资料或资料里没有联系人时是空状态。 */
export function BusinessPartyContactsReadout({
  partyId,
  query,
  onUpdate,
}: {
  partyId: string;
  query: AnswerQuery<BusinessPartyProfileRevisionListResponseBody>;
  onUpdate?: () => void;
}) {
  const state = profileStateOf(query, partyId);
  if (state.kind === 'note') return <p className="type-value leading-6 text-idpxyz-breadcrumb">{state.text}</p>;
  const contacts = state.kind === 'present' ? state.record.contacts : [];
  if (contacts.length === 0) {
    return (
      <ProfileEmptyState
        title={state.kind === 'present' ? '还没有登记联系人' : state.scheduled ? '还没有生效的资料' : '还没有登记资料'}
        lines={['联系人与地址、税号一起在「更新资料」里登记。']}
        onUpdate={onUpdate}
      />
    );
  }
  return <LegalEntityContactCards contacts={contacts} />;
}

/** 「资料版本」分区：全部已登记的版本，从新到旧；此刻有效的那一版标「当前」，未到生效时间的标「未生效」。 */
export function BusinessPartyProfileVersions({
  partyId,
  query,
  onUpdate,
}: {
  partyId: string;
  query: AnswerQuery<BusinessPartyProfileRevisionListResponseBody>;
  onUpdate?: () => void;
}) {
  const state = profileStateOf(query, partyId);
  if (state.kind === 'note') return <p className="type-value leading-6 text-idpxyz-breadcrumb">{state.text}</p>;
  const body = state.body;
  if (body.revisions.length === 0) {
    return <ProfileEmptyState title="还没有登记资料" lines={['登记后每一版都列在这里，旧版本不会被覆盖。']} onUpdate={onUpdate} />;
  }
  const now = Date.now();
  const rows = [...body.revisions].sort((left, right) => right.revision - left.revision);
  return (
    <ol className="flex flex-col divide-y divide-idpxyz-border rounded-md border border-idpxyz-border">
      {rows.map((row) => {
        const current = row.revision === body.effectiveRevision;
        const future = Date.parse(row.effectiveFrom) > now;
        return (
          <li key={row.revision} className="flex flex-wrap items-baseline gap-x-4 gap-y-1 px-3 py-2 type-value">
            <span className="font-medium text-idpxyz-textBright">第 {row.revision} 版</span>
            {current ? <span className="rounded bg-idpxyz-hover px-1.5 py-0.5 type-body text-idpxyz-text">当前</span> : null}
            {future ? <span className="rounded bg-idpxyz-hover px-1.5 py-0.5 type-body text-idpxyz-breadcrumb">未生效</span> : null}
            <span className="text-idpxyz-breadcrumb">{formatInstant(row.effectiveFrom, { precision: 'day' })} 起生效</span>
            <span className="font-mono type-body text-idpxyz-breadcrumb">来源单据 {row.basis}</span>
            <span className="ml-auto type-body text-idpxyz-breadcrumb">登记于 {formatInstant(row.registeredAt, { precision: 'minute' })}</span>
          </li>
        );
      })}
    </ol>
  );
}

/**
 * 「更新资料」编辑态：先把修订链读回来（打开时现问，不拿缓存那份预填——缓存那份可能早于别人刚落的一版），基于此刻有效的一版
 * 预填，再挂表单。资料各项都可缺；登记一版就是整份内容，留空的项这一版就不带。「保存 / 取消」与答复经 `footerSlot` 钉进页脚。
 */
export function BusinessPartyProfileEditMode({
  partyId,
  registrationCountry,
  footerSlot,
  onLanded,
  onUnsavedChange,
  onCancel,
  onRequestLeave,
}: {
  partyId: string;
  registrationCountry?: string;
  footerSlot: HTMLElement | null;
  onLanded: (sent: BusinessPartyProfileDraft | null) => void;
  onUnsavedChange: (unsaved: boolean) => void;
  onCancel: () => void;
  /** 表单里的站内链接要离开时交给对象页：有没提交的输入时先问。 */
  onRequestLeave: (href: string) => void;
}) {
  const query = useBusinessPartyProfiles(partyId, true);
  const renderFooter = (node: ReactNode) => (footerSlot ? createPortal(node, footerSlot) : node);
  const { record, body } = effectiveProfileOf(query, partyId);
  if (body === null) {
    return (
      <>
        <p className="type-body text-idpxyz-breadcrumb">{query.answer === null ? '正在读取资料，表单会基于此刻有效的一版预填…' : '资料暂时读不到，稍后再试。'}</p>
        {renderFooter(
          <div className="flex justify-end">
            <Button variant="outline" onClick={onCancel}>
              取消
            </Button>
          </div>,
        )}
      </>
    );
  }
  // 没有生效版但已登过（都在未来生效）时，基于最新登记的那一版预填并说清：新的一版版本号更大，生效后会取代它。
  const latest = record === null ? [...body.revisions].sort((left, right) => right.revision - left.revision)[0] ?? null : null;
  return (
    <ProfileForm
      partyId={partyId}
      base={record ?? latest}
      scheduled={latest}
      known={body.revisions}
      registrationCountry={registrationCountry}
      renderFooter={renderFooter}
      onLanded={onLanded}
      onUnsavedChange={onUnsavedChange}
      onCancel={onCancel}
      onRequestLeave={onRequestLeave}
    />
  );
}

function ProfileForm({
  partyId,
  base,
  scheduled,
  known,
  registrationCountry,
  renderFooter,
  onLanded,
  onUnsavedChange,
  onCancel,
  onRequestLeave,
}: {
  partyId: string;
  base: BusinessPartyProfileRevisionRecord | null;
  /** 没有生效版、预填用的是这一笔尚未生效的版本时给它；有生效版为 null。 */
  scheduled: BusinessPartyProfileRevisionRecord | null;
  known: BusinessPartyProfileRevisionRecord[];
  registrationCountry?: string;
  renderFooter: (node: ReactNode) => ReactNode;
  onLanded: (sent: BusinessPartyProfileDraft | null) => void;
  onUnsavedChange: (unsaved: boolean) => void;
  onCancel: () => void;
  onRequestLeave: (href: string) => void;
}) {
  const baseDraft = useMemo(() => businessPartyProfileDraftBasedOn(partyId, base), [partyId, base]);
  const form = useRegistrationForm<BusinessPartyProfileDraft>({
    kind,
    empty: () => baseDraft,
    suggestion: () => suggestedBusinessPartyProfileRevision(known),
    localProblems: businessPartyProfileLocalProblems,
    payloadOf: businessPartyProfilePayloadOf,
    landedOutcome: 'REGISTERED',
    onLanded,
  });
  const { draft, patch, locked, timeZone } = form;
  useEffect(() => {
    onUnsavedChange(form.unsaved);
  }, [onUnsavedChange, form.unsaved]);
  useEffect(() => () => onUnsavedChange(false), [onUnsavedChange]);
  // 必填未填只在点过保存之后才显在格下与按钮旁，判据同法人资料表单。
  const [attempted, setAttempted] = useState(false);
  const missing = businessPartyProfileMissingFields(form.effectiveDraft);
  const problems = attempted ? problemsWithMissing(form.problems, missing) : form.problems;
  const bodyRef = useRef<HTMLDivElement>(null);
  // 内容没变就不登新版：只改来源单据或生效时间不算改动。第一版照旧可存。
  const changed = base === null || businessPartyProfileChanged(baseDraft, form.effectiveDraft, timeZone);
  const save = () => {
    if (missing.length > 0) {
      setAttempted(true);
      scrollToField(bodyRef.current, missing[0].path);
      return;
    }
    form.send();
  };
  const [advancedChoice, setAdvancedChoice] = useState<boolean | null>(null);
  const advancedOpen = advancedGroupOpen(advancedChoice, problems, advancedPaths, false);
  const setName = (index: number, change: Partial<BusinessPartyProfileDraft['names'][number]>) =>
    patch({ names: draft.names.map((row, at) => (at === index ? { ...row, ...change } : row)) });

  return (
    <>
      <div ref={bodyRef} className="flex min-w-0 scroll-mt-14 flex-col gap-5">
        <div className="rounded-md border border-idpxyz-border bg-idpxyz-sidebar px-3 py-2 type-body leading-5 text-idpxyz-text">
          {scheduled
            ? `还没有生效的资料：第 ${scheduled.revision} 版定在 ${formatInstant(scheduled.effectiveFrom, { precision: 'day' })} 起生效，表单基于它预填。保存后登记为版本号更大的新一版，从它的生效时间起取代第 ${scheduled.revision} 版；要改第 ${scheduled.revision} 版，就在这里改好再保存。`
            : base
              ? `基于此刻有效的第 ${base.revision} 版预填；保存后登记为新的一版，从它的生效时间起取代上一版，历次版本都保留。`
              : '还没有登记资料，从空白开始；保存后登记为第一版。'}
        </div>
        <FormGroup title="其他语言名称" description="对外单据与海外往来要用的写法，如英文名；身份上的名称不变。都可以不填。">
          <div className="md:col-span-2 flex flex-col gap-2">
            {draft.names.map((row, index) => (
              <div key={index} className="grid grid-cols-[10rem_minmax(0,1fr)_auto] items-start gap-2">
                <FieldAnchor path={`profiles[0].names[${index}]`}>
                  <Input
                    aria-label={`其他语言名称 ${index + 1} 语言`}
                    placeholder="语言，如 en"
                    className="font-mono type-value"
                    value={row.language}
                    readOnly={locked}
                    onChange={(event) => setName(index, { language: event.target.value })}
                  />
                </FieldAnchor>
                <Input
                  aria-label={`其他语言名称 ${index + 1} 名称`}
                  placeholder="该语言的名称"
                  className="type-value"
                  value={row.name}
                  readOnly={locked}
                  onChange={(event) => setName(index, { name: event.target.value })}
                />
                <Button
                  variant="ghost"
                  className="text-idpxyz-breadcrumb"
                  disabled={locked}
                  onClick={() => patch({ names: draft.names.filter((_, at) => at !== index) })}
                >
                  移除
                </Button>
                <div className="col-span-3">
                  <Problems lines={problems[`profiles[0].names[${index}]`]} />
                </div>
              </div>
            ))}
            <div>
              <Button
                variant="outline"
                className="shrink-0 whitespace-nowrap text-idpxyz-breadcrumb"
                disabled={locked}
                onClick={() => patch({ names: [...draft.names, { language: '', name: '' }] })}
              >
                添加一个名称
              </Button>
            </div>
          </div>
        </FormGroup>

        <LegalEntityProfileFields
          draft={draft}
          patch={patch}
          problems={problems}
          locked={locked}
          onLeave={onRequestLeave}
          groups={fieldGroups}
          registeredAddressOptional
          taxNumberCountry={registrationCountry}
        />

        <FormGroup title="这次更新" description="依据哪份单据、从什么时候起生效；生效时间可以在未来。">
          <FieldAnchor path="profiles[0].basis">
            <Field label="来源单据 *" path="profiles[0].basis" problems={problems}>
              <Input
                value={draft.basis}
                readOnly={locked}
                className="font-mono type-value"
                aria-label="来源单据"
                placeholder="合同、营业执照或对方来函编号"
                onChange={(event) => patch({ basis: event.target.value })}
              />
            </Field>
          </FieldAnchor>
          <FieldAnchor path="profiles[0].effectiveFrom">
            <WallTimeField
              label="生效时间 *"
              path="profiles[0].effectiveFrom"
              problems={problems}
              value={draft.effectiveFrom}
              locked={locked}
              timeZone={timeZone}
              onChange={(effectiveFrom) => patch({ effectiveFrom })}
            />
          </FieldAnchor>
        </FormGroup>

        <AdvancedGroup
          open={advancedOpen}
          onToggle={() => setAdvancedChoice(!advancedOpen)}
          summary={`版本号 ${form.revisionField.value.trim() || '未填'}${form.revisionField.edited ? '（手填）' : '（建议值）'}`}
        >
          <RevisionField
            label="版本号"
            path="profiles[0].revision"
            problems={problems}
            {...form.revisionField}
            note="系统按已登记的版本自动算好，一般不用改。"
          />
        </AdvancedGroup>
        <p className={fieldLabel}>对方的税号与收款账户在这里登记、供对账与付款指令使用；开票与付款仍由财务系统执行。</p>
      </div>
      {renderFooter(
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="min-w-0 flex-1 type-body">
            {attempted && missing.length > 0 ? (
              <p role="alert" className="text-idpxyz-danger">
                还有 {missing.length} 项必填未填：{missing.map((field) => field.label).join('、')}
              </p>
            ) : null}
            <RegistrationAnswerNote state={form.state} owner={info.owner} outcomeLabels={businessPartyProfileOutcomeLabels} problemNote={problemNote} />
            {!changed && form.state.kind === 'idle' ? <p className="text-idpxyz-breadcrumb">资料没有改动，不用保存。</p> : null}
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button variant="outline" disabled={locked} onClick={onCancel}>
              取消
            </Button>
            <Button variant="default" className={primaryActionClass} onClick={save} disabled={locked || !form.canSend || !changed}>
              {locked ? '保存中…' : '保存'}
            </Button>
          </div>
        </div>,
      )}
    </>
  );
}
