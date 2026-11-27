// The dashboard's getting-started checklist for a new company. Its steps tick
// themselves from the company's records; once they are all done it goes away,
// and anyone can hide it sooner (remembered for them; the help page shows it
// again).
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { preferencesKey } from "./candidateColumns";
import { stepsFor } from "../help/firstRun";
import "../help/help-screens.css";

/** Saves whether the checklist is hidden, showing the change at once. */
export function useHideGuide() {
  const queryClient = useQueryClient();
  return useMutation({
    scope: { id: "first-run-guide" },
    mutationFn: async (guideDismissed: boolean) => unwrap(await api.PUT("/v1/me/preferences", { body: { guideDismissed } })),
    onMutate: async (guideDismissed) => {
      await queryClient.cancelQueries({ queryKey: preferencesKey });
      queryClient.setQueryData(preferencesKey, (old: Schemas["Preferences"] | undefined) => ({ ...old, guideDismissed }));
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: preferencesKey }),
  });
}

export function FirstRunGuide({ permissions }: { permissions: readonly Permission[] }) {
  const steps = stepsFor(permissions);
  const prefs = useQuery({ queryKey: preferencesKey, queryFn: async () => unwrap(await api.GET("/v1/me/preferences")), staleTime: 60_000 });
  const hidden = prefs.data?.guideDismissed === true;
  const checks = useQueries({
    queries: steps.map((s) => ({ queryKey: ["first-run", s.id], queryFn: s.done, enabled: prefs.isSuccess && !hidden, staleTime: 30_000 })),
  });
  const hide = useHideGuide();
  // Nothing until everything is known, so an agency already set up never sees it flash by.
  if (!prefs.isSuccess || hidden || steps.length === 0 || checks.some((c) => c.isPending)) return null;
  // A step that could not be checked (e.g. a role changed meanwhile) is left out.
  const shown = steps.flatMap((s, i) => (checks[i]?.isSuccess ? [{ step: s, done: checks[i].data }] : []));
  const done = shown.filter((s) => s.done).length;
  if (shown.length === 0 || done === shown.length) return null;

  return (
    <section className="panel first-run" aria-labelledby="first-run-title">
      <div className="first-run-head">
        <div>
          <h3 id="first-run-title">Get HyPlug ready</h3>
          <p className="muted">A few steps for a new company. Each ticks itself once it is done.</p>
        </div>
        <button type="button" className="link small" onClick={() => { hide.mutate(true); }}>Hide checklist</button>
      </div>
      <div className="first-run-progress">
        <progress max={shown.length} value={done} aria-label="Steps done" />
        <span>{done} of {shown.length} done</span>
      </div>
      <ol className="first-run-steps">
        {shown.map(({ step, done: isDone }) => (
          <li key={step.id} className={isDone ? "done" : undefined}>
            <span className="first-run-tick" aria-hidden="true">{isDone ? "✓" : ""}</span>
            <div className="first-run-text">
              <strong>{step.title}</strong>
              <span className="muted">{isDone ? "Done" : step.text}</span>
            </div>
            {!isDone && <a className="button small-button" href={step.href}>{step.action}</a>}
            <span className="visually-hidden">{isDone ? "(done)" : "(to do)"}</span>
          </li>
        ))}
      </ol>
      <p className="small muted first-run-foot">Questions? <a href="#/help">Open the help</a> (the ? in the top bar).</p>
    </section>
  );
}
