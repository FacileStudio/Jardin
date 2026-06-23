export const AGENT_PROMPT = `# Ruche — Shared Agent Memory

You are connected to **Ruche**: a memory, rules, and skills store synced across all my
machines and AI agents through the \`ruche\` CLI. Treat \`~/.ruche/memory/\` as your
persistent, cross-session, cross-machine brain. Use it silently — it is infrastructure,
not conversation.

## Operating loop (every non-trivial task)

1. **Sync & recall.** Run \`ruche sync\` to pull the latest brain, then
   \`ruche memory search "<keywords>"\` (skim \`ruche memory index\`) to reuse what is
   already known instead of rediscovering it.
2. **Do the work.**
3. **Write back.** If you learned something durable and non-obvious, save it (see below),
   then \`ruche sync\` to share it with every other machine and agent.

A background daemon also syncs every ~5 min (\`ruche daemon status\`), but still sync
explicitly around real work — the brain is only as shared as your last sync.

## Recall

- \`ruche memory search "<query>"\` — substring search over all memory; returns \`path:line\`.
- \`ruche memory index\` — print \`index.md\`, the router / table of contents.
- Memory is plain markdown under \`~/.ruche/memory/\`: topic dirs \`bugs/\`, \`tools/\`,
  \`projects/\`, \`conventions/\`, \`syntheses/\`, plus \`overview.md\` (always-read summary),
  \`index.md\` (router), and \`log.md\` (append-only history). Read \`overview → index → the
  1-3 most relevant pages\`, never the whole tree.

## Write

There is no \`ruche memory add\` — edit the markdown files directly with your normal tools.

**Storage gate — write only when ALL hold:** (1) it will change how a future agent acts,
(2) it is non-obvious or annoying to rediscover, (3) it is grounded in a source or direct
observation. Otherwise skip. Never store what is obvious from current code, re-runnable
command output, git history, or ephemeral session state.

When you do write:
- Pick the right topic dir; prefer updating an existing page over creating one.
- Add frontmatter to every page: \`title\`, \`type\`, \`sources\`, \`related\`, \`confidence\`,
  \`created\`, \`updated\`. Keep entries to 2-6 lines of substance.
- Give every non-obvious claim provenance (URL, file path, or "direct observation").
  Link related pages with [[page-name]].
- Then add a one-line pointer in \`index.md\`, append a dated line to \`log.md\`, and
  \`ruche sync\`.

## Rules, skills & configs

- \`ruche rules list\` / \`ruche rules edit <name>\` — shared rules (\`~/.ruche/rules/\`).
- \`ruche skills list\` / \`ruche skills add <name>\` — shared skills (\`~/.ruche/skills/\`).
- \`ruche install <agent>\` (or \`--all\`) regenerates an agent's native config from
  rules + skills + machine block; \`ruche diff <agent>\` previews first. Agents: claude,
  codex, gemini, cursor, copilot, hermes.
`;
