# Salary calculation

Go adaptation of Kano's salary scanner (`khoryik96-creator/Kano`, main
`9a50bca`), from the salary handover supplied by the owner on 10 November 2026.
Reference entry points: `calcFixedSalaryDetailed`, `applyExpectedPercentIncrementRule`,
`applyExpectedSameAsCurrentRule`, `applySalaryCurrencyRule` and rate detection.
The reference's original 148 checks passed before adaptation.

HiringPlug uses deterministic Go code only. Salary is never sent to AI. The
calculator previews values; the explicit, versioned screening-call transaction
saves them. Raw descriptions stay in the existing protected compensation table.

Company settings control fixed allowances, guaranteed bonus, bonus basis and
rounding. Variable/conditional components stay out unless an allowance is explicitly
described as fixed. Settings changes do not recalculate saved compensation.

Intentional differences from Kano:

- Complete descriptions are parsed, avoiding regexFallback's 80-character limit.
- AWS and fixed/contractual bonus aliases include common typing mistakes.
- Extra bonus months (2) differ from total guaranteed months (14).
- Generic fixed bonus without a size and conflicting totals require review.
- Conditional shift pay and “up to” bonus are excluded.
- All reviewed figures are monthly equivalents. Annual base is divided by 12;
  hourly/daily/weekly conversion needs working-time information and is refused.
- Standalone “lateral” works, but explicit amounts and percentages take precedence.
- Currency is read only from explicit salary wording. Unstated currency remains
  the recruiter's chosen form currency; no nationality or location inference.

The fixtures in salary_test.go use synthetic amounts. Extend them when a recruiter
reports a miss. Do not add real candidates or CVs to tests.
