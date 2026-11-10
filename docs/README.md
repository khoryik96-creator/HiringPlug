# HiringPlug documentation: start here

Read in this order. Each page is short and links to the next.

1. **[Architecture](architecture.md)**: how the system is put together and why.
   Read this first.
2. **[Features](features.md)**: your feature list, mapped to modules and
   milestones, with the design of each.
3. **[Guardrails](guardrails.md)**: what stops changes from breaking things, and
   where each rule is enforced.
4. **[Access control](access-control.md)**: roles, permissions, and how salary
   and other sensitive data stay hidden.
5. **[Data model](data-model.md)**: the records and their fields, today and planned.
6. **[AI worker](ai-worker.md)**: reading CVs (text, OCR), parsing,
   formatting and blind CVs, the AI assistant and CV vs job matching.
7. **[Integrations](integrations.md)**: job ads puller, company auto-fill, email
   verifier, email, calendars, job boards, public API.
8. **[Roadmap](roadmap.md)**: the milestones, in order.

- [HANDOVER.md](HANDOVER.md): where the work stands and what is next (read first when picking the project up).
- [Turning HiringPlug into a SaaS: scaling to 20,000 users](saas-plan-scaling-to-20k-users.md): what to change, set-up and costs at each size, Python or Go.
- [Profit Planner](https://claude.ai/artifact/SXZUgNHU16BkqiUHX6uU2G) (source: [pricing-calculator.html](pricing-calculator.html)): prices, users, costs and tax in, margin, break-even and a forecast out.

## How-to guides

- [Add a module](playbooks/add-a-module.md)
- [Add an endpoint or a field](playbooks/add-an-endpoint.md)
- [Fix a bug](playbooks/fix-a-bug.md)
- [Efficient development and CI diagnosis](playbooks/development-workflow.md)
- [Debug a problem](playbooks/debug-a-problem.md)
- [Run it on your own machine or server](playbooks/run-locally.md)

## Decisions (and why)

Short records of the big choices, so nobody has to re-argue them:

- [0001 Go + Python + TypeScript](adr/0001-languages.md)
- [0002 One codebase split into modules (modular monolith)](adr/0002-modular-monolith.md)
- [0003 PostgreSQL for data, jobs and (at first) search](adr/0003-postgresql.md)
- [0004 Contract first, one file per module](adr/0004-contract-first.md)
- [0005 Tenant and field security inside the database](adr/0005-database-security.md)
- [0006 Local first, cloud ready](adr/0006-local-first.md)
