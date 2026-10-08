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
    display: flex; align-items: center; gap: 1rem;
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
  nav { display: flex; gap: 0.25rem; margin-left: auto; }
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
</style>
</head>
<body>
<header>
  <h1>WoW AI Bridge</h1>
  <span id="badge" class="badge">stopped</span>
  <nav>
    <button type="button" class="active" data-tab="status">Status</button>
    <button type="button" data-tab="settings">Settings</button>
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

  <section id="tab-settings" class="tab">
    <p class="path" id="cfg-path"></p>
    <form id="settings" class="form-grid" autocomplete="off">
      <div class="section-title">Game</div>
      <label>AddOns folder (…\Interface\AddOns)
        <input name="addonDir" id="addonDir"/>
      </label>
      <label>SavedVariables file (optional, for /reload outbox)
        <input name="savedVariablesFile" id="savedVariablesFile"/>
      </label>
      <label>Default project folder
        <input name="defaultCwd" id="defaultCwd"/>
      </label>
      <label>TOC Interface
        <input name="tocInterface" id="tocInterface"/>
      </label>
		<label>Max game clients (instances)
        <input name="maxInstances" id="maxInstances" type="number" min="1" max="8"/>
      </label>
      <label>WoW process name (no .exe)
        <input name="processName" id="processName"/>
      </label>
      <p class="hint">Two clients from the same install: Install slots once, set each window with /wow-ai instance 1 and /wow-ai instance 2 (left→right order), then restart WoW.</p>

      <div class="section-title">LLM</div>
      <label>Provider
        <select name="provider" id="provider">
          <option value="openai">OpenAI</option>
          <option value="openai-compatible">OpenAI-compatible</option>
          <option value="anthropic">Anthropic</option>
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
      </label>
      <label>In-game agent name
        <input name="agentName" id="agentName"/>
      </label>
      <label>Max tokens
        <input name="maxTokens" id="maxTokens" type="number"/>
      </label>

      <div class="section-title">Logging</div>
      <label style="display:flex;align-items:center;gap:0.55rem;flex-direction:row">
        <input name="debug" id="debug" type="checkbox" style="width:auto;margin:0"/>
        Debug log
      </label>
      <p class="hint">When off, Status only shows game chat traffic and errors. When on, also shows capture lock, startup, context size, probes, etc.</p>

      <div class="row" style="margin-top:0.5rem">
        <button class="btn" type="submit">Save settings</button>
      </div>
      <p class="hint">Keys are stored in your user config file (mode 0600). Prefer OS credential stores for production later.</p>
    </form>
  </section>
</main>
<script>
const $ = (id) => document.getElementById(id);
document.querySelectorAll('nav button').forEach(btn => {
  btn.onclick = () => {
    document.querySelectorAll('nav button').forEach(b => b.classList.remove('active'));
    document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
    btn.classList.add('active');
    $('tab-' + btn.dataset.tab).classList.add('active');
  };
});

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
  $('provider').value = (c.llm && c.llm.provider) || 'openai';
  $('baseURL').value = (c.llm && c.llm.baseURL) || '';
  $('apiKey').value = (c.llm && c.llm.apiKey) || '';
  $('model').value = (c.llm && c.llm.model) || '';
  $('agentName').value = (c.llm && c.llm.agentName) || 'api';
  $('maxTokens').value = (c.llm && c.llm.maxTokens) || 2048;
  $('debug').checked = !!c.debug;
}

$('settings').onsubmit = async (e) => {
  e.preventDefault();
  const cur = await (await fetch('/api/config')).json();
  cur.addonDir = $('addonDir').value.trim();
  cur.inboxFile = cur.addonDir ? cur.addonDir.replace(/[\\/]+$/, '') + '\\\\WoWAI\\\\Inbox.lua' : '';
  cur.savedVariablesFile = $('savedVariablesFile').value.trim();
  cur.defaultCwd = $('defaultCwd').value.trim();
  cur.tocInterface = $('tocInterface').value.trim() || '30300';
  cur.maxInstances = Number($('maxInstances').value) || 4;
  cur.capture = cur.capture || {};
  cur.capture.enabled = true;
  cur.capture.processName = $('processName').value.trim() || 'Wow';
  cur.llm = cur.llm || {};
  cur.llm.provider = $('provider').value;
  cur.llm.baseURL = $('baseURL').value.trim();
  cur.llm.apiKey = $('apiKey').value;
  cur.llm.model = $('model').value.trim();
  cur.llm.agentName = $('agentName').value.trim() || 'api';
  cur.llm.maxTokens = Number($('maxTokens').value) || 2048;
  cur.debug = $('debug').checked;
  const res = await fetch('/api/config', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(cur),
  });
  if (!res.ok) alert(await res.text());
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
          (w.imagePath ? '\\n  saved ' + w.imagePath : '')
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

loadConfig();
refresh();
setInterval(refresh, 1500);
</script>
</body>
</html>
`))
