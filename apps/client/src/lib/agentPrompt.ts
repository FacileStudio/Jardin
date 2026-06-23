export const AGENT_PROMPT = `# Ruche — Shared Agent Memory

You are connected to **Ruche**, a shared memory/rules/skills system synced across all my
machines and AI agents through the \`ruche\` CLI. Treat \`~/.ruche/memory/\` as your persistent,
cross-session, cross-machine brain. Query and write it silently — it is infrastructure, not
conversation.

## Operating loop (every non-trivial task)

1. **Before acting:** run \`ruche sync\` to pull the latest shared brain, then
   \`ruche memory search "<keywords>"\` (and skim \`ruche memory index\`) to reuse what is
   already known instead of rediscovering it.
2. **Do the work.**
3. **After:** if you learned something durable and non-obvious, write it to memory (below),
   then \`ruche sync\` to share it with every other machine and agent.

## Reading memory

- \`ruche memory search "<query>"\` — case-insensitive substring search over all memory
  markdown; returns \`path:line\`.
- \`ruche memory index\` — prints \`index.md\`, the router / table of contents.
- Memory is plain markdown under \`~/.ruche/memory/\`: \`bugs/\`, \`tools/\`, \`projects/\`,
  \`conventions/\`, \`syntheses/\`, plus \`index.md\` (router), \`overview.md\` (always-read
  summary), and \`log.md\` (append-only history).

## Writing memory

There is no \`ruche memory add\` — write the markdown files directly with your normal tools.

- Pick the right subdir; prefer updating an existing page over creating a new one.
- Frontmatter on every page: \`title\`, \`type\`, \`sources\`, \`related\`, \`confidence\`,
  \`created\`, \`updated\`.
- Keep entries to 2–6 lines of substance. Every non-obvious claim needs provenance: a URL,
  a file path, or "direct observation". Link related pages with [[page-name]].
- After writing: add a one-line pointer in \`index.md\`, append a dated line to \`log.md\`,
  then \`ruche sync\`.

**Storage gate — only write when ALL are true:** (1) it will change how a future agent acts,
(2) it is non-obvious or annoying to rediscover, (3) it is grounded in a source or direct
observation. Otherwise skip. Never store: things obvious from current code, raw re-runnable
command output, git history, or ephemeral session state.

## Rules, skills, and generating configs

- \`ruche rules list\` / \`ruche rules edit <name>\` — shared rules (\`~/.ruche/rules/\`).
- \`ruche skills list\` / \`ruche skills add <name>\` — shared skills (\`~/.ruche/skills/\`).
- \`ruche install <agent>\` (or \`--all\`) regenerates this agent's config from rules + skills +
  machine block; \`ruche diff <agent>\` previews first. Agents: claude, codex, gemini, cursor,
  copilot, hermes.

## Sync

\`ruche sync\` (pull+push), \`ruche push\`, \`ruche pull\`, \`ruche status\`. Sync early, sync
often — the brain is only as shared as your last sync.
`;
