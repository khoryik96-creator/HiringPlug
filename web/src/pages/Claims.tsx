// Expense claims: your own (drop receipts, check what was read, submit),
// approvals for those who decide them, and the dashboard.
import { useState } from "react";

import type { Permission } from "../api/client";
import { ClaimApprovals } from "../components/claims/ClaimApprovals";
import { ClaimsDashboard } from "../components/claims/ClaimsDashboard";
import { MyClaims } from "../components/claims/MyClaims";
import { type Section, SectionTabs } from "../components/SectionTabs";
import { hashParam } from "../hashQuery";
import "./claims-workspace.css";

type Tab = "mine" | "approvals" | "dashboard";

export function Claims({ permissions }: { permissions: Permission[] }) {
  const canClaim = permissions.includes("claims:create");
  const canApprove = permissions.includes("claims:approve");
  const [tab, setTab] = useState<Tab>(() => {
    const asked = hashParam("tab");
    return asked === "approvals" || asked === "dashboard" ? asked : canClaim ? "mine" : "approvals";
  });
  const sections: Section<Tab>[] = [
    ...(canClaim ? [{ id: "mine" as const, label: "My claims", content: <MyClaims permissions={permissions} /> }] : []),
    ...(canApprove ? [{ id: "approvals" as const, label: "Approvals", content: <ClaimApprovals /> }] : []),
    { id: "dashboard", label: "Dashboard", content: <ClaimsDashboard canSeeTeam={canApprove} /> },
  ];
  return (
    <main className="claims-page list-page">
      <h2>Claims</h2>
      <SectionTabs name="claims" label="Claims" sections={sections} active={tab} onChange={setTab} />
    </main>
  );
}
