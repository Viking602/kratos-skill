# Kratos Skill

Practical guidance for designing, implementing, and troubleshooting Go-Kratos services.

中文 [README_CN.md](./README_CN.md).

This repository is a single-skill package for `npx skills`. The skill entry point is the root [`SKILL.md`](./SKILL.md). This `README.md` is only for repository readers and does not participate in skill discovery.

## Install

Quick install from GitHub:

```bash
npx skills add viking602/kratos-skill
```

This starts the interactive installer for the skill package.

Install from a GitHub repository:

```bash
npx skills add <owner>/<repo>
```

Install from a local path:

```bash
npx skills add /path/to/kratos-skill
```

List the skill without installing:

```bash
npx skills add <owner>/<repo> --list
```

Install only this skill for Codex without prompts:

```bash
npx skills add viking602/kratos-skill -a codex -s kratos-skill -y
```

## Manage

```bash
npx skills list
npx skills check
npx skills update
npx skills remove kratos-skill -y
```

Project-level installs are typically written to `./.agents/skills/kratos-skill`. Global installs use `-g`.

## What The Skill Covers

- `api/**/*.proto`, `errors.proto`, and validation rules
- `make api`, `make errors`, and `make validate`
- Kratos layer boundaries across `internal/{biz,data,service,server}`
- Wire setup, middleware ordering, auth selectors, and service discovery
- Cross-service gRPC calls and Kratos-style error handling

## Repository Layout

```text
.
├── SKILL.md
├── agents/openai.yaml
├── references/
├── examples/
├── evals/
├── best-practices.md
└── troubleshooting.md
```

- `SKILL.md`: skill metadata and execution guidance
- `agents/openai.yaml`: UI metadata for skill-aware clients
- `references/`: task-specific reference material loaded on demand
- `examples/`: example proto and Go snippets
- `evals/`: evaluation fixtures

## Development Notes

- Keep skill behavior in `SKILL.md`; keep repository documentation in `README.md`.
- Preserve root-level `SKILL.md` and `agents/openai.yaml` so `npx skills` can discover the package reliably.
- When changing examples or references, prefer updating linked files instead of expanding `SKILL.md` unnecessarily.
