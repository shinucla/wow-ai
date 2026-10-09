package ui

import "html/template"

var pageTmpl = template.Must(template.New("page").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>WoW AI Bridge</title>
<style>
  :root {
    --bg: #1a1d23;
    --panel: #242830;
    --border: #3a404c;
    --text: #e8eaed;
    --muted: #9aa0a6;
    --accent: #5b9fd4;
    --ok: #3d9a6a;
    --danger: #c45c5c;
    --font: "Segoe UI", system-ui, sans-serif;
    --mono: "Cascadia Mono", "Consolas", monospace;
  }
  * { box-sizing: border-box; }
  body {
    margin: 0; background: var(--bg); color: var(--text);
    font: 14px/1.45 var(--font);
    min-height: 100vh;
  }
  header {
    display: flex; align-items: center; gap: 1rem; flex-wrap: wrap;
    padding: 0.85rem 1.25rem;
    border-bottom: 1px solid var(--border);
    background: var(--panel);
  }
  header h1 { margin: 0; font-size: 1.1rem; font-weight: 600; letter-spacing: 0.02em; }
  .badge {
    font-size: 0.75rem; padding: 0.15rem 0.5rem; border-radius: 999px;
    background: #333843; color: var(--muted);
  }
  .badge.on { background: #1e3d2f; color: #7dcea0; }
  nav { display: flex; gap: 0.25rem; margin-left: auto; flex-wrap: wrap; }
  nav button {
    background: transparent; border: 1px solid transparent; color: var(--muted);
    padding: 0.4rem 0.85rem; border-radius: 6px; cursor: pointer; font: inherit;
  }
  nav button.active, nav button:hover {
    color: var(--text); border-color: var(--border); background: #2c313a;
  }
  main { max-width: 920px; margin: 0 auto; padding: 1.25rem; }
  .tab { display: none; }
  .tab.active { display: block; }
  .row { display: flex; flex-wrap: wrap; gap: 0.6rem; margin-bottom: 1rem; align-items: center; }
  button.btn {
    background: var(--accent); color: #0b1220; border: none;
    padding: 0.5rem 1rem; border-radius: 6px; font: inherit; font-weight: 600; cursor: pointer;
  }
  button.btn.secondary { background: #3a404c; color: var(--text); }
  button.btn.danger { background: var(--danger); color: #fff; }
  button.btn:disabled { opacity: 0.5; cursor: default; }
  .log {
    background: #12151a; border: 1px solid var(--border); border-radius: 8px;
    padding: 0.75rem 1rem; height: 420px; overflow: auto;
    font: 12.5px/1.5 var(--mono); white-space: pre-wrap; word-break: break-word;
  }
  .form-grid { display: grid; gap: 0.85rem; }
  label { display: grid; gap: 0.3rem; color: var(--muted); font-size: 0.85rem; }
  input, select, textarea {
    background: #12151a; border: 1px solid var(--border); color: var(--text);
    border-radius: 6px; padding: 0.5rem 0.65rem; font: inherit;
  }
  textarea { min-height: 70px; resize: vertical; }
  .hint { color: var(--muted); font-size: 0.8rem; margin-top: 0.25rem; }
  .path { font: 12px var(--mono); color: var(--muted); margin-bottom: 1rem; }
  .section-title { margin: 1.25rem 0 0.5rem; font-size: 0.95rem; color: var(--text); }
  .section-title:first-child { margin-top: 0; }
  .two-col { display: grid; grid-template-columns: 1fr 1fr; gap: 0.85rem; }
  .check-row {
    display: flex; align-items: center; gap: 0.55rem; flex-direction: row;
    color: var(--text); font-size: 0.9rem;
  }
  .check-row input { width: auto; margin: 0; }
  .form-actions {
    display: none; align-items: center; gap: 0.75rem; flex-wrap: wrap;
    margin-top: 1.5rem; padding-top: 1rem; border-top: 1px solid var(--border);
  }
</style>
</head>
<body>
<header>
  <h1>WoW AI Bridge</h1>
  <span id="badge" class="badge">stopped</span>
  <nav>
    <button type="button" class="active" data-tab="status">Status</button>
    <button type="button" data-tab="game">Game</button>
    <button type="button" data-tab="llm">LLM</button>
    <button type="button" data-tab="whisper">Whisper mode</button>
    <button type="button" data-tab="party">Party mode</button>
    <button type="button" data-tab="say">Say mode</button>
    <button type="button" data-tab="logging">Logging</button>
  </nav>
</header>
<main>
  <section id="tab-status" class="tab active">
    <div class="row">
      <button class="btn" id="btn-start" type="button">Start</button>
      <button class="btn danger" id="btn-stop" type="button">Stop</button>
      <button class="btn secondary" id="btn-slots" type="button">Install slots</button>
      <button class="btn secondary" id="btn-probe" type="button">Probe capture</button>
    </div>
    <div class="row">
      <input id="inject-text" placeholder="Inject test message (no game capture)" style="flex:1;min-width:200px"/>
      <button class="btn secondary" id="btn-inject" type="button">Inject</button>
    </div>
    <div class="log" id="log"></div>
  </section>

  <form id="settings" autocomplete="off">
    <section id="tab-game" class="tab">
      <p class="path" id="cfg-path"></p>
      <div class="form-grid">
        <label>AddOns folder (…\Interface\AddOns)
          <input name="addonDir" id="addonDir"/>
        </label>
        <label>SavedVariables file (optional, for /reload outbox)
          <input name="savedVariablesFile" id="savedVariablesFile"/>
        </label>
        <label>Default project folder
          <input name="defaultCwd" id="defaultCwd"/>
        </label>
        <div class="two-col">
          <label>TOC Interface
            <input name="tocInterface" id="tocInterface"/>
          </label>
          <label>Max game clients (instances)
            <input name="maxInstances" id="maxInstances" type="number" min="1" max="8"/>
          </label>
        </div>
        <label>WoW process name (no .exe)
          <input name="processName" id="processName"/>
        </label>
        <p class="hint">Two clients from the same install: Install slots once, set each window with /wow-ai instance 1 and /wow-ai instance 2 (left→right order), then restart WoW.</p>
      </div>
    </section>

    <section id="tab-llm" class="tab">
      <div class="form-grid">
        <label>Provider
          <select name="provider" id="provider">
            <option value="openai">OpenAI</option>
            <option value="deepseek">DeepSeek</option>
            <option value="anthropic">Anthropic</option>
            <option value="openai-compatible">OpenAI-compatible</option>
          </select>
        </label>
        <label>API base URL
          <input name="baseURL" id="baseURL"/>
        </label>
        <label>API key / access token
          <input name="apiKey" id="apiKey" type="password"/>
        </label>
        <label>Model
          <input name="model" id="model"/>
          <span class="hint" id="modelHint"></span>
        </label>
        <div class="two-col">
          <label>In-game agent name
            <input name="agentName" id="agentName"/>
          </label>
          <label>Max tokens
            <input name="maxTokens" id="maxTokens" type="number"/>
          </label>
        </div>
        <label>Temperature
          <input name="temperature" id="temperature" type="number" step="0.1" min="0" max="2"/>
        </label>
        <p class="hint">DeepSeek: models are <code>deepseek-chat</code> (fast) and <code>deepseek-reasoner</code> (slower, deeper). Keys are stored in your user config file (mode 0600). Prefer OS credential stores for production later.</p>
      </div>
    </section>

    <section id="tab-whisper" class="tab">
      <div class="form-grid">
        <label class="check-row">
          <input name="whisperEnabled" id="whisperEnabled" type="checkbox"/>
          Auto-reply to whispers (bridge master switch)
        </label>
        <p class="hint">When off, the bridge ignores whisper auto-reply jobs and answers &quot;skip&quot;, so nothing is spoken. The in-game <b>Auto-whisper</b> checkbox still decides whether the addon sends a whisper at all — a private whisper is never answered unless that is on.</p>
        <div class="section-title">Reply style</div>
        <div class="two-col">
          <label>Personality
            <select id="whisperPersonality"></select>
          </label>
          <label>Education level
            <select id="whisperEducation"></select>
          </label>
        </div>
        <label>Characteristics
          <select id="whisperCharacteristics"></select>
        </label>
        <p class="hint">Optional. These only bend tone and word choice — the persona's rules, the length limit, and the &quot;never sound like an AI&quot; rule all still apply. Leave them on Default to keep the voice exactly as designed.</p>
      </div>
    </section>

    <section id="tab-party" class="tab">
      <div class="form-grid">
        <label class="check-row">
          <input name="partyEnabled" id="partyEnabled" type="checkbox"/>
          Auto-reply in party chat (bridge master switch)
        </label>
        <p class="hint">When off, the bridge ignores party auto-reply jobs and answers &quot;skip&quot;. The in-game <b>Auto-party</b> checkbox still decides whether the addon sends a party line at all.</p>
        <div class="section-title">Reply style</div>
        <div class="two-col">
          <label>Personality
            <select id="partyPersonality"></select>
          </label>
          <label>Education level
            <select id="partyEducation"></select>
          </label>
        </div>
        <label>Characteristics
          <select id="partyCharacteristics"></select>
        </label>
        <p class="hint">Optional. These only bend tone and word choice — the persona's rules, the length limit, and the &quot;never sound like an AI&quot; rule all still apply. Leave them on Default to keep the voice exactly as designed.</p>
      </div>
    </section>

    <section id="tab-say" class="tab">
      <div class="form-grid">
        <label class="check-row">
          <input name="sayEnabled" id="sayEnabled" type="checkbox"/>
          Auto-say replies to nearby /say (bridge master switch)
        </label>
        <p class="hint">The in-game <b>Auto-say</b> checkbox decides whether surrounding /say lines are sent to the bridge. When this switch is off, the bridge collects and then quietly discards them.</p>
        <div class="two-col">
          <label>Reply chance (%)
            <input name="sayReplyChance" id="sayReplyChance" type="number" min="0" max="100"/>
          </label>
          <label>Collect for (seconds)
            <input name="sayCollectSeconds" id="sayCollectSeconds" type="number" min="1" max="300"/>
          </label>
        </div>
        <p class="hint">After each collection window the bridge rolls the reply chance. A lower chance means the character chimes in less often.</p>
        <div class="two-col">
          <label>Min words per reply
            <input name="sayMinWords" id="sayMinWords" type="number" min="1" max="100"/>
          </label>
          <label>Max words per reply
            <input name="sayMaxWords" id="sayMaxWords" type="number" min="1" max="100"/>
          </label>
        </div>
        <p class="hint">Each reply picks a random length between min and max, so answers vary instead of all matching.</p>
        <div class="section-title">Reply style</div>
        <div class="two-col">
          <label>Personality
            <select id="sayPersonality"></select>
          </label>
          <label>Education level
            <select id="sayEducation"></select>
          </label>
        </div>
        <label>Characteristics
          <select id="sayCharacteristics"></select>
        </label>
        <p class="hint">Optional. These only bend tone and word choice — the persona's rules, the word budget, and the &quot;never sound like an AI&quot; rule all still apply. Leave them on Default to keep the voice exactly as designed.</p>
      </div>
    </section>

    <section id="tab-logging" class="tab">
      <div class="form-grid">
        <label class="check-row">
          <input name="debug" id="debug" type="checkbox"/>
          Debug log
        </label>
        <p class="hint">When off, Status only shows game chat traffic and errors. When on, also shows capture lock, startup, context size, probes, say word budgets, and raw model replies.</p>
      </div>
    </section>

    <div class="form-actions" id="form-actions">
      <button class="btn" type="submit">Save settings</button>
      <span class="hint" id="save-note"></span>
    </div>
  </form>
</main>
<script>
const $ = (id) => document.getElementById(id);

const SETTINGS_TABS = ['game','llm','whisper','party','say','logging'];

const PROVIDERS = {
  openai:              { base: 'https://api.openai.com/v1',   model: 'gpt-4o-mini',              hint: 'e.g. gpt-4o-mini, gpt-4o' },
  deepseek:            { base: 'https://api.deepseek.com/v1', model: 'deepseek-chat',            hint: 'deepseek-chat (fast) or deepseek-reasoner (slower, deeper)' },
  anthropic:           { base: 'https://api.anthropic.com',   model: 'claude-3-5-sonnet-latest', hint: 'e.g. claude-3-5-sonnet-latest' },
  'openai-compatible': { base: '',                            model: '',                         hint: 'Any OpenAI-style /chat/completions endpoint (local server, OpenRouter, ...)' },
};

function showTab(name) {
  document.querySelectorAll('nav button').forEach(b => b.classList.toggle('active', b.dataset.tab === name));
  document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
  const el = $('tab-' + name);
  if (el) el.classList.add('active');
  $('form-actions').style.display = SETTINGS_TABS.includes(name) ? 'flex' : 'none';
}
document.querySelectorAll('nav button').forEach(btn => {
  btn.onclick = () => showTab(btn.dataset.tab);
});

const num = (id, def) => {
  const v = $(id).value.trim();
  return v === '' ? def : Number(v);
};

function applyProviderPreset(force) {
  const p = PROVIDERS[$('provider').value] || {};
  const knownBases = Object.values(PROVIDERS).map(x => x.base).filter(Boolean);
  const knownModels = Object.values(PROVIDERS).map(x => x.model).filter(Boolean);
  const base = $('baseURL').value.trim();
  const model = $('model').value.trim();
  if (force || base === '' || knownBases.includes(base)) $('baseURL').value = p.base || '';
  if (force || model === '' || knownModels.includes(model)) $('model').value = p.model || '';
  $('modelHint').textContent = p.hint || '';
}
$('provider').onchange = () => applyProviderPreset(false);

// The personality/education/characteristics dropdowns are filled from the same
// Go catalogs the prompt builder uses, so labels can never drift from behaviour.
const STYLE_MODES = ['whisper', 'party', 'say'];
const STYLE_FIELDS = [['Personality', 'personalities'], ['Education', 'education'], ['Characteristics', 'characteristics']];

async function fillStyleSelects() {
  let opts;
  try {
    opts = await (await fetch('/api/style-options')).json();
  } catch (_) {
    return;
  }
  for (const mode of STYLE_MODES) {
    for (const [field, key] of STYLE_FIELDS) {
      const sel = $(mode + field);
      if (!sel) continue;
      sel.innerHTML = '';
      for (const o of (opts[key] || [])) {
        const opt = document.createElement('option');
        opt.value = o.key;
        opt.textContent = o.label;
        sel.appendChild(opt);
      }
    }
  }
}

function readStyle(mode) {
  return {
    personality: $(mode + 'Personality').value,
    education: $(mode + 'Education').value,
    characteristics: $(mode + 'Characteristics').value,
  };
}

function applyStyle(mode, s) {
  const m = s || {};
  $(mode + 'Personality').value = m.personality || '';
  $(mode + 'Education').value = m.education || '';
  $(mode + 'Characteristics').value = m.characteristics || '';
}

async function refresh() {
  const s = await (await fetch('/api/status')).json();
  const badge = $('badge');
  badge.textContent = s.running ? 'running' : 'stopped';
  badge.className = 'badge' + (s.running ? ' on' : '');
  if (!s.running) {
    // Keep a sticky hint in the log area header via title attribute.
    $('log').title = 'Capture is stopped — click Start or the strip is never read';
  } else {
    $('log').title = '';
  }
  $('log').textContent = (s.logs || []).join('\n');
  $('log').scrollTop = $('log').scrollHeight;
  $('cfg-path').textContent = 'Config: ' + (s.cfgPath || '');
  $('btn-start').disabled = s.running;
  $('btn-stop').disabled = !s.running;
}

async function loadConfig() {
  const c = await (await fetch('/api/config')).json();
  $('addonDir').value = c.addonDir || '';
  $('savedVariablesFile').value = c.savedVariablesFile || '';
  $('defaultCwd').value = c.defaultCwd || '';
  $('tocInterface').value = c.tocInterface || '30300';
  $('maxInstances').value = c.maxInstances || 4;
  $('processName').value = (c.capture && c.capture.processName) || 'Wow';

  const llm = c.llm || {};
  $('provider').value = llm.provider || 'openai';
  $('baseURL').value = llm.baseURL || '';
  $('apiKey').value = llm.apiKey || '';
  $('model').value = llm.model || '';
  $('agentName').value = llm.agentName || 'api';
  $('maxTokens').value = llm.maxTokens || 2048;
  $('temperature').value = (llm.temperature != null) ? llm.temperature : 0.7;
  applyProviderPreset(false);

  const m = c.modes || {};
  const mw = m.whisper || {}, mp = m.party || {}, ms = m.say || {};
  $('whisperEnabled').checked = mw.enabled !== false;
  $('partyEnabled').checked = mp.enabled !== false;
  $('sayEnabled').checked = ms.enabled !== false;
  $('sayReplyChance').value = (ms.replyChance != null) ? ms.replyChance : 50;
  $('sayCollectSeconds').value = (ms.collectSeconds != null) ? ms.collectSeconds : 10;
  $('sayMinWords').value = (ms.minWords != null) ? ms.minWords : 4;
  $('sayMaxWords').value = (ms.maxWords != null) ? ms.maxWords : 13;
  applyStyle('whisper', mw);
  applyStyle('party', mp);
  applyStyle('say', ms);

  $('debug').checked = !!c.debug;
}

$('settings').onsubmit = async (e) => {
  e.preventDefault();
  const cur = await (await fetch('/api/config')).json();
  cur.addonDir = $('addonDir').value.trim();
  cur.inboxFile = cur.addonDir ? cur.addonDir.replace(/[\\/]+$/, '') + '\\WoWAI\\Inbox.lua' : '';
  cur.savedVariablesFile = $('savedVariablesFile').value.trim();
  cur.defaultCwd = $('defaultCwd').value.trim();
  cur.tocInterface = $('tocInterface').value.trim() || '30300';
  cur.maxInstances = num('maxInstances', 4) || 4;
  cur.capture = cur.capture || {};
  cur.capture.enabled = true;
  cur.capture.processName = $('processName').value.trim() || 'Wow';

  cur.llm = cur.llm || {};
  cur.llm.provider = $('provider').value;
  cur.llm.baseURL = $('baseURL').value.trim();
  cur.llm.apiKey = $('apiKey').value;
  cur.llm.model = $('model').value.trim();
  cur.llm.agentName = $('agentName').value.trim() || 'api';
  cur.llm.maxTokens = num('maxTokens', 2048) || 2048;
  cur.llm.temperature = num('temperature', 0.7);

  cur.modes = {
    whisper: Object.assign({ enabled: $('whisperEnabled').checked }, readStyle('whisper')),
    party:   Object.assign({ enabled: $('partyEnabled').checked }, readStyle('party')),
    say: Object.assign({
      enabled: $('sayEnabled').checked,
      replyChance: num('sayReplyChance', 50),
      collectSeconds: num('sayCollectSeconds', 10),
      minWords: num('sayMinWords', 4),
      maxWords: num('sayMaxWords', 13),
    }, readStyle('say')),
  };

  cur.debug = $('debug').checked;

  const res = await fetch('/api/config', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(cur),
  });
  const note = $('save-note');
  if (!res.ok) {
    note.textContent = 'save failed: ' + (await res.text());
  } else {
    note.textContent = 'saved';
    setTimeout(() => { note.textContent = ''; }, 2000);
  }
  await refresh();
};

$('btn-start').onclick = async () => {
  const res = await fetch('/api/start', {method:'POST'});
  if (!res.ok) alert(await res.text());
  await refresh();
};
$('btn-stop').onclick = async () => { await fetch('/api/stop', {method:'POST'}); await refresh(); };
$('btn-slots').onclick = async () => {
  const res = await fetch('/api/install-slots', {method:'POST'});
  if (!res.ok) alert(await res.text());
  await refresh();
};
$('btn-probe').onclick = async () => {
  const res = await fetch('/api/probe', {method:'POST'});
  const body = await res.text();
  if (!res.ok) {
    alert('Probe failed: ' + body);
  } else {
    try {
      const j = JSON.parse(body);
      if (j.error) alert(j.error);
      else if (!j.windows || !j.windows.length) alert('No WoW windows found for process "' + (j.processName||'?') + '"');
      else {
        const lines = j.windows.map(w =>
          'i' + w.instance + ' clientOrigin=(' + w.clientScreenX + ',' + w.clientScreenY + ') → ' + w.decode +
          (w.imagePath ? '\n  saved ' + w.imagePath : '')
        );
        alert((j.note ? j.note + '\n\n' : '') + lines.join('\n'));
      }
    } catch (_) {}
  }
  await refresh();
};
$('btn-inject').onclick = async () => {
  const text = $('inject-text').value.trim();
  if (!text) return;
  const res = await fetch('/api/inject', {
    method: 'POST',
    headers: {'Content-Type':'application/json'},
    body: JSON.stringify({text}),
  });
  if (!res.ok) {
    alert('Inject failed: ' + (await res.text()));
    await refresh();
    return;
  }
  $('inject-text').value = '';
  await refresh();
};

(async () => {
  await fillStyleSelects();
  await loadConfig();
  refresh();
  setInterval(refresh, 1500);
})();
</script>
</body>
</html>
`))
