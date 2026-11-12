import { type Section, SectionTabs } from "./SectionTabs";

export type CandidateTab = "details" | "cv" | "jobs" | "activity" | "salary" | "email";
export type CandidateSection = Section<CandidateTab>;

/** A candidate's sections as tabs (see SectionTabs). */
export function CandidateTabs({ sections, active, onChange }: {
  sections: CandidateSection[]; active: CandidateTab; onChange: (tab: CandidateTab) => void;
}) {
  return <SectionTabs name="candidate" label="Candidate sections" sections={sections} active={active} onChange={onChange} />;
}
