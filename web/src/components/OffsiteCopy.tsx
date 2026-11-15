// Whether the last backup was also copied off the server (Settings > System
// and Settings > Backups).
import type { Schemas } from "../api/client";
import { formatWhen } from "../labels";

export function OffsiteCopy({ backup }: { backup: Schemas["BackupStatus"] | null | undefined }) {
  if (!backup || backup.status !== "done") return null;
  const when = backup.offsiteAt ? formatWhen(backup.offsiteAt) : "";
  switch (backup.offsiteStatus) {
    case "done":
      return <p className="muted small">✓ Also copied off the server ({when}).</p>;
    case "failed":
      return (
        <p className="warning" role="alert">
          Its copy off the server failed ({when}): {backup.offsiteError}. The backup itself is on this server; check the off-site
          storage settings (HP_OFFSITE_* in the server guide).
        </p>
      );
    case "off":
      return (
        <p className="muted small">
          Not copied off the server: no off-site storage is set up, so losing this server would lose its backups too (see the
          server guide).
        </p>
      );
    case null:
      return null; // made before off-site copies
  }
}
