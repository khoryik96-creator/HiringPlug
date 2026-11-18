import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "./ErrorMessage";

type Interview = Schemas["Interview"];
type Invitation = Schemas["InterviewInvitation"];
const labels: Record<Invitation["status"], string> = {
  none: "", pending: "Waiting for Outlook", sent: "Sent in Outlook",
  failed: "Needs attention", conflict: "Changed in Outlook", cancelled: "Cancellation synced",
};

export function InterviewInvitation({ interview }: { interview: Interview }) {
  const [open, setOpen] = useState(false);
  const [teams, setTeams] = useState(false);
  const [minutes, setMinutes] = useState(15);
  const client = useQueryClient();
  const key = ["interview-outlook-invitation", interview.id, interview.version];
  const params = { path: { interviewId: interview.id } };
  const invite = useQuery({
    queryKey: key,
    queryFn: async () => unwrap(await api.GET("/v1/interviews/{interviewId}/outlook-invitation", { params })),
    enabled: open,
    refetchInterval: (q) => {
      if (!open) return false;
      const state = q.state.data;
      if (state?.status === "pending") return 3000;
      return state?.status === "sent" && state.teams && !state.joinUrl ? 15000 : false;
    },
  });
  const send = useMutation({
    mutationFn: async (row: Invitation) => unwrap(await api.POST("/v1/interviews/{interviewId}/outlook-invitation", {
      params,
      body: {
        version: interview.version, teams: row.status === "none" ? row.teamsAvailable && teams : row.teams,
        reminderMinutes: row.status === "none" ? minutes : row.reminderMinutes,
      },
    })),
    onSuccess: (row) => {
      client.setQueryData(key, row);
      void client.invalidateQueries({ queryKey: key });
    },
  });
  const row = invite.data;
  return <>
    <div className="row">
      <button type="button" className="link" aria-expanded={open} onClick={() => { setOpen(!open); }}>Outlook invitation</button>
      {row && row.status !== "none" && <span className="small muted" role="status">{labels[row.status]}</span>}
      {row?.joinUrl && <a href={row.joinUrl} target="_blank" rel="noreferrer">Join Teams</a>}
    </div>
    {open && <section className="panel" aria-label="Outlook invitation">
      {invite.isPending && <p className="muted">Loading Outlook invitation…</p>}
      <ErrorMessage error={invite.error ?? send.error} />
      {row?.issue && <p className="small muted">{row.issue}</p>}
      {row?.canSend && <form className="form-grid" onSubmit={(event) => { event.preventDefault(); send.mutate(row); }}>
        <div className="full small">To {row.recipient}</div>
        {row.status === "none" && <>
          <small className="full muted">Outlook sends an update or cancellation when you change the interview.</small>
          {row.teamsAvailable && <label className="full"><input type="checkbox" checked={teams} onChange={(event) => { setTeams(event.target.checked); }} /> Microsoft Teams meeting</label>}
          <label>Outlook reminder
            <select value={minutes} onChange={(event) => { setMinutes(Number(event.target.value)); }}>
              <option value={0}>Off</option>
              <option value={15}>15 minutes before</option>
              <option value={30}>30 minutes before</option>
              <option value={60}>1 hour before</option>
              <option value={1440}>1 day before</option>
            </select>
          </label>
        </>}
        <div className="full"><button className="primary" type="submit" disabled={send.isPending}>
          {row.status === "none" ? "Send Outlook invitation" : interview.status === "cancelled" ? "Retry Outlook cancellation" : "Retry Outlook invitation"}
        </button></div>
      </form>}
      {row?.status === "none" && !row.canSend && !row.issue && <p className="small muted">No Outlook invitation sent.</p>}
    </section>}
  </>;
}
