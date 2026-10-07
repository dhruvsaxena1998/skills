#!/usr/bin/env bun
/**
 * Symlink every skill in ./skills into each agent's skills directory.
 *
 *   bun run link              link all skills (global: ~/.agents, ~/.claude, ~/.pi)
 *   bun run link --dry-run    show what would change
 *   bun run link --force      replace existing symlinks that point elsewhere
 *   bun run link --project .  link into <dir>/.agents/skills, <dir>/.claude/skills, <dir>/.pi/skills
 *   bun run link --agents=claude,pi   only these agents (agents | claude | pi); default is all
 *   bun run link --skip-agents=pi     all agents except these
 *   bun run unlink            remove symlinks that point into this repo
 *   bun run status            show link state per skill per target
 *
 * A skill is any directory under ./skills containing a SKILL.md.
 * Stale links (pointing into ./skills at a skill that no longer exists) are pruned on link.
 */
import { existsSync, lstatSync, mkdirSync, readdirSync, readlinkSync, symlinkSync, unlinkSync } from "node:fs";
import { homedir } from "node:os";
import { join, resolve } from "node:path";
import { parseArgs } from "node:util";

const SKILLS_DIR = resolve(import.meta.dir, "..", "skills");

const { values, positionals } = parseArgs({
  args: Bun.argv.slice(2),
  options: {
    "dry-run": { type: "boolean", default: false },
    force: { type: "boolean", default: false },
    project: { type: "string" },
    agents: { type: "string" },
    agent: { type: "string" },
    "skip-agents": { type: "string" },
    "skip-agent": { type: "string" },
  },
  allowPositionals: true,
});

const command = positionals[0] ?? "link";
const dryRun = values["dry-run"];

const AGENTS = {
  agents: { project: ".agents/skills", global: ".agents/skills" },
  claude: { project: ".claude/skills", global: ".claude/skills" },
  // Pi reads global skills from ~/.pi/agent/skills (project-level is .pi/skills).
  pi: { project: ".pi/skills", global: ".pi/agent/skills" },
} as const;
type Agent = keyof typeof AGENTS;

function parseAgentList(raw: string): Agent[] {
  const names = raw.split(",").map((s) => s.trim()).filter(Boolean);
  const unknown = names.filter((n) => !(n in AGENTS));
  if (unknown.length > 0 || names.length === 0) {
    console.error(`Unknown agent(s): ${unknown.join(", ") || raw}. Valid: ${Object.keys(AGENTS).join(", ")}`);
    process.exit(1);
  }
  return [...new Set(names)] as Agent[];
}

function selectedAgents(): Agent[] {
  const only = values.agents ?? values.agent;
  const skip = values["skip-agents"] ?? values["skip-agent"];
  const included = only ? parseAgentList(only) : (Object.keys(AGENTS) as Agent[]);
  const skipped = skip ? parseAgentList(skip) : [];
  const result = included.filter((a) => !skipped.includes(a));
  if (result.length === 0) {
    console.error("No agents left to act on after applying --agents/--skip-agents.");
    process.exit(1);
  }
  return result;
}

function targetDirs(): string[] {
  const root = values.project ? resolve(values.project) : homedir();
  return selectedAgents().map((a) => join(root, values.project ? AGENTS[a].project : AGENTS[a].global));
}

function listSkills(): string[] {
  if (!existsSync(SKILLS_DIR)) return [];
  return readdirSync(SKILLS_DIR, { withFileTypes: true })
    .filter((d) => d.isDirectory() && existsSync(join(SKILLS_DIR, d.name, "SKILL.md")))
    .map((d) => d.name)
    .sort();
}

type LinkState = "missing" | "linked" | "foreign-link" | "not-a-link";

function stateOf(dest: string, source: string): LinkState {
  let stat;
  try {
    stat = lstatSync(dest);
  } catch {
    return "missing";
  }
  if (!stat.isSymbolicLink()) return "not-a-link";
  return resolve(join(dest, ".."), readlinkSync(dest)) === source ? "linked" : "foreign-link";
}

/** Symlinks in `dir` that point somewhere inside SKILLS_DIR. */
function ownedLinks(dir: string): string[] {
  if (!existsSync(dir)) return [];
  return readdirSync(dir).filter((name) => {
    const p = join(dir, name);
    try {
      if (!lstatSync(p).isSymbolicLink()) return false;
      const target = resolve(dir, readlinkSync(p));
      return target === SKILLS_DIR || target.startsWith(SKILLS_DIR + "/");
    } catch {
      return false;
    }
  });
}

const log = (tag: string, msg: string) => console.log(`${dryRun ? "[dry-run] " : ""}${tag.padEnd(8)} ${msg}`);

function link() {
  const skills = listSkills();
  if (skills.length === 0) console.log(`No skills found in ${SKILLS_DIR} (expected skills/<name>/SKILL.md).`);

  for (const dir of targetDirs()) {
    if (!existsSync(dir)) {
      log("mkdir", dir);
      if (!dryRun) mkdirSync(dir, { recursive: true });
    }

    for (const name of skills) {
      const source = join(SKILLS_DIR, name);
      const dest = join(dir, name);
      switch (stateOf(dest, source)) {
        case "linked":
          break;
        case "missing":
          log("link", `${dest} -> ${source}`);
          if (!dryRun) symlinkSync(source, dest, "dir");
          break;
        case "foreign-link":
          if (values.force) {
            log("relink", `${dest} -> ${source}`);
            if (!dryRun) {
              unlinkSync(dest);
              symlinkSync(source, dest, "dir");
            }
          } else {
            log("skip", `${dest} is a symlink to ${readlinkSync(dest)} (use --force to replace)`);
          }
          break;
        case "not-a-link":
          log("skip", `${dest} exists and is not a symlink; leaving it alone`);
          break;
      }
    }

    for (const name of ownedLinks(dir)) {
      if (!skills.includes(name)) {
        log("prune", join(dir, name));
        if (!dryRun) unlinkSync(join(dir, name));
      }
    }
  }
}

function unlink() {
  for (const dir of targetDirs()) {
    for (const name of ownedLinks(dir)) {
      log("unlink", join(dir, name));
      if (!dryRun) unlinkSync(join(dir, name));
    }
  }
}

function status() {
  const skills = listSkills();
  const dirs = targetDirs();
  console.log(`Skills: ${SKILLS_DIR}\n`);
  for (const dir of dirs) {
    console.log(dir);
    if (skills.length === 0) console.log("  (no skills)");
    for (const name of skills) {
      console.log(`  ${stateOf(join(dir, name), join(SKILLS_DIR, name)).padEnd(12)} ${name}`);
    }
    for (const name of ownedLinks(dir).filter((n) => !skills.includes(n))) {
      console.log(`  ${"stale".padEnd(12)} ${name}`);
    }
  }
}

switch (command) {
  case "link":
    link();
    break;
  case "unlink":
    unlink();
    break;
  case "status":
    status();
    break;
  default:
    console.error(`Unknown command "${command}". Use: link | unlink | status`);
    process.exit(1);
}
