# 0002 One codebase split into modules (modular monolith)

**Decision.** The Go backend is one deployable application, divided into
modules with enforced boundaries. Not microservices.

**Why.** Microservices add network failures, distributed debugging, data
consistency across services and much more to operate, a heavy cost for a small
team. A modular monolith keeps one process and one database transaction per
action, while giving modules the separation that makes change safe:

- each module owns its contract file, tables and SQL;
- modules never import each other (an architecture test fails if they do);
  they meet only in `internal/app/modules.go` through small interfaces;
- Go's `internal` directories make each module's storage code private to it.

**Scaling.** API and worker copies scale horizontally. A module that ever needs
to scale on its own can be moved out: its contract file and its job messages
are already its boundary.
