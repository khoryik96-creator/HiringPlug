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
export const PersonPlusIcon = () => <Svg><circle cx="10" cy="8" r="4" /><path d="M3 21a7 7 0 0 1 13.5-2.6M19 14v6m-3-3h6" /></Svg>;
export const CheckIcon = () => <Svg><path d="m5 12 4.5 4.5L19 7" /></Svg>;
export const AlertIcon = () => <Svg><circle cx="12" cy="12" r="9" /><path d="M12 7.5v5.5m0 3.2h.01" /></Svg>;
export const PersonIcon = () => <Svg><circle cx="12" cy="8" r="4" /><path d="M4 21a8 8 0 0 1 16 0" /></Svg>;
export const SparkIcon = () => <Svg><path d="M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8Z" /></Svg>;
export const ShareIcon = () => (
  <Svg><circle cx="9" cy="8" r="3.5" /><path d="M2.5 20a6.5 6.5 0 0 1 13 0" /><path d="M19 8v6M16 11h6" /></Svg>
);
export const LeaveIcon = () => <Svg><path d="M14 4h4a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-4" /><path d="M10 16l-4-4 4-4" /><path d="M6 12h10" /></Svg>;
/** Several emails: one to each person. */
export const MailManyIcon = () => (
  <Svg><rect x="6" y="8" width="15" height="11" rx="2" /><path d="m6 10 7.5 5 7.5-5M3 15V7a2 2 0 0 1 2-2h12" /></Svg>
);
export const MailCheckIcon = () => (
  <Svg><path d="M21 12V7a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h7" /><path d="m3 7 9 6 9-6" /><path d="m16 19 2 2 4-4" /></Svg>
);
export const PlusIcon = () => <Svg><path d="M12 5v14M5 12h14" /></Svg>;
export const CloseIcon = () => <Svg><path d="m6 6 12 12M18 6 6 18" /></Svg>;
export const SideBySideIcon = () => <Svg><rect x="3" y="5" width="18" height="14" rx="2" /><path d="M12 5v14" /></Svg>;
export const StepsIcon = () => <Svg><path d="M8 6h12M8 12h12M8 18h12M4 6h.1M4 12h.1M4 18h.1" /></Svg>;
export const BoardIcon = () => <Svg><rect x="3" y="5" width="18" height="14" rx="2" /><path d="M9 5v14m6-14v14" /></Svg>;
export const DockRightIcon = () => <Svg><rect x="3" y="5" width="18" height="14" rx="2" /><path d="M15 5v14m-8-7h4m-2-2 2 2-2 2" /></Svg>;
export const OutlookIcon = () => (
  <Svg><rect x="2" y="4" width="11" height="15" rx="2" /><ellipse cx="7.5" cy="11.5" rx="2.2" ry="2.8" /><path d="M13 6h6a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2h-9a2 2 0 0 1-2-2m5-9 4 3 4-3" /></Svg>
);
export const UploadIcon = () => (
  <Svg><path d="M12 16V4m-5 5 5-5 5 5M4 16v3a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-3" /></Svg>
);
export const DownloadIcon = () => (
  <Svg><path d="M12 4v12m-5-5 5 5 5-5M4 16v3a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-3" /></Svg>
);
export const ViewIcon = () => <Svg><path d="M2 12s3.6-6 10-6 10 6 10 6-3.6 6-10 6S2 12 2 12Z" /><circle cx="12" cy="12" r="2.5" /></Svg>;
export const RefreshIcon = () => <Svg><path d="M20 5v5h-5m5 0a8 8 0 1 0 .1 4" /></Svg>;
export const FileIcon = () => <Svg><path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9ZM14 3v6h6" /></Svg>;
export const TextIcon = () => <Svg><path d="M4 6h16M4 10h16M4 14h16M4 18h10" /></Svg>;
export const ComposeIcon = () => <Svg><path d="M12 4H6a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-6M13 7l4 4m-7 3 3-.5 7-7-2.5-2.5-7 7z" /></Svg>;
export const ClockIcon = () => <Svg><circle cx="12" cy="12" r="8" /><path d="M12 7v5l3 2" /></Svg>;
export const PaperclipIcon = () => <Svg><path d="m9 13 6-6a2.2 2.2 0 0 1 3 3l-8 8a4 4 0 0 1-5.7-5.7l8-8" /></Svg>;
export const ReplyIcon = () => <Svg><path d="m10 5-6 6 6 6v-4c5 0 8 2 10 5 0-7-3-9-10-9z" /></Svg>;
export const ReplyAllIcon = () => <Svg><path d="m6 7-4 4 4 4m5-10-6 6 6 6v-4c5 0 8 2 10 5 0-7-3-9-10-9z" /></Svg>;
export const ForwardIcon = () => <Svg><path d="m14 5 6 6-6 6v-4c-5 0-8 2-10 5 0-7 3-9 10-9z" /></Svg>;
export const ArchiveIcon = () => <Svg><path d="M4 5h16v4H4zm2 4v10h12V9m-8 4h4" /></Svg>;
export const MoveFolderIcon = () => <Svg><path d="M3 8V6h6l2 2h10v11H3zm6 5h7m-3-3 3 3-3 3" /></Svg>;
export const ConversationIcon = () => <Svg><path d="M4 5h16v12h-7l-4 4v-4H4zm4 4h8m-8 4h5" /></Svg>;
export const LinkIcon = () => <Svg><path d="m10 14 4-4m-7 2-2 2a3.5 3.5 0 0 0 5 5l3-3m4-4 2-2a3.5 3.5 0 0 0-5-5l-3 3" /></Svg>;
// The message editor's formatting tools.
export const StrikeIcon = () => (
  <Svg><path d="M4 12h16" /><path d="M16.5 7.5C16 5.9 14.3 5 12 5 9.5 5 8 6.3 8 8c0 1.2.8 2.2 2.4 2.8M8 16.5c.5 1.6 2.2 2.5 4.5 2.5 2.5 0 4-1.3 4-3 0-.8-.3-1.5-.9-2" /></Svg>
);
export const NumberedListIcon = () => (
  <Svg><path d="M10 6h10M10 12h10M10 18h10M4 4.5 5.5 4v5M4 9h3" /><path d="M4 14.5a1.5 1.5 0 0 1 3 0c0 1.4-3 2-3 4.5h3" /></Svg>
);
export const IndentIcon = () => <Svg><path d="M4 5h16M11 10h9M11 14h9M4 19h16M4 9l3 3-3 3" /></Svg>;
export const OutdentIcon = () => <Svg><path d="M4 5h16M11 10h9M11 14h9M4 19h16M7 9l-3 3 3 3" /></Svg>;
export const AlignLeftIcon = () => <Svg><path d="M4 6h16M4 10h10M4 14h16M4 18h10" /></Svg>;
export const AlignCenterIcon = () => <Svg><path d="M4 6h16M7 10h10M4 14h16M7 18h10" /></Svg>;
export const AlignRightIcon = () => <Svg><path d="M4 6h16M10 10h10M4 14h16M10 18h10" /></Svg>;
export const ClearFormatIcon = () => <Svg><path d="M5 5h12M11 5 8.5 17M14 14l6 6m0-6-6 6" /></Svg>;
export const FlagIcon = () => <Svg><path d="M5 21V4m0 0h11l-2 4 2 4H5" /></Svg>;
export const MoreIcon = () => <Svg><path d="M5 12h.1m7 0h.1m7 0h.1" /></Svg>;
export const PowerIcon = () => <Svg><path d="M12 3v9m5-6a8 8 0 1 1-10 0" /></Svg>;
export const CalendarPlusIcon = () => (
  <Svg><rect x="3" y="5" width="18" height="16" rx="2" /><path d="M8 3v4m8-4v4M3 10h18m-9 3v5m-2.5-2.5h5" /></Svg>
);
export const OfferIcon = () => (
  <Svg><rect x="5" y="3" width="14" height="18" rx="2" /><path d="M8 7h8m-8 4h8m-7 5 2 2 4-4" /></Svg>
);

/**
 * A page's actions as a neat row of icons (.icon-actions): each named for
 * screen readers and in a tooltip shown on hover or keyboard focus.
 */
export function IconActions({ label, children }: { label: string; children: ReactNode }) {
  return <span className="icon-actions" role="toolbar" aria-label={label}>{children}</span>;
}

export function IconButton({ label, icon, onClick, disabled = false, tip = label, danger = false, pressed, className = "" }: {
  label: string; icon: ReactNode; onClick: () => void; disabled?: boolean; tip?: string;
  /** Deletes or removes something: red on hover. */
  danger?: boolean;
  pressed?: boolean;
  className?: string;
}) {
  return (
    <button type="button" className={["icon-action", danger && "icon-danger", className].filter(Boolean).join(" ")}
      aria-label={label} aria-pressed={pressed} data-tip={disabled ? undefined : tip} title={disabled ? tip : undefined}
      disabled={disabled} onClick={onClick}>{icon}</button>
  );
}

export function IconLink({ label, icon, href, external = false }: { label: string; icon: ReactNode; href: string; external?: boolean }) {
  return <a className="icon-action" href={href} aria-label={label} data-tip={label}
    target={external ? "_blank" : undefined} rel={external ? "noreferrer" : undefined}>{icon}</a>;
}

/** A thin line before the delete icon, which stands apart. */
export const IconSeparator = () => <span className="icon-sep" aria-hidden="true" />;
