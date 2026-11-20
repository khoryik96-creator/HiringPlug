import type { Permission } from "../../api/client";
import { CloseIcon, IconButton } from "../ActionIcons";
import { EmailComposer } from "../EmailComposer";
import { OutlookDialog } from "./OutlookDialog";
import type { OutlookProfile } from "./OutlookProfileMail";

export function OutlookATSComposer({ profile, permissions, onClose }: {
  profile?: OutlookProfile; permissions: Permission[]; onClose: () => void;
}) {
  return <OutlookDialog className="outlook-draft-dialog outlook-ats-compose" labelledBy="outlook-ats-heading" onCancel={onClose}>
    <div className="row spread outlook-draft-head">
      <h3 id="outlook-ats-heading">Write with HiringPlug</h3>
      <IconButton label="Close" icon={<CloseIcon />} onClick={onClose} />
    </div>
    <EmailComposer {...profile} iconTools permissions={permissions} canManageTemplates={permissions.includes("emails.templates:manage")}
      canCreateTemplates={permissions.includes("emails.templates:create")} canBook={permissions.includes("scheduling:use")}
      onDone={onClose} />
  </OutlookDialog>;
}
