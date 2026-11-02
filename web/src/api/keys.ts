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
