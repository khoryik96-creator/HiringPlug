# 0006 Local first, cloud ready

**Decision.** HiringPlug runs on one machine with Docker Compose (PostgreSQL,
API, both workers, web) and uses only portable building blocks, so the same
containers move to the cloud later.

**How it stays portable.**
- Files are stored behind an interface and handed to workers as **signed
  links**. On disk today; in the cloud, S3 presigned links, with no change to
  the workers.
- Settings come from environment variables (`HP_*`, all in `.env.example`).
- Plain PostgreSQL (local, then managed), plain containers, plain HTTP;
  OpenID Connect for single sign-on when hosted.

**Operating locally.** Back up the database (`pg_dump`) and the files volume;
upgrade by pulling new images (migrations run automatically by `hp bootstrap`).
See [playbooks/run-locally.md](../playbooks/run-locally.md).
