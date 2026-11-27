import { useEffect, useState } from 'react';
import { CircleHelp, X } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { serviceProductHelpLead, serviceProductHelpSections, serviceProductHelpTitle } from './service-product-help';

// 帮助不写进表单。点标题旁的问号才拉开，仍停在这一页，不换地址。
export function ServiceProductHelp() {
  const [open, setOpen] = useState(false);
  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open]);

  return (
    <>
      <Button
        variant="ghost"
        size="icon"
        className="text-idpxyz-breadcrumb"
        aria-label="服务产品怎么登记"
        title="服务产品怎么登记"
        onClick={() => setOpen(true)}
      >
        <CircleHelp className="h-4 w-4" aria-hidden="true" />
      </Button>
      {open ? (
        <div className="fixed inset-0 z-[120]" role="presentation">
          <button type="button" className="absolute inset-0 bg-black/45" aria-label="关闭说明" onClick={() => setOpen(false)} />
          <aside
            role="dialog"
            aria-labelledby="service-product-help-title"
            className="absolute inset-y-0 right-0 flex w-full max-w-xl flex-col border-l border-idpxyz-border bg-idpxyz-bg shadow-xl"
          >
            <div className="flex items-center justify-between border-b border-idpxyz-border px-5 py-4">
              <h2 id="service-product-help-title" className="type-title font-semibold text-idpxyz-textBright">
                {serviceProductHelpTitle}
              </h2>
              <Button variant="ghost" size="icon" className="text-idpxyz-breadcrumb" aria-label="关闭" onClick={() => setOpen(false)}>
                <X className="h-4 w-4" aria-hidden="true" />
              </Button>
            </div>
            <div className="flex flex-1 flex-col gap-6 overflow-auto px-5 py-5">
              <p className="type-body leading-6 text-idpxyz-text">{serviceProductHelpLead}</p>
              {serviceProductHelpSections.map((section, index) => (
                <section key={section.title}>
                  <h3 className="type-value font-medium text-idpxyz-text">
                    {index + 1}. {section.title}
                  </h3>
                  {section.paragraphs.map((paragraph) => (
                    <p key={paragraph} className="mt-2 type-body leading-6 text-idpxyz-textMuted">
                      {paragraph}
                    </p>
                  ))}
                  {section.items ? (
                    <dl className="mt-3 flex flex-col gap-3">
                      {section.items.map((item) => (
                        <div key={item.label}>
                          <dt className="type-body font-medium text-idpxyz-text">{item.label}</dt>
                          <dd className="mt-0.5 type-body leading-6 text-idpxyz-textMuted">{item.text}</dd>
                        </div>
                      ))}
                    </dl>
                  ) : null}
                </section>
              ))}
            </div>
          </aside>
        </div>
      ) : null}
    </>
  );
}
