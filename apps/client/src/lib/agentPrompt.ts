export const AGENT_PROMPT = `# Jardin — Shared Agent Memory

You are connected to **Jardin**: a memory, rules, and skills store synced across all my
machines and AI agents through the \`jardin\` CLI. Treat \`~/.jardin/memory/\` as your
persistent, cross-session, cross-machine brain. Use it silently — it is infrastructure,
not conversation.

## Operating loop (every non-trivial task)

1. **Sync & recall.** Run \`jardin sync\` to pull the latest brain, then
   \`jardin memory search "<keywords>"\` (skim \`jardin memory index\`) to reuse what is
   already known instead of rediscovering it.
2. **Do the work.**
3. **Write back.** If you learned something durable and non-obvious, save it (see below),
   then \`jardin sync\` to share it with every other machine and agent.

A background daemon also syncs every ~5 min (\`jardin daemon status\`), but still sync
explicitly around real work — the brain is only as shared as your last sync.

If \`jardin sync\` fails (offline, sandboxed shell, server unreachable), do NOT skip the
rest of the loop: memory is local-first. Keep reading, searching, and writing
\`~/.jardin/memory/\` — the daemon reconciles once the network is back.

## Recall

- \`jardin memory search "<query>"\` — substring search over all memory; returns \`path:line\`.
- \`jardin memory index\` — print \`index.md\`, the router / table of contents.
- Memory is plain markdown under \`~/.jardin/memory/\`: topic dirs \`bugs/\`, \`tools/\`,
  \`projects/\`, \`conventions/\`, \`syntheses/\`, plus \`overview.md\` (always-read summary),
  \`index.md\` (router), and \`log.md\` (append-only history). Read \`overview → index → the
  1-3 most relevant pages\`, never the whole tree.

## Write

There is no \`jardin memory add\` — edit the markdown files directly with your normal tools.

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
  \`jardin sync\`.

## Rules, skills & configs

- \`jardin rules list\` / \`jardin rules edit <name>\` — shared rules (\`~/.jardin/rules/\`).
- \`jardin skills list\` / \`jardin skills add <name>\` — shared skills (\`~/.jardin/skills/\`).
- \`jardin install <agent>\` (or \`--all\`) regenerates an agent's native config from
  rules + skills + machine block; \`jardin diff <agent>\` previews first. Agents: agents,
  claude, codex, gemini, cursor, copilot, hermes, opencode.
`;
