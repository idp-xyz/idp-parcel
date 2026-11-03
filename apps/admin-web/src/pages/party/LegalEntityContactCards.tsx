import { type ReactNode } from 'react';
import { Mail, MessageCircle, Phone, type LucideIcon } from 'lucide-react';
import type { LegalEntityContactRecord } from './api';
import { contactCardOf, socialAccountsText, type ContactCard } from './legal-entity-contact-cards';
import './legal-entity-profile-readout.css';

/**
 * 联系人卡（票 company-subject-ux/11 第 1 条）：一人一张，随容器宽度排一到三列。此前是五列表，可逐列比对的只有姓名与
 * 用途，其余三列都是一格叠几行、一人占一百多像素高，四个人就占满一屏；卡把一个人的信息收在一块，一屏看得完整个名册。
 * 卡与资料读出卡（ProfileReadoutCard）同一种画法——细边框、无投影——但不带浅底标题条：人名就是标题。
 * 哪几行显、用途怎么折算由 legal-entity-contact-cards.ts 判，这里只摆。
 */
export function LegalEntityContactCards({ contacts }: { contacts: readonly LegalEntityContactRecord[] }) {
  // 整张清单没有人是结论句，照旧用文字（spec 公司主体 UX「统一用语」）。
  if (!contacts.length) return <p className="text-[14px] leading-6 text-idpxyz-breadcrumb">还没有登记联系人</p>;
  return (
    <div className="legal-entity-contact-cards">
      <ul className="legal-entity-contact-cards-grid" aria-label="联系人清单">
        {contacts.map((contact, index) => (
          <ContactCardView key={index} card={contactCardOf(contact)} />
        ))}
      </ul>
    </div>
  );
}

function ContactCardView({ card }: { card: ContactCard }) {
  const hasLines = card.phone || card.email || card.socialAccounts.length > 0;
  // 字号与资料读出同一把尺（票 company-subject-ux/13 第 3 条）：人名与各行 14、所属与平台名 13、等宽账号 13。
  return (
    <li className="min-w-0 rounded-lg border border-idpxyz-border bg-idpxyz-bg px-4 py-3">
      <h5 className="break-words text-[14px] font-semibold leading-6 text-idpxyz-textBright">{card.name}</h5>
      {card.affiliation ? <p className="mt-0.5 break-words text-[13px] leading-5 text-idpxyz-breadcrumb">{card.affiliation}</p> : null}
      {card.purposes.length ? (
        <ul className="mt-2 flex flex-wrap gap-1.5" aria-label="联系用途">
          {card.purposes.map((purpose) => (
            // 「主要」此前用强调色，看着像链接却不可点；这里只换字号不换色，胶囊底取工作区那层冷灰（bg-idpxyz-editor），
            // 不取 hover 色——浅色主题里 hover 色就是选中蓝（理由同 ProfileReadoutCard 文件头）。
            <li
              key={purpose.code}
              title={purpose.primary ? `${purpose.label}的主要联系人` : undefined}
              className="inline-flex items-center gap-1 whitespace-nowrap rounded-full border border-idpxyz-border bg-idpxyz-editor px-2 py-0.5 text-[12px] leading-4 text-idpxyz-text"
            >
              {purpose.label}
              {purpose.primary ? (
                <>
                  <span aria-hidden="true" className="text-idpxyz-breadcrumb">·</span>
                  <span className="text-[11px] font-medium">主要</span>
                </>
              ) : null}
            </li>
          ))}
        </ul>
      ) : null}
      {hasLines ? (
        <ul className="mt-2.5 space-y-1 text-[14px] leading-6 text-idpxyz-text">
          {card.phone ? (
            <ContactLine icon={Phone} label="电话">
              <span className="tabular-nums">{card.phone}</span>
            </ContactLine>
          ) : null}
          {card.email ? (
            <ContactLine icon={Mail} label="邮箱">
              {/* 邮箱按字符折、不按连字符折：按词折会在域名的「idp-」后断开，读的人要把两段拼回去才认得出是哪个域。 */}
              <span className="break-all">{card.email}</span>
            </ContactLine>
          ) : null}
          {card.socialAccounts.length ? (
            <ContactLine icon={MessageCircle} label="社交账号" title={socialAccountsText(card.socialAccounts)}>
              {/* 一个账号一行、平台名占固定一栏：几个账号串在一行时平台名与账号混在一起，折行处还会剩一个分隔点挂在行尾
                  （用户 2026-11-03 指出）。平台名空的那一栏留空，账号仍对齐。 */}
              <span className="grid gap-0.5">
                {card.socialAccounts.map((account, index) => (
                  <span key={`${account.platform}-${account.account}-${index}`} className="flex min-w-0 items-baseline gap-2">
                    <span className="w-[4.5em] shrink-0 text-[13px] text-idpxyz-breadcrumb">{account.platform}</span>
                    <span className="min-w-0 select-text break-all font-mono text-[13px]">{account.account}</span>
                  </span>
                ))}
              </span>
            </ContactLine>
          ) : null}
        </ul>
      ) : null}
    </li>
  );
}

/** 卡内一行：弱色小图标在前、内容在后。图标只是视觉记号，行名以 sr-only 给读屏器，否则三行只剩内容、分不清哪行是什么。 */
function ContactLine({ icon: Icon, label, title, children }: { icon: LucideIcon; label: string; title?: string; children: ReactNode }) {
  return (
    <li className="flex min-w-0 items-start gap-2" title={title}>
      <Icon className="mt-[5px] h-3.5 w-3.5 shrink-0 text-idpxyz-breadcrumb" aria-hidden="true" />
      <span className="sr-only">{label}</span>
      <span className="min-w-0">{children}</span>
    </li>
  );
}
