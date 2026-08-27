export const AGENT_PROMPT = `# Mycelium — Shared Agent Memory

You are connected to **Mycelium**: a memory, rules, and skills store synced across all my
machines and AI agents through the \`mycelium\` CLI. Treat \`~/.mycelium/memory/\` as your
persistent, cross-session, cross-machine brain. Use it silently — it is infrastructure,
not conversation.

## Operating loop (every non-trivial task)

1. **Sync & recall.** Run \`mycelium sync\` to pull the latest brain, then
   \`mycelium memory search "<keywords>"\` (skim \`mycelium memory index\`) to reuse what is
   already known instead of rediscovering it.
2. **Do the work.**
3. **Write back.** If you learned something durable and non-obvious, save it (see below),
   then \`mycelium sync\` to share it with every other machine and agent.

A background daemon also syncs every ~5 min (\`mycelium daemon status\`), but still sync
explicitly around real work — the brain is only as shared as your last sync.

If \`mycelium sync\` fails (offline, sandboxed shell, server unreachable), do NOT skip the
rest of the loop: memory is local-first. Keep reading, searching, and writing
\`~/.mycelium/memory/\` — the daemon reconciles once the network is back.

## Recall

- \`mycelium memory search "<query>"\` — substring search over all memory; returns \`path:line\`.
- \`mycelium memory index\` — print \`index.md\`, the router / table of contents.
- Memory is plain markdown under \`~/.mycelium/memory/\`: topic dirs \`bugs/\`, \`tools/\`,
  \`projects/\`, \`conventions/\`, \`standards/\`, \`syntheses/\`, \`people/\`, plus \`overview.md\` (always-read summary),
  \`index.md\` (router), and \`log.md\` (append-only history). Read \`overview → index → the
  1-3 most relevant pages\`, never the whole tree.

## Write

There is no \`mycelium memory add\` — edit the markdown files directly with your normal tools.

**Storage gate — write only when ALL hold:** (1) it will change how a future agent acts,
(2) it is non-obvious or annoying to rediscover, (3) it is grounded in a source or direct
observation, (4) it carries no secret. Credentials, tokens, API keys, passwords and private
keys are refused whatever the first three say: the wiki syncs to every machine and every
agent, and a page is retrieved into a context window by design. Otherwise skip. Never store
what is obvious from current code, re-runnable command output, git history, or ephemeral
session state.

When you do write:
- Pick the right topic dir; prefer updating an existing page over creating one.
- Add frontmatter to every page: \`title\`, \`type\`, \`sources\`, \`related\`, \`confidence\`,
  \`created\`, \`updated\`. Keep entries to 2-6 lines of substance.
- Give every non-obvious claim provenance (URL, file path, or "direct observation").
  Link related pages with [[page-name]].
- When you re-check an existing finding and it still holds, stamp it: put
  \`<!-- confirmed: YYYY-MM-DD -->\` under that finding's \`###\` heading, nothing else in
  the comment. Ranking reads it as the date the claim was last known good. A new finding
  never carries it.
- \`type: standard\` (pages under \`standards/\`) marks a normative page: it says a repo
  that disagrees is wrong. Propose changes to those; do not lint them the way you lint a
  dated observation.
- Then add a one-line pointer in \`index.md\`, append a dated line to \`log.md\`, and
  \`mycelium sync\`.

## Rules, skills & configs

- \`mycelium rules list\` / \`mycelium rules edit <name>\` — shared rules (\`~/.mycelium/rules/\`).
- \`mycelium skills list\` / \`mycelium skills add <name>\` — shared skills (\`~/.mycelium/skills/\`).
- \`mycelium install <agent>\` (or \`--all\`) regenerates an agent's native config from
  rules + skills + machine block; \`mycelium diff <agent>\` previews first. Agents: agents,
  claude, codex, gemini, cursor, copilot, hermes, opencode.
`;
