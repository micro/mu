# Skills

[Agent Skills](https://www.anthropic.com/engineering/equipping-agents-for-the-real-world-with-agent-skills)
are an open standard: a directory with a `SKILL.md` whose frontmatter is loaded
into an agent's system prompt at startup, whose body is read when the agent
decides it is relevant, and whose linked files are read on demand. Claude Code,
claude.ai and the Agent SDK all read them.

## Why Mu ships one

An agent connecting to `/mcp` gets tool definitions and nothing else. Definitions
say what each tool does; they cannot say how the tools compose, which ones need
an account, which cost money, or that the caller can have its own email address
and a store that outlives the session.

That layer has to live somewhere. Mu declares only the `tools` MCP capability —
no resources, no prompts — so a skill is how it travels, and it travels further
than a protocol extension would: to any client reading the standard, without Mu
implementing anything protocol-side.

Mu is also the only party that can write it honestly. Mu runs the services these
tools are derived from.

## What is here

- `mu/` — using a Mu instance over MCP: connecting, what needs an account, what
  costs, and the combinations that are not obvious from the tool list

## Using it

Copy the skill directory into wherever your agent reads skills from:

```bash
# Claude Code, for one project
mkdir -p .claude/skills && cp -r skills/mu .claude/skills/

# Claude Code, everywhere
mkdir -p ~/.claude/skills && cp -r skills/mu ~/.claude/skills/
```

Other clients differ; the directory is the portable part.

## Keeping it true

A skill that is wrong is worse than no skill, because it is read as authoritative
and it is read before anything is tried.

Two rules hold the rest together:

**Never copy anything that varies per instance.** Prices are instance settings
(`CREDIT_COST_*`), and a self-hosted Mu with payments unconfigured charges
nothing at all. The registered tools depend on which services an instance runs.
So the skill teaches `wallet_check` and `tools/list` rather than reproducing
their answers.

**State the shape, not the inventory.** Tool names are `service_method` and
methods returning a set are always `List`. An agent that knows the rule can
guess a tool it has never seen; a list of names goes stale the moment a service
is added.

Mu itself does not run bundled skill scripts. It has no shell, and apps execute
in the browser sandbox rather than on the server — so the skill covers the
instruction layers and points at `apps_create` where code is wanted. That is a
deliberate seam, not a gap to be filled later.
