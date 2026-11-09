import { useId, type ReactNode } from 'react';
import { Button, Input, Textarea } from '@idpxyz/ui-primitives';
import type { ContactPurpose } from './api';
import { policiesHash } from './commercial-policies-address';
import { useRegistrationNumberTypeCandidates } from './registration-number-type-candidates';
import { useRefdataNames } from '../use-refdata-names';
import type { RefdataNames } from '../refdata-names';
import {
  bankAccountCurrencyOptions,
  contactPurposeLabels,
  invoiceTitleLanguageOptions,
  profileTaxTypeOptions,
  type BankAccountDraft,
  type ContactDraft,
  type LegalEntityProfileDraft,
} from './legal-entity-profile';
import { Field, Problems, SearchableSelect } from './PublicationFormFields';
import type { ProfileFieldGroupId } from './legal-entity-profile-sections';
import { FormGroup } from './party-registration-fields';

/** 结算政策册在商业规则页里的位置：开票币种在那里按公司主体、相对方与合同解析，这里只指路。 */
export const settlementPolicyHash = policiesHash('#/commercial-policies', { tab: 'registers', register: 'SETTLEMENT_POLICY' });

/**
 * 必填格外面的锚：点保存时把第一个缺项滚进视野靠它找格（LegalEntityProfileSection 的 scrollToField）。路径与格认领问题
 * 的 JSON 路径同一个串，缺项清单（legalEntityProfileMissingFields）与锚不必各记一份名字。
 */
export function FieldAnchor({ path, children }: { path: string; children: ReactNode }) {
  return <div data-field-path={path}>{children}</div>;
}

export function LegalEntityProfileFields({
  draft,
  patch,
  problems,
  locked,
  onLeave,
  groups,
  registeredAddressOptional = false,
  taxNumberCountry,
}: {
  draft: LegalEntityProfileDraft;
  patch: (change: Partial<LegalEntityProfileDraft>) => void;
  problems: Record<string, string[]>;
  locked: boolean;
  /** 表单里的站内链接要离开本页时交给调用方：有没提交的输入时先确认再走，不让一个链接把表单静默关掉。 */
  onLeave: (href: string) => void;
  /** 只渲染这几组（分区抽屉，票 company-subject-section-edit/01）；不给渲染全部。 */
  groups?: readonly ProfileFieldGroupId[];
  /** 往来单位资料（ADR-0369）各项都可缺：注册地址不标必填，地址组的说明也换一句。法人资料不给。 */
  registeredAddressOptional?: boolean;
  /** 监管登记号的类型候选按哪个国家 / 地区取；不给按注册地址国家。往来单位带注册国家时由调用方交它。 */
  taxNumberCountry?: string;
}) {
  const show = (group: ProfileFieldGroupId) => groups === undefined || groups.includes(group);
  const platformListId = useId();
  // 语言标签用 datalist 而不是选单：常用的四种一点即选，别的 BCP 47 标签照样能手填（ADR-0359 决定四只锁形状）。
  const languageListId = useId();
  // 币种候选来自产品内置的基础数据目录；订阅是为了目录读回时把手填格换成选单。
  const refdataNames = useRefdataNames();
  const setInvoiceTitle = (index: number, change: Partial<LegalEntityProfileDraft['invoiceTitles'][number]>) =>
    patch({ invoiceTitles: draft.invoiceTitles.map((row, at) => (at === index ? { ...row, ...change } : row)) });
  const setContact = (index: number, change: Partial<ContactDraft>) =>
    patch({ contacts: draft.contacts.map((row, at) => (at === index ? { ...row, ...change } : row)) });
  const setBankAccount = (index: number, change: Partial<BankAccountDraft>) =>
    patch({ bankAccounts: draft.bankAccounts.map((row, at) => (at === index ? { ...row, ...change } : row)) });
  const setTaxNumber = (index: number, change: Partial<LegalEntityProfileDraft['taxNumbers'][number]>) =>
    patch({ taxNumbers: draft.taxNumbers.map((item, at) => (at === index ? { ...item, ...change } : item)) });
  return (
    <>
      <datalist id={platformListId}>
        {['微信', '企业微信', 'WhatsApp', 'Telegram', 'LINE', 'QQ', 'LinkedIn', 'Skype'].map((platform) => <option key={platform} value={platform} />)}
      </datalist>
      <datalist id={languageListId}>
        {invoiceTitleLanguageOptions.map((option) => <option key={option.value} value={option.value} label={option.label} />)}
      </datalist>
      {show('representative') ? (
      <FormGroup title="代表人 / 负责人" description="按登记资料填写；与业务联系人分别管理。">
        <Field label="身份类型" path="profiles[0].representative" problems={problems}>
          <select
            aria-label="代表人身份类型"
            value={draft.representativeKind}
            disabled={locked}
            onChange={(event) =>
              patch({ representativeKind: event.target.value as LegalEntityProfileDraft['representativeKind'] })
            }
            className="h-9 w-full rounded-md border border-idpxyz-border bg-idpxyz-card px-3 type-value text-idpxyz-text"
          >
            <option value="">未选</option>
            <option value="LEGAL_REPRESENTATIVE">法定代表人</option>
            <option value="RESPONSIBLE_PERSON">登记负责人</option>
          </select>
        </Field>
        <Field label="姓名" path="profiles[0].representative" problems={problems}>
          <Input
            aria-label="代表人姓名"
            value={draft.representativeName}
            readOnly={locked}
            onChange={(event) => patch({ representativeName: event.target.value })}
          />
        </Field>
      </FormGroup>
      ) : null}
      {show('addresses') ? (
      <FormGroup
        title="地址"
        description={
          registeredAddressOptional
            ? '都可以不填；往来单位登了注册地时，注册地址的国家 / 地区须与它一致。'
            : '注册地址须与身份的注册国家 / 地区一致；办公和通信地址可分别登记。'
        }
      >
        {(
          [
            ['registeredAddress', '注册地址', 'addressCountry', 'addressLines'],
            ['officeAddress', '办公地址', 'officeCountry', 'officeLines'],
            ['correspondenceAddress', '通信地址', 'correspondenceCountry', 'correspondenceLines'],
          ] as const
        ).map(([path, label, country, lines]) => (
          <div key={path} className="md:col-span-2 grid gap-3 sm:grid-cols-[9rem_minmax(0,1fr)]">
            <FieldAnchor path={`profiles[0].${path}`}>
              <Field
                label={`${label}国家 / 地区${path === 'registeredAddress' && !registeredAddressOptional ? ' *' : ''}`}
                path={`profiles[0].${path}`}
                problems={problems}
              >
                <Input
                  aria-label={`${label}国家 / 地区`}
                  placeholder="如 CN、SG"
                  value={draft[country]}
                  readOnly={locked}
                  onChange={(event) => patch({ [country]: event.target.value })}
                />
              </Field>
            </FieldAnchor>
            <FieldAnchor path={`profiles[0].${path}.lines`}>
              <Field
                label={`${label}${path === 'registeredAddress' && !registeredAddressOptional ? ' *' : '（选填）'}`}
                path={`profiles[0].${path}.lines`}
                problems={problems}
              >
                <Textarea
                  aria-label={label}
                  rows={2}
                  value={draft[lines]}
                  readOnly={locked}
                  onChange={(event) => patch({ [lines]: event.target.value })}
                />
              </Field>
            </FieldAnchor>
          </div>
        ))}
      </FormGroup>
      ) : null}
      {show('invoicing') ? (
      <FormGroup title="税务与开票" description="开票抬头留空表示此版本不带开票资料；其他语言的抬头挂在它上面，哪种语言印到单据上由开单方按收件方定。">
        <Field label="开票抬头" path="profiles[0].invoiceTitle" problems={problems}>
          <Input
            aria-label="开票抬头"
            value={draft.invoiceTitle}
            readOnly={locked}
            onChange={(event) => patch({ invoiceTitle: event.target.value })}
          />
        </Field>
        <Field label="主抬头语言" path="profiles[0].invoiceTitleLanguage" problems={problems}>
          <Input
            list={languageListId}
            aria-label="主抬头语言"
            placeholder="如 zh-Hans、en（选填）"
            className="font-mono type-value"
            value={draft.invoiceTitleLanguage}
            readOnly={locked}
            onChange={(event) => patch({ invoiceTitleLanguage: event.target.value })}
          />
        </Field>
        <div className="md:col-span-2 space-y-2">
          <p className="type-body font-medium text-idpxyz-text">其他语言的抬头 <span className="font-normal text-idpxyz-breadcrumb">（选填，一种语言一行）</span></p>
          {draft.invoiceTitles.map((row, index) => (
            <div key={index} className="space-y-1">
              <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,2fr)_auto] items-start gap-2">
                <Input
                  list={languageListId}
                  aria-label={`其他语言的抬头 ${index + 1} 语言`}
                  placeholder="语言，如 en"
                  className="font-mono type-value"
                  value={row.language}
                  readOnly={locked}
                  onChange={(event) => setInvoiceTitle(index, { language: event.target.value })}
                />
                <Input
                  aria-label={`其他语言的抬头 ${index + 1} 抬头`}
                  placeholder="该语言的抬头"
                  value={row.title}
                  readOnly={locked}
                  onChange={(event) => setInvoiceTitle(index, { title: event.target.value })}
                />
                <Button
                  variant="ghost"
                  aria-label={`移除其他语言的抬头 ${index + 1}`}
                  disabled={locked}
                  onClick={() => patch({ invoiceTitles: draft.invoiceTitles.filter((_, at) => at !== index) })}
                >
                  移除
                </Button>
              </div>
              <Problems lines={problems[`profiles[0].invoiceTitles[${index}]`]} />
            </div>
          ))}
          <Button
            variant="ghost"
            size="sm"
            disabled={locked}
            onClick={() => patch({ invoiceTitles: [...draft.invoiceTitles, { language: '', title: '' }] })}
          >
            添加其他语言的抬头
          </Button>
        </div>
        {/* 币种不在本表单：结算政策按公司主体、相对方与合同解析，这里只指路（票 company-subject-ux/04 第 7 项）。 */}
        <p className="md:col-span-2 type-body leading-5 text-idpxyz-breadcrumb">
          开票币种不在这里定：由「
          <a
            href={settlementPolicyHash}
            className="text-idpxyz-accent underline-offset-2 hover:underline"
            onClick={(event) => {
              event.preventDefault();
              onLeave(settlementPolicyHash);
            }}
          >
            商业规则 › 结算政策
          </a>
          」按公司主体、相对方与合同解析。
        </p>
        <div className="md:col-span-2">
          <p className="mb-2 type-body font-medium text-idpxyz-text">监管登记号（税务 / 海关）</p>
          {draft.taxNumbers.map((row, index) => (
            <div key={index} className="mb-3 grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] items-start gap-3">
              <TaxTypeField
                index={index}
                country={taxNumberCountry ?? draft.addressCountry}
                value={row.typeCode}
                locked={locked}
                problems={problems}
                onChange={(typeCode) => setTaxNumber(index, { typeCode })}
              />
              <Field label="号码" path={`profiles[0].taxRegistrationNumbers[${index}].number`} problems={problems}>
                <Input
                  aria-label={`监管登记号 ${index + 1} 号码`}
                  placeholder="登记号原样填"
                  className="font-mono type-value"
                  value={row.number}
                  readOnly={locked}
                  onChange={(event) => setTaxNumber(index, { number: event.target.value })}
                />
              </Field>
              <Button
                variant="ghost"
                className="mt-5"
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
            添加监管登记号
          </Button>
          {/* 税务与海关的号不分两组（ADR-0359 决定一）：判法一样——类型与格式由目录按注册地说，号属税务还是海关也由类型说。 */}
          <p className="mt-2 type-body leading-5 text-idpxyz-breadcrumb">
            税务、海关等监管机关发给公司、可后办可变的登记号都登在这里：税号、海关注册编码、AEO 认证编号、VAT / IOSS / EORI；
            类型按注册地在「基础数据 › 注册号类型」配置后在这里选。
          </p>
        </div>
      </FormGroup>
      ) : null}
      {show('bankAccounts') ? (
      <FormGroup
        title="收款账户"
        description="对账单与发票上印的我方银行账户，可按币种登记多个。币种只说这个账户收哪种钱，不决定结算币种。"
      >
        <div className="md:col-span-2 space-y-4">
          {draft.bankAccounts.map((row, index) => (
            <fieldset key={index} disabled={locked} className="min-w-0 border-t border-idpxyz-border pt-3">
              <legend className="px-1 type-value font-medium text-idpxyz-text">收款账户 {index + 1}</legend>
              <div className="grid gap-3 sm:grid-cols-2">
                {(
                  [
                    ['label', '用途标签', '如 人民币对公、美元收款'],
                    ['holderName', '户名 *', ''],
                    ['bankName', '开户行 *', ''],
                    ['accountNumber', '账号 / IBAN *', '账号原样填'],
                    ['swiftCode', 'SWIFT / BIC', ''],
                    ['bankAddress', '开户行地址', ''],
                  ] as const
                ).map(([key, label, placeholder]) => {
                  const path = `profiles[0].bankAccounts[${index}].${key}`;
                  const mono = key === 'accountNumber' || key === 'swiftCode';
                  return (
                    <FieldAnchor key={key} path={path}>
                      <Field label={label} path={path} problems={problems}>
                        <Input
                          aria-label={`收款账户 ${index + 1} ${label.replace(' *', '')}`}
                          placeholder={placeholder || undefined}
                          className={mono ? 'font-mono type-value' : undefined}
                          value={row[key]}
                          readOnly={locked}
                          onChange={(event) => setBankAccount(index, { [key]: event.target.value })}
                        />
                      </Field>
                    </FieldAnchor>
                  );
                })}
                <BankAccountCurrencyField
                  index={index}
                  value={row.currency}
                  locked={locked}
                  problems={problems}
                  names={refdataNames}
                  onChange={(currency) => setBankAccount(index, { currency })}
                />
                <Field label="备注" path={`profiles[0].bankAccounts[${index}].note`} problems={problems}>
                  <Input
                    aria-label={`收款账户 ${index + 1} 备注`}
                    value={row.note}
                    readOnly={locked}
                    onChange={(event) => setBankAccount(index, { note: event.target.value })}
                  />
                </Field>
              </div>
              <Problems lines={problems[`profiles[0].bankAccounts[${index}]`]} />
              <Button
                className="mt-2"
                variant="ghost"
                disabled={locked}
                onClick={() => patch({ bankAccounts: draft.bankAccounts.filter((_, at) => at !== index) })}
              >
                移除此账户
              </Button>
            </fieldset>
          ))}
          <Button
            variant="outline"
            disabled={locked}
            onClick={() =>
              patch({
                bankAccounts: [
                  ...draft.bankAccounts,
                  { label: '', holderName: '', bankName: '', accountNumber: '', swiftCode: '', bankAddress: '', currency: '', note: '' },
                ],
              })
            }
          >
            添加收款账户
          </Button>
        </div>
      </FormGroup>
      ) : null}
      {show('contacts') ? (
      <FormGroup title="联系人" description="可配置多个用途；每种用途可分别指名一位主要联系人。">
        <div className="md:col-span-2 space-y-4">
          {draft.contacts.map((row, index) => (
            <fieldset key={index} disabled={locked} className="min-w-0 border-t border-idpxyz-border pt-3">
              <legend className="px-1 type-value font-medium text-idpxyz-text">联系人 {index + 1}</legend>
              <div className="grid gap-3 sm:grid-cols-2">
                {(
                  [
                    ['name', '姓名 *'],
                    ['department', '部门'],
                    ['position', '职务'],
                    ['email', '邮箱'],
                    ['phone', '电话'],
                  ] as const
                ).map(([key, label]) => {
                  const path = `profiles[0].contacts[${index}]${key === 'name' ? '' : '.' + key}`;
                  const field = (
                    <Field label={label} path={path} problems={problems}>
                      <Input
                        aria-label={`联系人 ${index + 1} ${label}`}
                        value={row[key] ?? ''}
                        readOnly={locked}
                        onChange={(event) => setContact(index, { [key]: event.target.value })}
                      />
                    </Field>
                  );
                  return key === 'name' ? (
                    <FieldAnchor key={key} path={path}>
                      {field}
                    </FieldAnchor>
                  ) : (
                    <div key={key}>{field}</div>
                  );
                })}
              </div>
              <div className="mt-4 space-y-2">
                <p className="type-body font-medium text-idpxyz-text">社交账号 <span className="font-normal text-idpxyz-breadcrumb">（选填，可添加多个）</span></p>
                {(row.socialAccounts ?? []).map((social, accountIndex) => (
                  <div key={accountIndex} className="space-y-1">
                    <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,2fr)_auto] items-start gap-2">
                      <Input
                        list={platformListId}
                        aria-label={`联系人 ${index + 1} 社交账号 ${accountIndex + 1} 平台`}
                        placeholder="选择或填写平台"
                        value={social.platform}
                        onChange={(event) => setContact(index, { socialAccounts: row.socialAccounts!.map((item, at) => at === accountIndex ? { ...item, platform: event.target.value } : item) })}
                      />
                      <Input
                        aria-label={`联系人 ${index + 1} 社交账号 ${accountIndex + 1} 账号`}
                        placeholder="账号或资料链接"
                        value={social.account}
                        onChange={(event) => setContact(index, { socialAccounts: row.socialAccounts!.map((item, at) => at === accountIndex ? { ...item, account: event.target.value } : item) })}
                      />
                      <Button variant="ghost" aria-label={`移除联系人 ${index + 1} 社交账号 ${accountIndex + 1}`} onClick={() => setContact(index, { socialAccounts: row.socialAccounts!.filter((_, at) => at !== accountIndex) })}>移除</Button>
                    </div>
                    <Problems lines={problems[`profiles[0].contacts[${index}].socialAccounts[${accountIndex}]`]} />
                  </div>
                ))}
                <Button variant="ghost" size="sm" aria-label={`为联系人 ${index + 1} 添加社交账号`} onClick={() => setContact(index, { socialAccounts: [...(row.socialAccounts ?? []), { platform: '', account: '' }] })}>添加社交账号</Button>
              </div>
              <div className="mt-3 flex flex-wrap gap-x-5 gap-y-2">
                {(Object.entries(contactPurposeLabels) as [ContactPurpose, string][]).map(([purpose, label]) => {
                  const selected = row.purposes?.includes(purpose) ?? false;
                  return (
                    <div key={purpose} className="flex flex-wrap items-center gap-2 type-body text-idpxyz-text">
                      <label className="flex items-center gap-1.5">
                        <input
                          type="checkbox"
                          checked={selected}
                          onChange={(event) =>
                            setContact(index, {
                              purposes: event.target.checked
                                ? [...(row.purposes ?? []), purpose]
                                : (row.purposes ?? []).filter((value) => value !== purpose),
                              primaryFor: (row.primaryFor ?? []).filter((value) => value !== purpose),
                            })
                          }
                        />
                        {label}
                      </label>
                      {selected ? (
                        <label className="flex items-center gap-1 text-idpxyz-breadcrumb">
                          <input
                            type="checkbox"
                            aria-label={`联系人 ${index + 1} ${label}主要联系人`}
                            checked={row.primaryFor?.includes(purpose) ?? false}
                            onChange={(event) =>
                              patch({
                                contacts: draft.contacts.map((contact, at) => ({
                                  ...contact,
                                  primaryFor:
                                    at === index && event.target.checked
                                      ? [...(contact.primaryFor ?? []).filter((value) => value !== purpose), purpose]
                                      : (contact.primaryFor ?? []).filter((value) => value !== purpose),
                                })),
                              })
                            }
                          />
                          主要
                        </label>
                      ) : null}
                    </div>
                  );
                })}
              </div>
              <Button
                className="mt-2"
                variant="ghost"
                disabled={locked}
                onClick={() => patch({ contacts: draft.contacts.filter((_, at) => at !== index) })}
              >
                移除此联系人
              </Button>
            </fieldset>
          ))}
          <Button
            variant="outline"
            disabled={locked}
            onClick={() =>
              patch({
                contacts: [
                  ...draft.contacts,
                  { name: '', email: '', phone: '', department: '', position: '', purposes: [], primaryFor: [] },
                ],
              })
            }
          >
            添加联系人
          </Button>
        </div>
      </FormGroup>
      ) : null}
    </>
  );
}

/**
 * 收款账户的币种格：候选是产品内置币种目录（基础数据），显「人民币（CNY）」、值是码。目录读不回时退回手填并提示填
 * ISO 4217——新登记的币种只收目录里的码由服务端判（ADR-0345 决定五），表单不替目录造码，也不在本地判形状。
 */
function BankAccountCurrencyField({
  index,
  value,
  locked,
  problems,
  names,
  onChange,
}: {
  index: number;
  value: string;
  locked: boolean;
  problems: Record<string, string[]>;
  names: RefdataNames;
  onChange: (currency: string) => void;
}) {
  const path = `profiles[0].bankAccounts[${index}].currency`;
  const options = bankAccountCurrencyOptions(names);
  return (
    <FieldAnchor path={path}>
      <Field label="币种 *" path={path} problems={problems}>
        {options.length > 0 ? (
          <SearchableSelect options={options} value={value} disabled={locked} onChange={onChange} unknownNote="手填，不在目录上" />
        ) : (
          <>
            <Input
              aria-label={`收款账户 ${index + 1} 币种`}
              placeholder="如 CNY、USD"
              className="font-mono type-value"
              value={value}
              readOnly={locked}
              onChange={(event) => onChange(event.target.value)}
            />
            <span className="mt-1 block type-body text-idpxyz-breadcrumb">币种目录暂时读不到：先手填 ISO 4217 三位字母码。</span>
          </>
        )}
      </Field>
    </FieldAnchor>
  );
}

/**
 * 监管登记号的类型格（票 company-subject-ux/04 第 6 项）：候选是目录里资料层、国家 = 注册地址国家的类型，由读口按这两格收窄
 * （registration-number-type-candidates.ts），显目录译名，码进载荷。目录里这一国家没有资料层类型、或目录读不到时退回手填类型码
 * 并指路——要加类型得先到基础数据配置，表单不替目录造码。选出来的只是类型码，合不合目录仍由服务端登记时判。
 */
function TaxTypeField({
  index,
  country,
  value,
  locked,
  problems,
  onChange,
}: {
  index: number;
  country: string;
  value: string;
  locked: boolean;
  problems: Record<string, string[]>;
  onChange: (typeCode: string) => void;
}) {
  const path = `profiles[0].taxRegistrationNumbers[${index}].typeCode`;
  const candidates = useRegistrationNumberTypeCandidates('PROFILE', country);
  const options = candidates?.kind === 'outcome' ? profileTaxTypeOptions(candidates.body.registrationNumberTypes, country) : [];
  if (options.length > 0) {
    return (
      <Field label="类型" path={path} problems={problems}>
        <SearchableSelect options={options} value={value} disabled={locked} onChange={onChange} unknownNote="手填，不在目录上" />
      </Field>
    );
  }
  const reason =
    country.trim() === ''
      ? '先填注册地址国家 / 地区，这里才有候选。'
      : candidates === null
        ? '正在读注册号类型目录…'
        : candidates.kind === 'outcome'
          ? `目录里「${country.trim()}」还没有资料层的注册号类型，可先手填类型码。`
          : '注册号类型目录暂时读不到，可先手填类型码。';
  return (
    <Field label="类型" path={path} problems={problems}>
      <Input
        aria-label={`监管登记号 ${index + 1} 类型`}
        placeholder="类型码"
        className="font-mono type-value"
        value={value}
        readOnly={locked}
        onChange={(event) => onChange(event.target.value)}
      />
      <span className="mt-1 block type-body text-idpxyz-breadcrumb">
        {reason}要加类型，先到「基础数据 › 注册号类型」配置。
      </span>
    </Field>
  );
}
