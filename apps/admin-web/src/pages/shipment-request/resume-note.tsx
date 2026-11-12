import type { ResumeNote } from './last-attempt';

/** 「谁来续办」那一句；能链到登记处时跟一个链接。「受理进度」与「受理前财务控制」同一种摆法。 */
export function ResumeNoteText({ note }: { note: ResumeNote }) {
  return (
    <span>
      {note.text}
      {note.link ? (
        <>
          {' '}
          <a href={note.link.href} className="text-idpxyz-accent hover:underline focus-visible:underline">
            {note.link.label}
          </a>
        </>
      ) : null}
    </span>
  );
}
