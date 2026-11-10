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

Bug-review guarantees: percentages are separated from literal amounts before
parsing; bonus clauses cannot change the pay period of expected base salary.
Annual fixed allowances are divided by 12. Mixed-currency components and
relative expectations require a known matching base currency or a reviewed
converted amount. Negative amounts, ambiguous current ranges and malformed
grouping cannot be applied automatically. Expected-only and known zero salaries
are distinct from a missing current salary. Review status is an explicit API
boolean, not inferred by the web app from English warning text.

Currency hints are taken from the selected base-pay clause, before normalising
amount spacing. Currency on an excluded bonus never relabels base pay, and
normalisation cannot turn a PHP software-version token into a peso hint.
Signs before a currency symbol/code are checked just like signs before digits.
