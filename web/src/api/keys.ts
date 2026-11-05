// Query keys shared by the screens that read data and the forms that change
// it. Keys are plain arrays, so a typo would compile and silently stop a page
// from updating after a save; using these avoids that.
export const candidateKey = (id: string) => ["candidate", id] as const;
export const candidatesKey = ["candidates"] as const;
export const companyKey = (id: string) => ["company", id] as const;
export const companiesKey = ["companies"] as const;
export const contactKey = (id: string) => ["contact", id] as const;
export const contactsKey = ["contacts"] as const;
export const jobKey = (id: string) => ["job", id] as const;
export const jobsKey = ["jobs"] as const;
export const jobApplicationsKey = (jobId: string) => ["applications", "job", jobId] as const;
export const candidateApplicationsKey = (candidateId: string) => ["applications", "candidate", candidateId] as const;
export const applicationsKey = ["applications"] as const;
export const notesKey = (subjectType: string, subjectId: string) => ["notes", subjectType, subjectId] as const;
export const activityKey = (entityType: string, entityId: string) => ["activity", entityType, entityId] as const;
export const summaryKey = ["pipeline-summary"] as const;
export const usedValuesKey = ["candidates", "used-values"] as const;
export const jobAdsKey = (jobId: string) => ["job-ads", jobId] as const;
export const interviewsKey = ["interviews"] as const;
export const applicationInterviewsKey = (applicationId: string) => ["interviews", "application", applicationId] as const;
export const usersKey = ["users"] as const;
export const emailTemplatesKey = ["email-templates"] as const;
export const candidateEmailsKey = (candidateId: string) => ["emails", candidateId] as const;
export const placementsKey = ["placements"] as const;
export const applicationPlacementKey = (applicationId: string) => ["placements", "application", applicationId] as const;
export const offersKey = ["offers"] as const;
export const applicationOffersKey = (applicationId: string) => ["offers", "application", applicationId] as const;
export const companyAutofillKey = (id: string) => ["company", id, "autofill"] as const;
export const assistantKey = ["assistant"] as const;
export const assistantAnswersKey = (subjectType: string, subjectId: string) => ["assistant", "answers", subjectType, subjectId] as const;
export const matchesKey = ["matches"] as const;
export const jobMatchesKey = (jobId: string) => ["matches", "job", jobId] as const;
export const applicationMatchKey = (applicationId: string) => ["matches", "application", applicationId] as const;
export const privacyKey = ["privacy"] as const;
export const candidatePrivacyKey = (candidateId: string) => ["privacy", "candidate", candidateId] as const;
export const accessKey = ["access"] as const;
export const teamsKey = ["teams"] as const;
export const jobSuggestionsKey = (jobId: string) => ["suggestions", jobId] as const;
export const fieldSettingsKey = ["settings", "fields"] as const;
export const cvImportsKey = ["cv-imports"] as const;
// Under applicationsKey: every add or move of an application refreshes the timeline.
export const stageMovesKey = (candidateId?: string) => ["applications", "moves", candidateId ?? "all"] as const;
