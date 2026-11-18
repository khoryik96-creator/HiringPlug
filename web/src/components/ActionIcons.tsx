// Small line icons for page actions, drawn inline (no icon library): each is
// an 18px stroke drawing in the current text colour.
import type { ReactNode } from "react";

const Svg = ({ children }: { children: ReactNode }) => (
  <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
    {children}
  </svg>
);

export const MailIcon = () => <Svg><rect x="3" y="5" width="18" height="14" rx="2" /><path d="m3 7 9 6 9-6" /></Svg>;
export const PhoneIcon = () => (
  <Svg><path d="M5 4h4l2 5-2.5 1.5a11 11 0 0 0 5 5L15 13l5 2v4a2 2 0 0 1-2 2A16 16 0 0 1 3 6a2 2 0 0 1 2-2Z" /></Svg>
);
export const ReferenceIcon = () => (
  <Svg><path d="M9 4h6v3H9z" /><path d="M15 5h2a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2h2" /><path d="m9 14 2 2 4-4" /></Svg>
);
export const EditIcon = () => <Svg><path d="M4 20h4L19 9l-4-4L4 16v4Z" /><path d="m13.5 6.5 4 4" /></Svg>;
export const TrashIcon = () => (
  <Svg><path d="M4 7h16" /><path d="M10 11v6M14 11v6" /><path d="M6 7l1 12a2 2 0 0 0 2 2h6a2 2 0 0 0 2-2l1-12" /><path d="M9 7V4h6v3" /></Svg>
);
export const BuildingIcon = () => (
  <Svg><rect x="5" y="3" width="14" height="18" rx="2" /><path d="M9 7h2M13 7h2M9 11h2M13 11h2M10 21v-4h4v4" /></Svg>
);
export const PersonIcon = () => <Svg><circle cx="12" cy="8" r="4" /><path d="M4 21a8 8 0 0 1 16 0" /></Svg>;
export const SparkIcon = () => <Svg><path d="M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8Z" /></Svg>;
export const ShareIcon = () => (
  <Svg><circle cx="9" cy="8" r="3.5" /><path d="M2.5 20a6.5 6.5 0 0 1 13 0" /><path d="M19 8v6M16 11h6" /></Svg>
);
export const LeaveIcon = () => <Svg><path d="M14 4h4a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-4" /><path d="M10 16l-4-4 4-4" /><path d="M6 12h10" /></Svg>;
export const MailCheckIcon = () => (
  <Svg><path d="M21 12V7a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h7" /><path d="m3 7 9 6 9-6" /><path d="m16 19 2 2 4-4" /></Svg>
);

/**
 * A page's actions as a neat row of icons (.icon-actions): each named for
 * screen readers and in a tooltip shown on hover or keyboard focus.
 */
export function IconActions({ label, children }: { label: string; children: ReactNode }) {
  return <span className="icon-actions" role="toolbar" aria-label={label}>{children}</span>;
}

export function IconButton({ label, icon, onClick, disabled = false, tip = label, danger = false }: {
  label: string; icon: ReactNode; onClick: () => void; disabled?: boolean; tip?: string;
  /** Deletes or removes something: red on hover. */
  danger?: boolean;
}) {
  return (
    <button type="button" className={danger ? "icon-action icon-danger" : "icon-action"} aria-label={label} data-tip={tip}
      disabled={disabled} onClick={onClick}>{icon}</button>
  );
}

export function IconLink({ label, icon, href }: { label: string; icon: ReactNode; href: string }) {
  return <a className="icon-action" href={href} aria-label={label} data-tip={label}>{icon}</a>;
}

/** A thin line before the delete icon, which stands apart. */
export const IconSeparator = () => <span className="icon-sep" aria-hidden="true" />;
