# Enterprise Agent Platform

## Architecture

```
React/TS Frontend (Ant Design 5 + Vite)
  → Go Platform Backend (Gin + Asynq + PostgreSQL + Redis)
    → Python Agent Service (FastAPI + LangGraph + Qwen/DeepSeek LLM)
```

## Key Principles

- Platform must remain business-domain neutral. V1 implements finance only, but Workflow Engine, Agent Gateway, Audit Log, Approval Engine must not hardcode finance logic.
- New business scenarios (HR, procurement, legal, IT, customer service) are added via: Business App → Workflow Template → Agent Registry → Tool Registry → Domain Policy. Never by writing if/else in platform code.
- Workflow Template explicitly routes to Python Agent Graph via `graph_key`. No LLM-based cross-domain routing.
- Graph isolated per process, Agent reusable by capability, Tool isolated by permission, Domain Policy constrains cross-domain access.
- All state changes persisted by Go backend. All critical actions write audit logs. All agent outputs validated as structured JSON.

## Project Layout

```
enterprise_agent_platform/
├── go-platform/          # Go backend — cmd/server, internal/{auth,workflow,agent,tool,approval,audit,file,business,policy,platform}
├── agent-service/        # Python agent service — app/{main,core,registry,graphs,agents}
├── frontend/             # React frontend — src/{pages,components,services,store,router,hooks}
├── docs/                 # 28 design documents (source of truth)
├── scripts/              # dev-start.sh, dev-start.ps1
├── docker-compose.yml    # 6 services: postgres, redis, minio, qdrant, go-backend, agent-service, frontend
├── .env.example          # Environment variable template
└── CLAUDE.md             # This file
```

## Technology Stack

| Layer | Technology | Versions |
|---|---|---|
| Frontend | React 18 + TypeScript + Vite + Ant Design 5 + Zustand | Latest |
| Backend | Go 1.22 + Gin + Asynq + pgx + golang-jwt | See go.mod |
| Agent | Python 3.12 + FastAPI 0.136+ + LangGraph 1.2+ + LangChain 1.2+ | See requirements.txt |
| Database | PostgreSQL 16 | — |
| Cache/Queue | Redis 7 + Asynq | — |
| Storage | MinIO | — |
| Vector | Qdrant | — |
| LLM | Qwen (primary) / DeepSeek (alternative) | OpenAI-compatible API |
| Deployment | Docker Compose | — |

## Available Skills

### Built-in
- `/review` — Review pending changes (style, tests, architecture, security)
- `/security-review` — Complete security audit of current branch changes
- `/simplify` — Review changed code for reuse, quality, and efficiency

### Pensive Plugin (installed)
- `pensive:unified-review` — Orchestrates multi-domain review (code, arch, tests, security) in a single pass
- `pensive:code-refinement` — Improves code quality across duplication, efficiency, and architectural fit
- `pensive:architecture-review` — Assesses architecture decisions, ADR compliance, and coupling
- `pensive:bug-review` — Hunts bugs with evidence trails
- `pensive:blast-radius` — Analyzes code change impact with risk scoring
- `pensive:api-review` — Evaluates API surface design and consistency
- `pensive:safety-critical-patterns` — NASA Power of 10 rules (relevant: finance system)
- `pensive:test-review` — Evaluates test coverage gaps and anti-patterns
- `pensive:performance-review` — Detects O(n²) and complexity hotspots

### Leyline Plugin (installed, pensive dependency)
- `leyline:risk-classification` — Classifies actions into 4 risk tiers (GREEN/YELLOW/RED/CRITICAL)
- `leyline:supply-chain-advisory` — Audits dependency supply chains
- `leyline:additive-bias-defense` — Inverts burden of proof for code additions

## Code Review Protocol

After generating or modifying 3+ files in a session, you MUST self-review before reporting the task as complete:

### Automatic checks (every session with code changes)
1. **Consistency**: Do the new changes align with the 28 design docs in `docs/`?
2. **No dead code**: Are all imports used? Are all env vars consumed? Are all functions called?
3. **No platform-coupling**: New code in Go `internal/` must not import finance-specific logic. Workflow Engine must not contain business-scenario if/else.
4. **Cross-stack contract**: Does the Go API response shape match what the frontend `api.ts` expects? Does the Python agent output shape match the `agent_run_logs` schema?
5. **Security**: No hardcoded secrets, no SQL injection via string concatenation, JWT on all protected endpoints.
6. **Extension points intact**: New code must not block adding HR/procurement/legal/IT/customer-service business apps later.

### Formal review triggers
| When | Which skill |
|---|---|
| New Go `internal/` module or API endpoint | `pensive:api-review` |
| Cross-module changes affecting 2+ Go packages | `pensive:architecture-review` |
| Python Agent or LangGraph graph changes | `pensive:bug-review` + `pensive:blast-radius` |
| Database migration or schema change | `pensive:blast-radius` (risk scoring) |
| Before committing to main/PR | `pensive:unified-review` |
| Finance-specific logic (high risk) | `pensive:safety-critical-patterns` |
| Adding new dependencies | `leyline:supply-chain-advisory` |
| General code quality check | `pensive:code-refinement` |

## Phase Implementation Order

0. Project skeleton + Docker Compose (done)
1. Auth / RBAC (users, roles, permissions, JWT) — done
2. Workflow Core (templates, instances, nodes, state machine, Asynq) — done
3. Agent Registry & Gateway (agent/tool registration, domain policy, agent run log) — done
4. Python Agent Service (6 finance agents, LangGraph graphs, RAG) — done
5. Frontend Workbench (login, finance center, workflow detail, approval, audit) — done
6. Audit, Observability, Deployment (audit log completion, token/cost, Docker demo) — done
7. V1.1 Iteration (unified error handling, agent output persistence, unit tests, domain policy strict mode, agent singleton, frontend polling, docs) — done

## Seed Data

4 demo users (password: `password`):
- `admin` / `finance_user` / `finance_manager` / `ops_viewer`
