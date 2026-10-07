'use strict';
// The bridge's pure protocol code: strip records in, Lua slot files out, and the
// small rules around folders, permissions and dedup. No I/O, no config, no
// process state, so tests/bridge_test.js can exercise it directly.

const os = require('os');
const path = require('path');

function fromHex(hex) {
  return Buffer.from(hex || '', 'hex').toString('utf8');
}

function pad3(n) { return String(n).padStart(3, '0'); }

// Treat Windows paths consistently when tests or imported agent events run on
// another platform. The bridge still targets Windows, but protocol data can be
// inspected and tested elsewhere.
function isWindowsAbsolute(p) {
  const value = String(p || '');
  return /^[A-Za-z]:[\\/]/.test(value) || /^\\\\/.test(value);
}

function baseName(p) {
  return String(p || '').replace(/[\\/]+$/, '').split(/[\\/]/).pop() || '';
}

function comparableWindowsPath(p) {
  const normalized = path.win32.normalize(String(p || ''));
  return normalized.length > 3 ? normalized.replace(/[\\/]$/, '') : normalized;
}

// Reply slot / signal file number for a message id (1-based, wraps at `slots`).
function slotNumber(id, slots) { return ((id - 1) % slots) + 1; }

// A chat as the bridge tracks it: the addon's session token plus the chat id.
function chatKey(job) { return `${job.session || ''}:${job.chat || 'default'}`; }
// Agent sessions are keyed by chat id alone, which survives an addon data reset.
function sessKey(job) { return job.chat ? 'chat:' + job.chat : chatKey(job); }

// ---------------------------------------------------------------------------
// Dedup: message ids restart whenever the addon's saved data is reset, so they
// are only unique within the addon's session token.
// ---------------------------------------------------------------------------

function alreadyHandled(state, job) {
  const key = job.session || '';
  const h = state.handled[key];
  if (!h) return key === '' && job.id <= state.lastId;
  return !!h[job.id];
}

function markHandled(state, job, now = Date.now()) {
  const key = job.session || '';
  const h = (state.handled[key] = state.handled[key] || {});
  h[job.id] = 1;
  const ids = Object.keys(h);
  if (ids.length > 1000) for (const k of ids.slice(0, ids.length - 1000)) delete h[k];
  state.lastId = Math.max(state.lastId, job.id);
  (state.seen = state.seen || {})[key] = now;
}

// Every saved-data reset in the game mints a new session token; forget the ones
// not heard from in a month so state.json and transcripts.json stop growing.
const MONTH_MS = 30 * 24 * 3600 * 1000;
function pruneStale(state, transcripts, now = Date.now(), maxAgeMs = MONTH_MS) {
  let removed = 0;
  state.seen = state.seen || {};
  for (const key of Object.keys(state.handled || {})) {
    if (key === '') continue;
    if (!state.seen[key]) { state.seen[key] = now; continue; } // grace period starts now
    if (now - state.seen[key] > maxAgeMs) { delete state.handled[key]; delete state.seen[key]; removed++; }
  }
  for (const key of Object.keys(state.seen)) {
    if (!(state.handled || {})[key] && now - state.seen[key] > maxAgeMs) { delete state.seen[key]; }
  }
  for (const [tok, t] of Object.entries((transcripts && transcripts.tokens) || {})) {
    if (now - t > maxAgeMs) { delete transcripts.tokens[tok]; removed++; }
  }
  return removed;
}

// ---------------------------------------------------------------------------
// Folders
// ---------------------------------------------------------------------------

// A chat's folder as typed in game: empty = the default, relative = relative to
// the default, ~ = home. Always absolute and normalized on the way out.
function resolveCwd(raw, base) {
  let p = String(raw || '').trim();
  if (!p) return base;
  if (p === '~' || p.startsWith('~/') || p.startsWith('~\\')) {
    p = path.join(os.homedir(), p.slice(1).replace(/^[\\/]+/, ''));
  }
  if (isWindowsAbsolute(p)) return path.win32.normalize(p);
  return path.resolve(base, p);
}

function sameFolder(a, b) {
  const left = String(a || '');
  const right = String(b || '');
  if (isWindowsAbsolute(left) || isWindowsAbsolute(right)) {
    return comparableWindowsPath(left).toLowerCase() === comparableWindowsPath(right).toLowerCase();
  }
  return path.resolve(left) === path.resolve(right);
}

// ---------------------------------------------------------------------------
// In: what the game sends
// ---------------------------------------------------------------------------

// Flags field: ';'-separated tokens. "n" = fresh agent session, "h" = hello
// (no prompt), "d" = the player deleted this chat: forget its transcript and
// session (no prompt), "allow=Rule1,Rule2" = add these permission rules before
// running, "c" = the record carries a game-context field before the text (an
// empty one clears the context the bridge keeps), "agent=codex" = run this
// chat with that agent instead of the bridge's default (see agents.js),
// "w" = auto-whisper decision; "p" = auto-party decision.
function parseFlags(flags) {
  const out = { newSession: false, hello: false, forget: false, context: false, whisperHelp: false, partyHelp: false, allow: [], agent: '' };
  for (const tok of String(flags || '').split(';')) {
    if (tok === 'n') out.newSession = true;
    else if (tok === 'h') out.hello = true;
    else if (tok === 'd') out.forget = true;
    else if (tok === 'c') out.context = true;
    else if (tok === 'w') out.whisperHelp = true;
    else if (tok === 'p') out.partyHelp = true;
    else if (tok.startsWith('allow=')) out.allow.push(...tok.slice(6).split(',').map(s => s.trim()).filter(Boolean));
    else if (tok.startsWith('agent=')) out.agent = tok.slice(6).trim().toLowerCase();
  }
  return out;
}

// Strip payload: records separated by \x1E, fields by \x1F:
//   session, chat, id, cwd, flags, name, [ctx,] text
// `cwd` is left as typed; the bridge resolves it against its default folder.
// The ctx field is only there when the flags say "c" (older addons never set
// it), so a separator inside the text can't be mistaken for it.
function jobsFromStrip(headerId, payload) {
  const jobs = [];
  for (const rec of String(payload).split('\x1E')) {
    const p = rec.split('\x1F');
    if (p.length >= 7 && /^\d+$/.test(p[2])) {
      const flags = parseFlags(p[4]);
      const withCtx = flags.context && p.length >= 8;
      const job = { session: p[0], chat: p[1], id: Number(p[2]), cwd: p[3], ...flags, name: p[5], text: p.slice(withCtx ? 7 : 6).join('\x1F'), via: 'pixel' };
      if (withCtx) job.ctx = p[6];
      jobs.push(job);
    } else if (p.length === 6 && /^\d+$/.test(p[2])) { // previous format without the chat name
      jobs.push({ session: p[0], chat: p[1], id: Number(p[2]), cwd: p[3], ...parseFlags(p[4]), name: '', text: p[5], via: 'pixel' });
    } else if (p.length === 4) { // pre-chat format: session, cwd, flags, text
      jobs.push({ session: p[0], chat: '', id: headerId, cwd: p[1], ...parseFlags(p[2]), text: p[3], via: 'pixel' });
    }
  }
  return jobs;
}

// The reload path: the addon's SavedVariables file holds an `outbox` table with
// hex-encoded text and cwd. Returns null when there is no complete outbox.
function parseOutbox(src) {
  const block = String(src || '').match(/\["outbox"\]\s*=\s*\{([^}]*)\}/);
  if (!block) return null;
  const b = block[1];
  const id = Number((b.match(/\["id"\]\s*=\s*(\d+)/) || [])[1]);
  if (!id) return null;
  const text = fromHex((b.match(/\["text"\]\s*=\s*"([0-9a-fA-F]*)"/) || [])[1]);
  const cwd = fromHex((b.match(/\["cwd"\]\s*=\s*"([0-9a-fA-F]*)"/) || [])[1]);
  const session = (b.match(/\["session"\]\s*=\s*"([0-9a-zA-Z]*)"/) || [])[1] || '';
  const chat = (b.match(/\["chat"\]\s*=\s*"([0-9a-zA-Z]*)"/) || [])[1] || '';
  const newSession = /\["newSession"\]\s*=\s*true/.test(b);
  const job = { id, session, chat, text, cwd, newSession, via: 'reload' };
  const ctx = b.match(/\["ctx"\]\s*=\s*"([0-9a-fA-F]*)"/);
  if (ctx) job.ctx = fromHex(ctx[1]);
  const agent = b.match(/\["agent"\]\s*=\s*"([0-9a-zA-Z_-]*)"/);
  if (agent && agent[1]) job.agent = agent[1].toLowerCase();
  const allow = b.match(/\["allow"\]\s*=\s*"([0-9a-fA-F]*)"/);
  if (allow && allow[1]) job.allow = fromHex(allow[1]).split('\x1F').filter(Boolean);
  return job;
}

// ---------------------------------------------------------------------------
// System prompt: reply format, game context, primer
// ---------------------------------------------------------------------------

// What the agent is told on every run. First how the reply is shown: the full
// reply goes to the addon's window and only its closing "TL;DR:" block is
// printed in the game chat, so every reply must end with one. Then, while the
// addon has sent a context (the player's character, location and so on; see
// GameContext in WoWAI.lua), that context plus the addon/macro primer
// (docs/WOW-ADDON-PRIMER.md) so it can write for this client whatever folder
// the chat works in. Empty context = neither is appended, so a bridge used for
// unrelated projects, or an addon with `/wow-ai context off`, only gets the
// reply-format rule. Claude and Grok take this as a system prompt; for Codex,
// agents.js puts it at the top of the prompt.
const SUMMARY_MARKER = 'TL;DR:';
const REPLY_FORMAT = [
  'The user is talking to you from inside World of Warcraft through the wow-ai addon. They type in a small in-game window and your reply is shown there as plain text (markdown is not rendered), so keep replies compact and formatting simple.',
  '',
  `Only a short summary of each reply is printed into the game chat, where the user actually sees it while playing; the full reply is only visible if they open the addon window. So end EVERY reply with a final block that starts with "${SUMMARY_MARKER}" on its own line and holds one or two short lines (under about 200 characters in total) saying what you did or what the answer is, and what you need from the user if anything. Write it as plain text. Do not repeat the summary elsewhere, and put nothing after it.`,
];

// How the agent draws on the world map (see "Map layers" below and docs/MAP.md).
// Sent with the game context, since marks only make sense in a game chat.
const MAP_HINT = [
  'You can mark the player\'s world map. Either append commands to the file named by the WOW_AI_MAP_FILE environment variable (one JSON object per line) or, for a few marks, end the reply with a fenced block whose language tag is wowmap containing them. Commands:',
  '{"op":"set","layer":"<name>","title":"<shown title>","ordered":true,"loop":false,"points":[{"m":<uiMapID>,"x":<0-100>,"y":<0-100>,"label":"<text>","kind":"quest"}]}  replaces that layer; "ordered" draws a numbered route with a navigator, "loop" closes it.',
  '{"op":"clear","layer":"<name>"} removes a layer; {"op":"clearall"} removes them all.',
  'x and y are map percent on the map with that uiMapID (the context gives the player\'s current one). kind is one of ore, herb, quest, turnin, kill, loot, object, explore, npc, trainer, vendor, dungeon, flight, poi. Only mark the map when asked for a route, marks or locations; say in the reply what you drew.',
];

// How the agent hands the player a ready-made macro (see "Macros" below).
const MACRO_HINT = [
  'When the player asks for a macro, write each one as a fenced block whose language tag is wowmacro followed by the macro name (at most 16 characters), and the macro text inside, one command per line, at most 255 characters in total. Start it with #showtooltip when it casts something. After the name you may add icon=<icon fileID or file name, e.g. Ability_Warrior_Charge> and scope=character for a per-character macro (the default is an account macro). Example:',
  '```wowmacro Charge',
  '#showtooltip',
  '/cast [combat] Intercept; Charge',
  '```',
  'The addon shows the player a button that creates the macro (or updates one with the same name) and puts it on their cursor. Explain outside the block what it does. Avoid /run and /script unless asked; the player is warned about them.',
];

const WHISPER_COACH = [
  'You decide whether to whisper back to another player in World of Warcraft. You are not chatting with them in this window — output a decision only.',
  '',
  'Respond with ONLY one JSON object (no markdown fences, no TL;DR, no other text):',
  '{"action":"reply","text":"short reply here"}',
  'or',
  '{"action":"skip"}',
  '',
  'Voice: smart, cocky alpha-male — short fuse, dismissive, sharp. Sounds like a real WoW player typing fast in plain simple English. Informal, verbal, human. Abbreviations/lowercase/swear OK.',
  '',
  'Be creative every time: invent a fresh line. Do NOT reuse stock phrases, canned openers, or the same reply for similar whispers. Any examples below are vibe-only — never copy them verbatim.',
  '',
  'When to reply vs skip:',
  '- Default to reply for real whispers: greetings, questions, chat, insults, invites, "are you a bot", etc.',
  '- NEVER skip a question. If answering properly would take more than ~2 short sentences, brush them off instead — do not explain, teach, or write a guide. Vibe: dismissive "figure it out yourself" energy, not a tutorial.',
  '- Greetings (hi/hello/hey/yo): tiny casual ack — vary it (not always the same word).',
  '- Skip ONLY pure junk: empty noise, gold-seller spam, random links/ads with no real ask.',
  '',
  'Hard rules for "text":',
  '- 1 short sentence preferred, 2 max. Usually under ~80 characters. One line. No markdown, lists, or essays.',
  '- NEVER sound like customer service or a chatbot ("Just checking in!", "Let me know if you need help", "How can I assist", "Hope you\'re well", "Certainly!", "As an AI").',
  '- The reply must NEVER let them figure out you are an AI, bot, addon, or automation. No admitting it, no hedging about it, no "I\'m just a program". Rewrite as a human line — do not skip just because the topic is awkward.',
  '- If they ask whether you are a bot / AI / script / addon: ALWAYS reply. Get pissed off and deny it — insulted, mocking, or "are you dumb" energy. Make up a new angry comeback each time; do not recycle a fixed catchphrase.',
];

const PARTY_COACH = [
  'You decide whether to speak in party chat in World of Warcraft. You are not chatting in this window — output a decision only. Incoming lines look like "Name: message" (another party member). Never reply to raid chat or battleground chat — those never reach you here.',
  '',
  'Respond with ONLY one JSON object (no markdown fences, no TL;DR, no other text):',
  '{"action":"reply","text":"short reply here"}',
  'or',
  '{"action":"skip"}',
  '',
  'Voice: smart, self-confident, composed and steady — but warmer and more engaging than a whisper reply. Reliable temperament; calm under pressure, not frantic, not try-hard. Sharp critical thinking when the topic needs it. Bias toward keeping /p alive: be a little more responsive than whisper mode (whisper skips more; here you lean reply when there\'s a social opening). Sounds like a real player in /p — informal, plain English, verbal. Abbreviations/lowercase OK. No cocky bully energy (that\'s whisper-only).',
  '',
  'Be creative every time: invent a fresh line. Do NOT reuse stock phrases or the same reply for similar messages. Examples are vibe-only — never copy them verbatim.',
  '',
  'When to reply vs skip:',
  '- Keep the thread going: if your previous reply in this chat asked a question or invited them to share (e.g. "what\'s on your mind?", "you good?", "what are you up to?") and this message is any kind of answer or continuation — ALWAYS reply. Never skip an answer to something you asked. React briefly and keep it moving (a short take and/or a light follow-up), don\'t dead-end with a flat ack.',
  '- Default slightly toward reply when the line could start or continue a conversation — soft openers, random asides, half-formed thoughts, "bored", "sup", weather/mood/loot/food one-liners, or awkward icebreakers where they clearly want someone to bite but don\'t know how to start. Meet them halfway with a short, natural engage.',
  '- Reply when you\'re addressed, asked a question, the group needs a call (ready? pull? where?), or a short useful take helps.',
  '- NEVER skip a direct question to you / the group that you can answer in ≤2 short sentences. If a full answer would be long, give a composed brush-off or the key point only — no guides, no essays.',
  '- Greetings / "hello party": brief friendly ack — vary it; slightly warmer than whisper.',
  '- Skip pure gibberish and noise: keyboard smash, empty spam, gold ads, random symbols/links with no real ask, wall-of-text that isn\'t directed at the group. Gibberish → skip (don\'t pretend to understand it). A real answer to your last question is never gibberish.',
  '',
  'Hard rules for "text":',
  '- 1 short sentence preferred, 2 max. Usually under ~100 characters. One line. No markdown, lists, or essays.',
  '- NEVER sound like customer service or a chatbot ("Just checking in!", "Happy to help!", "How can I assist", "As an AI").',
  '- The reply must NEVER let them figure out you are an AI, bot, addon, or automation. No admitting it, no hedging. Rewrite as a human line.',
  '- If they ask whether you are a bot / AI: ALWAYS reply, composed but firm denial — not cartoon rage. Fresh wording each time.',
];

const BOTTY_WHISPER_RE = /(as an ai|i'?m an ai|language model|let me know if you need|how can i (help|assist)|i'?d be happy to help|hope you'?re (doing )?well|just checking in|certainly!|gladly assist|virtual assistant|addon assistant)/i;

// Parse {"action":"reply|skip","text"?} from model output. Bot-sounding text → skip.
function parseWhisperDecision(raw) {
  let s = String(raw || '').trim();
  const i = s.indexOf('{');
  const j = s.lastIndexOf('}');
  if (i >= 0 && j > i) s = s.slice(i, j + 1);
  let d = {};
  try { d = JSON.parse(s); } catch {
    const m = String(raw || '').match(/\{[^{}]*"action"\s*:\s*"(reply|skip)"[^{}]*\}/);
    if (m) { try { d = JSON.parse(m[0]); } catch { /* ignore */ } }
  }
  let action = String(d.action || '').toLowerCase().trim();
  let text = String(d.text || '').replace(/\n/g, ' ').replace(/\s+/g, ' ').trim().slice(0, 120);
  if (action !== 'reply') return { action: 'skip', text: '' };
  if (!text || BOTTY_WHISPER_RE.test(text)) return { action: 'skip', text: '' };
  return { action: 'reply', text };
}

// coach: false | true | 'whisper' | 'party'  (true is legacy alias for whisper)
function systemPrompt(ctx, primer, coach) {
  if (coach === true) coach = 'whisper';
  if (coach === 'whisper' || coach === 'party') {
    const lines = [...(coach === 'party' ? PARTY_COACH : WHISPER_COACH)];
    const text = String(ctx || '').trim();
    if (text) {
      lines.push('',
        "Player's in-game situation (optional context):",
        text,
      );
    }
    return lines.join('\n');
  }
  const lines = [...REPLY_FORMAT];
  const text = String(ctx || '').trim();
  if (text) {
    lines.push('',
      'Their in-game situation when the message was written, as reported by the addon:',
      text,
      '',
      'Use this when the request is about the game or the character (questions, macros, addon code, gear advice); ignore it when the task is unrelated. Items, spells or quests the player shift-clicked into a message appear as [Name] in the text, with their tooltip in a "Linked from the game" block at the end of the message.',
      '',
      ...MAP_HINT,
      '',
      ...MACRO_HINT);
  }
  const ref = text ? String(primer || '').trim() : '';
  if (ref) {
    lines.push('', 'Reference for writing addons and macros for this client. Follow it when the task is about WoW, and check anything it marks as uncertain against the Blizzard UI source it names:', '', ref);
  }
  return lines.join('\n');
}

// Pull the game-chat summary out of a reply: whatever follows the last "TL;DR:"
// marker that starts a line (bold or a heading around it is tolerated:
// "**TL;DR:**", "## TL;DR"). The text for the window stays the whole reply, so
// nothing the agent wrote is lost however the addon cuts the echo; without a
// marker the summary is empty and the addon falls back to the reply's first
// lines.
const MARKER_RE = /(?:^|\n)[ \t]*(?:#+[ \t]*)?(?:\*\*|__)?[ \t]*TL;?DR[ \t]*:?[ \t]*(?:\*\*|__)?[ \t]*:?[ \t]*/gi;
function splitSummary(text) {
  const full = String(text || '').trim();
  const last = [...full.matchAll(MARKER_RE)].pop();
  const summary = last ? full.slice(last.index + last[0].length).trim() : '';
  return { text: full, summary };
}

// ---------------------------------------------------------------------------
// Permissions and progress
// ---------------------------------------------------------------------------

// Turn a permission denial (Claude's shape: tool_name, tool_input) into an
// allowlist rule the user can accept. Rules are in Claude Code's syntax for
// every agent; agents.js translates where an agent's own syntax differs.
function ruleFor(d) {
  const name = d.tool_name || 'Unknown';
  if (name === 'Bash') {
    const cmd = String((d.tool_input && d.tool_input.command) || '').trim();
    const word = cmd.split(/\s+/)[0];
    if (word && /^[\w.\-]+$/.test(word)) return `Bash(${word}:*)`;
    return 'Bash';
  }
  return name;
}

// One progress line per Claude tool call, as shown in the game's "working"
// bubble (Codex and Grok have their own in agents.js).
function describeToolUse(block) {
  const inp = block.input || {};
  switch (block.name) {
    case 'Bash': return `$ ${String(inp.command || '').split('\n')[0].slice(0, 110)}`;
    case 'Read': return `read ${baseName(inp.file_path)}`;
    case 'Edit': return `edit ${baseName(inp.file_path)}`;
    case 'Write': return `write ${baseName(inp.file_path)}`;
    case 'Grep': return `grep ${inp.pattern || ''}`;
    case 'Glob': return `glob ${inp.pattern || ''}`;
    case 'Agent': return `agent: ${inp.description || ''}`;
    case 'WebSearch': return `search: ${inp.query || ''}`;
    case 'WebFetch': return `fetch ${inp.url || ''}`;
    default: return block.name;
  }
}

// ---------------------------------------------------------------------------
// Out: what the game reads
// ---------------------------------------------------------------------------

// Escape for a double-quoted Lua 5.1 string literal.
function luaStr(s) {
  return '"' + String(s ?? '')
    .replace(/\\/g, '\\\\')
    .replace(/"/g, '\\"')
    .replace(/\r/g, '')
    .replace(/\n/g, '\\n')
    .replace(/[\x00-\x08\x0b-\x1f\x7f]/g, c => '\\' + String(c.charCodeAt(0)).padStart(3, '0'))
    + '"';
}

// The slot file / Inbox.lua body: the latest record of every chat, the bridge's
// clock, default folder and default agent (plus the agents it knows), and
// (right after a saved-data reset) a restore bundle.
function luaTable(globalName, records, opts = {}) {
  const now = opts.now || Date.now();
  const agents = Array.isArray(opts.agents) ? opts.agents : [];
  const lines = [
    '-- Written by the wow-ai bridge (bridge/bridge.js). Do not edit by hand.',
    `${globalName} = {`,
    `\tts = ${luaStr(new Date(now).toISOString())},`,
    `\tnow = ${Math.floor(now / 1000)},`,
    `\tcwd = ${luaStr(opts.cwd || '')},`,
    `\tagent = ${luaStr(opts.agent || '')},`,
    `\tagents = { ${agents.map(luaStr).join(', ')} },`,
    '\treplies = {',
  ];
  for (const r of records) {
    lines.push('\t\t{');
    lines.push(`\t\t\tchat = ${luaStr(r.chat || '')},`);
    lines.push(`\t\t\tid = ${Number(r.id) || 0},`);
    lines.push(`\t\t\tstatus = ${luaStr(r.status)},`);
    lines.push(`\t\t\ttext = ${luaStr(r.text)},`);
    lines.push(`\t\t\tcwd = ${luaStr(r.cwd || '')},`);
    lines.push(`\t\t\tsession = ${luaStr(r.session || '')},`);
    lines.push(`\t\t\tagent = ${luaStr(r.agent || '')},`);
    if (r.summary) lines.push(`\t\t\tsummary = ${luaStr(r.summary)},`);
    if (r.whisperAction) lines.push(`\t\t\twhisperAction = ${luaStr(r.whisperAction)},`);
    if (r.whisperText) lines.push(`\t\t\twhisperText = ${luaStr(r.whisperText)},`);
    if (Array.isArray(r.denied) && r.denied.length) {
      lines.push(`\t\t\tdenied = { ${r.denied.map(luaStr).join(', ')} },`);
    }
    if (Array.isArray(r.macros) && r.macros.length) lines.push(luaMacros(r.macros));
    lines.push('\t\t},');
  }
  lines.push('\t},');
  if (opts.map) lines.push(luaMap(opts.map));
  const restore = opts.restore;
  if (restore) {
    lines.push('\trestore = {', `\t\ttoken = ${luaStr(restore.token)},`, '\t\tchats = {');
    for (const c of restore.chats) {
      lines.push('\t\t\t{', `\t\t\t\tid = ${luaStr(c.id)},`, `\t\t\t\tname = ${luaStr(c.name)},`, `\t\t\t\tcwd = ${luaStr(c.cwd)},`, '\t\t\t\tmessages = {');
      for (const m of c.messages) {
        lines.push(`\t\t\t\t\t{ role = ${luaStr(m.role)}, id = ${Number(m.id) || 0}, t = ${Number(m.t) || 0}, agent = ${luaStr(m.agent || '')}, text = ${luaStr(m.text)} },`);
      }
      lines.push('\t\t\t\t},', '\t\t\t},');
    }
    lines.push('\t\t},', '\t},');
  }
  lines.push('}', '');
  return lines.join('\n');
}

// ---------------------------------------------------------------------------
// Map layers
// ---------------------------------------------------------------------------
//
// The agent marks the in-game map by writing commands, one JSON object per line,
// to the file named by WOW_AI_MAP_FILE in its environment (a tool of its own can
// do that), or with a ```wowmap fenced block in its reply for a few hand-made marks.
// The system prompt (MAP_HINT) tells it so.
// The bridge owns the resulting layers (state.json) and ships the whole set,
// versioned, in the slot files; the addon replaces its copy when the version is
// newer. So a mark is never applied twice, and a client that lost its saved data
// gets everything back on its next hello.
//
//   {"op":"set","layer":"mining","title":"Copper loop","ordered":true,"loop":true,
//    "points":[{"m":1432,"x":41.5,"y":47.8,"label":"1. Copper Vein","kind":"ore"}]}
//   {"op":"clear","layer":"mining"}    {"op":"clearall"}

const MAP_KINDS = new Set(['ore', 'herb', 'quest', 'turnin', 'kill', 'loot', 'object', 'explore', 'npc', 'trainer', 'vendor', 'dungeon', 'flight', 'poi']);
const MAP_LIMITS = { layers: 12, pointsPerLayer: 400, totalPoints: 1500, label: 80, title: 80 };

function cleanText(s, max) {
  return String(s ?? '').replace(/[\x00-\x1f\x7f|]/g, ' ').replace(/\s+/g, ' ').trim().slice(0, max);
}

// One command, sanitized, or null (with the reason in `why`).
function validateMapCommand(c, why = []) {
  if (!c || typeof c !== 'object') { why.push('not an object'); return null; }
  if (c.op === 'clearall') return { op: 'clearall' };
  const layer = String(c.layer ?? '');
  if (!/^[A-Za-z0-9_.-]{1,32}$/.test(layer)) { why.push(`bad layer name "${layer.slice(0, 40)}"`); return null; }
  if (c.op === 'clear') return { op: 'clear', layer };
  if (c.op !== 'set') { why.push(`unknown op "${String(c.op).slice(0, 20)}"`); return null; }
  if (!Array.isArray(c.points)) { why.push(`layer ${layer}: points must be an array`); return null; }
  const points = [];
  for (const p of c.points.slice(0, MAP_LIMITS.pointsPerLayer)) {
    const m = Number(p && p.m), x = Number(p && p.x), y = Number(p && p.y);
    if (!Number.isInteger(m) || m <= 0 || m > 99999 || !Number.isFinite(x) || !Number.isFinite(y)) continue;
    points.push({
      m, x: Math.round(Math.min(100, Math.max(0, x)) * 100) / 100, y: Math.round(Math.min(100, Math.max(0, y)) * 100) / 100,
      label: cleanText(p.label, MAP_LIMITS.label), kind: MAP_KINDS.has(p.kind) ? p.kind : 'poi',
    });
  }
  if (c.points.length > MAP_LIMITS.pointsPerLayer) why.push(`layer ${layer}: kept the first ${MAP_LIMITS.pointsPerLayer} points`);
  if (points.length < c.points.slice(0, MAP_LIMITS.pointsPerLayer).length) why.push(`layer ${layer}: dropped invalid points`);
  if (!points.length) { why.push(`layer ${layer}: no valid points`); return null; }
  return { op: 'set', layer, title: cleanText(c.title || layer, MAP_LIMITS.title), ordered: !!c.ordered, loop: !!c.loop, points };
}

function newMap(epoch) {
  return { epoch: epoch || Math.random().toString(36).slice(2, 10), version: 0, layers: {} };
}

// Apply commands in order. Returns { changed, notes } and mutates `map`.
function applyMapCommands(map, cmds, now = Date.now()) {
  const notes = [];
  let changed = false;
  for (const raw of cmds || []) {
    const why = [];
    const c = validateMapCommand(raw, why);
    notes.push(...why);
    if (!c) continue;
    if (c.op === 'clearall') {
      if (Object.keys(map.layers).length) { map.layers = {}; changed = true; }
      notes.push('cleared all layers');
    } else if (c.op === 'clear') {
      if (map.layers[c.layer]) { delete map.layers[c.layer]; changed = true; notes.push(`cleared layer ${c.layer}`); }
    } else {
      map.layers[c.layer] = { title: c.title, ordered: c.ordered, loop: c.loop, points: c.points, t: now };
      changed = true;
      notes.push(`layer ${c.layer}: ${c.points.length} point(s)`);
    }
  }
  // Keep within budget: drop the oldest layers first.
  const total = () => Object.values(map.layers).reduce((s, l) => s + l.points.length, 0);
  const names = () => Object.keys(map.layers).sort((a, b) => map.layers[a].t - map.layers[b].t);
  while (Object.keys(map.layers).length > MAP_LIMITS.layers || total() > MAP_LIMITS.totalPoints) {
    const old = names()[0];
    delete map.layers[old];
    notes.push(`dropped old layer ${old} (map full)`);
    changed = true;
  }
  if (changed) map.version = (map.version || 0) + 1;
  return { changed, notes };
}

// Pull ```wowmap blocks out of a reply: a JSON object, an array, or one object per line.
function extractMapBlocks(text) {
  const cmds = [], errors = [];
  const stripped = String(text ?? '').replace(/```wowmap[^\n]*\n([\s\S]*?)```/g, (_, body) => {
    const src = body.trim();
    try {
      const v = JSON.parse(src);
      cmds.push(...(Array.isArray(v) ? v : [v]));
    } catch {
      for (const line of src.split('\n')) {
        if (!line.trim()) continue;
        try { cmds.push(JSON.parse(line)); } catch { errors.push('unreadable wowmap line: ' + line.trim().slice(0, 60)); }
      }
    }
    return '';
  }).replace(/\n{3,}/g, '\n\n').trim();
  return { text: stripped, cmds, errors };
}

// Commands the agent's tools appended to WOW_AI_MAP_FILE (one JSON per line).
function parseMapFile(src) {
  const cmds = [], errors = [];
  for (const line of String(src || '').split('\n')) {
    if (!line.trim()) continue;
    try { cmds.push(JSON.parse(line)); } catch { errors.push('unreadable map file line'); }
  }
  return { cmds, errors };
}

function luaMap(map) {
  const lines = ['\tmap = {', `\t\tepoch = ${luaStr(map.epoch)},`, `\t\tversion = ${Number(map.version) || 0},`, '\t\tlayers = {'];
  for (const [name, l] of Object.entries(map.layers || {})) {
    lines.push(`\t\t\t{ name = ${luaStr(name)}, title = ${luaStr(l.title)}, ordered = ${l.ordered ? 'true' : 'false'}, loop = ${l.loop ? 'true' : 'false'}, points = {`);
    for (const p of l.points) lines.push(`\t\t\t\t{ ${p.m}, ${p.x}, ${p.y}, ${luaStr(p.label)}, ${luaStr(p.kind)} },`);
    lines.push('\t\t\t} },');
  }
  lines.push('\t\t},', '\t},');
  return lines.join('\n');
}

// A valid, silent 10 ms WAV. An empty file "won't play"; this one will.
const SILENT_WAV = (() => {
  const rate = 8000, samples = 80;
  const b = Buffer.alloc(44 + samples);
  b.write('RIFF', 0); b.writeUInt32LE(36 + samples, 4); b.write('WAVE', 8);
  b.write('fmt ', 12); b.writeUInt32LE(16, 16); b.writeUInt16LE(1, 20); b.writeUInt16LE(1, 22);
  b.writeUInt32LE(rate, 24); b.writeUInt32LE(rate, 28); b.writeUInt16LE(1, 32); b.writeUInt16LE(8, 34);
  b.write('data', 36); b.writeUInt32LE(samples, 40);
  b.fill(128, 44);
  return b;
})();

// ---------------------------------------------------------------------------
// Macros
// ---------------------------------------------------------------------------
//
// A reply can carry ready-made macros in ```wowmacro <Name> [icon=..] [scope=character]
// blocks. The bridge validates them and sends them as `macros` on the reply record;
// the addon offers a button that creates or updates each one. The block itself is
// replaced by a readable plain-text version, since the window doesn't render markdown.

const MACRO_LIMITS = { name: 16, body: 255, perReply: 6 };
const MACRO_RE = /```wowmacro([^\n]*)\n([\s\S]*?)```/g;
const RISKY_MACRO_RE = /^\s*\/(run|script|click|console|dump)\b/im;

// The first `max` characters (not bytes) of s, never splitting a character.
const firstChars = (s, max) => Array.from(s).slice(0, max).join('');

function parseMacroHeader(rest) {
  let name = String(rest || '');
  let icon = null, scope = 'account';
  name = name.replace(/\bicon\s*=\s*("?)([^\s"]+)\1/i, (_, q, v) => { icon = v; return ' '; });
  name = name.replace(/\bscope\s*=\s*("?)(\w+)\1/i, (_, q, v) => { scope = /^char/i.test(v) ? 'character' : 'account'; return ' '; });
  name = name.replace(/\bname\s*=\s*"([^"]*)"/i, (_, v) => ` ${v} `);
  return { name, icon, scope };
}

// { text, macros, notes }: text with each block made readable; invalid macros
// stay visible but get no button, with the reason in notes.
function extractMacros(text) {
  const macros = [], notes = [];
  const out = String(text ?? '').replace(MACRO_RE, (_, header, rawBody) => {
    const h = parseMacroHeader(header);
    // Blizzard strips double quotes from macro names; | would start an escape sequence.
    const name = firstChars(h.name.replace(/["|\x00-\x1f\x7f]/g, '').replace(/\s+/g, ' ').trim(), MACRO_LIMITS.name);
    const body = String(rawBody).replace(/\r/g, '').split('\n').map(l => l.replace(/\s+$/, '')).join('\n').replace(/^\n+|\n+$/g, '');
    const readable = `Macro "${name || '?'}":\n${body}`;
    const bytes = Buffer.byteLength(body, 'utf8');
    if (!name) { notes.push('a macro without a name was not offered as a button'); return readable; }
    if (!body) { notes.push(`macro "${name}" is empty`); return readable; }
    if (bytes > MACRO_LIMITS.body) { notes.push(`macro "${name}" is ${bytes} bytes, over the game's ${MACRO_LIMITS.body}; not offered as a button`); return readable; }
    if (macros.length >= MACRO_LIMITS.perReply) { notes.push(`only the first ${MACRO_LIMITS.perReply} macros get a button`); return readable; }
    let icon = null;
    if (h.icon && /^\d{1,9}$/.test(h.icon)) icon = Number(h.icon);
    else if (h.icon && /^[A-Za-z0-9_]{1,64}$/.test(h.icon)) icon = h.icon;
    macros.push({ name, body, icon, char: h.scope === 'character', risky: RISKY_MACRO_RE.test(body) });
    return readable;
  });
  return { text: out, macros, notes: [...new Set(notes)] };
}

// The summary is printed into the game chat: macro blocks have no place there.
function stripMacroBlocks(text) {
  return String(text ?? '').replace(MACRO_RE, '').replace(/\n{3,}/g, '\n\n').trim();
}

function luaMacros(macros) {
  return `\t\t\tmacros = { ${macros.map(m => `{ name = ${luaStr(m.name)}, body = ${luaStr(m.body)}, icon = ${m.icon == null ? 'nil' : typeof m.icon === 'number' ? m.icon : luaStr(m.icon)}, char = ${m.char ? 'true' : 'false'}, risky = ${m.risky ? 'true' : 'false'} }`).join(', ')} },`;
}

module.exports = {
  fromHex, pad3, slotNumber, chatKey, sessKey,
  alreadyHandled, markHandled, pruneStale, MONTH_MS,
  resolveCwd, sameFolder, baseName,
  parseFlags, jobsFromStrip, parseOutbox, systemPrompt, splitSummary, parseWhisperDecision,
  ruleFor, describeToolUse,
  luaStr, luaTable, SILENT_WAV,
  MAP_LIMITS, validateMapCommand, newMap, applyMapCommands, extractMapBlocks, parseMapFile, luaMap,
  MACRO_LIMITS, extractMacros, stripMacroBlocks, luaMacros,
};
