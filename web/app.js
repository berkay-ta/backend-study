"use strict";

const KEY_BASE = "il.base", KEY_API = "il.key", KEY_LAST = "il.last";
const get = (k) => localStorage.getItem(k);
const $ = (s) => document.querySelector(s);
// Just a unique idempotency key. randomUUID needs a secure context
// (HTTPS/localhost); the suffix covers plain-HTTP LAN access.
const uuid = () =>
  crypto.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`;

const state = {
  leagues: [], league: null, teams: {}, fixtures: [], standings: [],
  history: [], lastPred: null, whatif: {}, viewWeek: null,
  showDetail: false, editing: null, inFlightKeys: {},
};

// ----- DOM helper -----
function el(tag, attrs, ...kids) {
  const e = document.createElement(tag);
  if (attrs) for (const [k, v] of Object.entries(attrs)) {
    if (v == null || v === false) continue;
    if (k === "class") e.className = v;
    else if (k.startsWith("on")) e[k] = v;
    else e.setAttribute(k, v);
  }
  for (const c of kids.flat()) {
    if (c == null || c === false) continue;
    e.append(c.nodeType ? c : String(c));
  }
  return e;
}

// ----- API -----
async function api(method, path, body, extra) {
  const h = { accept: "application/json" };
  if (body !== undefined) h["content-type"] = "application/json";
  if (get(KEY_API)) h["X-API-Key"] = get(KEY_API);
  if (extra) Object.assign(h, extra);
  const r = await fetch((get(KEY_BASE) || "") + path, {
    method, headers: h, body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (r.status === 204) return null;
  const j = await r.json().catch(() => null);
  if (!r.ok) {
    const er = j?.error || { code: "http_" + r.status, message: r.statusText };
    const e = new Error(er.message);
    e.code = er.code; e.status = r.status; e.fields = er.fields;
    throw e;
  }
  return j?.data;
}


let statusTimer;
function status(msg, kind) {
  const s = $("#status");
  s.textContent = msg;
  s.className = kind || "";
  s.hidden = false;
  clearTimeout(statusTimer);
  statusTimer = setTimeout(() => (s.hidden = true), 3000);
}

// ----- loaders -----
async function loadLeagues() {
  try { state.leagues = (await api("GET", "/api/v1/leagues")) || []; }
  catch (e) { status(e.message, "error"); return; }
  const sel = $("#league-select");
  sel.innerHTML = "";
  if (state.leagues.length === 0) {
    sel.hidden = true;
    $("#empty").hidden = false;
    $("#league").hidden = true;
    return;
  }
  $("#empty").hidden = true;
  sel.hidden = false;
  state.leagues.forEach((l) => sel.append(el("option", { value: l.id }, l.name)));
  const last = Number(localStorage.getItem(KEY_LAST));
  const pick = state.leagues.find((l) => l.id === last) || state.leagues[0];
  await selectLeague(pick.id);
}

async function selectLeague(id) {
  localStorage.setItem(KEY_LAST, String(id));
  $("#league-select").value = String(id);
  state.whatif = {};
  state.viewWeek = null;
  state.lastPred = null;
  await refresh(id);
}

async function refresh(id) {
  const lid = id ?? state.league?.id;
  if (!lid) return;
  try {
    const [lg, teams, fx, st, hist] = await Promise.all([
      api("GET", `/api/v1/leagues/${lid}`),
      api("GET", `/api/v1/leagues/${lid}/teams`),
      api("GET", `/api/v1/leagues/${lid}/fixtures`),
      api("GET", `/api/v1/leagues/${lid}/standings`),
      api("GET", `/api/v1/leagues/${lid}/predictions`),
    ]);
    state.league = lg;
    state.teams = Object.fromEntries((teams || []).map((t) => [t.id, t]));
    state.fixtures = fx || [];
    state.standings = st || [];
    state.history = hist || [];
    if (state.lastPred == null) {
      state.lastPred = state.history.find((p) => !p.stale) || state.history[0] || null;
    }
    const last = Math.max(1, Math.min(lg.current_week - 1, lg.total_weeks)) || 1;
    if (state.viewWeek == null || state.viewWeek < 1 || state.viewWeek > lg.total_weeks) {
      state.viewWeek = last;
    }
    render();
  } catch (e) { status(e.message, "error"); }
}

// ----- render -----
const teamName = (id) => state.teams[id]?.name || `#${id}`;

function render() {
  const l = state.league;
  $("#league").hidden = false;
  const played = Math.max(0, Math.min(l.current_week - 1, l.total_weeks));
  $("#m-played").textContent = `${played} / ${l.total_weeks}`;
  $("#m-status").textContent = l.complete ? "complete" : "in progress";
  $("#m-version").textContent = "v" + l.version;
  $("#btn-next").disabled = !!l.complete;
  $("#btn-all").disabled = !!l.complete;
  $("#btn-pred").disabled = !!l.complete || l.current_week <= 4;
  renderStandings();
  renderMatches();
  renderPrediction();
  renderWhatIf();
  renderHistory();
}

function renderStandings() {
  const tb = $("#standings");
  tb.innerHTML = "";
  state.standings.forEach((s, i) =>
    tb.append(el("tr", { class: i === 0 && s.played > 0 ? "lead" : null },
      el("td", null, String(i + 1)),
      el("td", { class: "team" }, s.team_name),
      el("td", { class: "num" }, s.played),
      el("td", { class: "num" }, s.won),
      el("td", { class: "num" }, s.drawn),
      el("td", { class: "num" }, s.lost),
      el("td", { class: "num" }, (s.goal_diff >= 0 ? "+" : "") + s.goal_diff),
      el("td", { class: "num pts" }, s.points),
    )));
}

function renderMatches() {
  const wrap = $("#matches");
  wrap.innerHTML = "";
  const week = state.viewWeek;
  const wk = state.fixtures.find((w) => w.week === week);
  $("#week-label").textContent = "Week " + week;
  $("#btn-prev-week").disabled = week <= 1;
  $("#btn-next-view-week").disabled = week >= state.league.total_weeks;
  if (!wk) { wrap.append(el("p", { class: "hint", style: "padding:12px 14px" }, "No fixtures for this week.")); return; }
  wk.matches.forEach((m) => {
    wrap.append(el("div", {
      class: "match-row " + (m.played ? "played" : "unplayed"),
      onclick: m.played ? () => openEdit(m) : null,
      title: m.played ? "click to edit" : null,
    },
      el("span", { class: "h" }, teamName(m.home_team_id)),
      el("span", { class: "sc" }, m.played ? `${m.home_goals} – ${m.away_goals}` : "vs"),
      el("span", { class: "a" }, teamName(m.away_team_id)),
    ));
  });
}

function predRows(p) {
  return p?.result?.projected_table || p?.result?.entries || [];
}

function renderPrediction() {
  const out = $("#pred-out");
  out.innerHTML = "";
  out.className = "";
  const p = state.lastPred;
  const l = state.league;
  const toggleBtn = $("#btn-toggle-detail");

  if (!p) {
    out.className = "muted pred-empty";
    out.textContent = l?.complete
      ? "Season complete"
      : l && l.current_week <= 4
      ? "Run after week 4"
      : "No prediction yet";
    toggleBtn.hidden = true;
    return;
  }
  toggleBtn.hidden = false;
  toggleBtn.textContent = state.showDetail ? "hide detail" : "show detail";

  const rows = predRows(p);
  const list = el("div", { class: "pred-list" });
  rows.forEach((r, i) => {
    const pct = Number(r.championship_pct ?? 0);
    const row = el("div", { class: "pred-row" + (i === 0 ? " lead" : "") },
      el("span", { class: "ppos" }, r.projected_position),
      el("span", { class: "pname" }, r.team_name),
      el("span", { class: "ppct" }, pct.toFixed(1) + "%"),
      el("span", { class: "pbar" }, el("span", { style: `width:${Math.min(100, Math.max(0, pct))}%` })),
    );
    list.append(row);
  });
  out.append(list);

  if (state.showDetail) out.append(detailTable(rows));

  out.append(el("div", { class: "pred-meta" },
    el("span", null, p.strategy),
    el("span", null, "v" + p.league_version),
    el("span", { class: p.stale ? "stale" : null }, p.stale ? "stale" : "fresh"),
    el("span", null, new Date(p.created_at).toLocaleString()),
  ));
}

function detailTable(rows) {
  const t = el("table", { class: "pred-detail" });
  t.append(el("thead", null, el("tr", null,
    el("th", null, "Team"),
    el("th", { class: "num" }, "avg pos"),
    el("th", { class: "num" }, "xPts"),
    el("th", { class: "num" }, "xGD"),
  )));
  const tb = el("tbody");
  rows.forEach((r) => tb.append(el("tr", null,
    el("td", null, r.team_name),
    el("td", { class: "num" }, (r.average_position ?? 0).toFixed(2)),
    el("td", { class: "num" }, (r.expected_points ?? 0).toFixed(1)),
    el("td", { class: "num" }, ((r.expected_goal_diff ?? 0) >= 0 ? "+" : "") + (r.expected_goal_diff ?? 0).toFixed(1)),
  )));
  t.append(tb);
  return t;
}

function renderHistory() {
  const wrap = $("#history");
  wrap.innerHTML = "";
  if (state.history.length === 0) { wrap.className = "muted"; wrap.textContent = "None yet."; return; }
  wrap.className = "";
  state.history.forEach((p) => {
    wrap.append(el("div", {
      class: "run-row",
      onclick: () => { state.lastPred = p; renderPrediction(); status("loaded run #" + p.id); },
    },
      el("span", null, "#" + p.id),
      el("span", null, p.strategy),
      el("span", null, "v" + p.league_version),
      el("span", { class: p.stale ? "stale" : "fresh" }, p.stale ? "stale" : "fresh"),
      el("span", null, predRows(p)[0]?.team_name || "—"),
      el("span", null, new Date(p.created_at).toLocaleString()),
    ));
  });
}

function renderWhatIf() {
  const wrap = $("#whatif-list");
  wrap.innerHTML = "";
  const played = state.fixtures.flatMap((wk) =>
    wk.matches.filter((m) => m.played).map((m) => ({ ...m, week: wk.week })));
  if (played.length === 0) {
    wrap.append(el("p", { class: "hint" }, "Play at least one match to set overrides."));
    return;
  }
  const t = el("table", { class: "wi-table" });
  t.append(el("thead", null, el("tr", null,
    el("th", { class: "center" }, "Wk"),
    el("th", null, "Home"),
    el("th", { class: "center" }, "Score"),
    el("th", null, "Away"),
  )));
  const tb = el("tbody");
  played.forEach((m) => {
    const ov = state.whatif[m.id];
    const home = el("input", { type: "number", min: 0, value: ov?.home_goals ?? m.home_goals });
    const away = el("input", { type: "number", min: 0, value: ov?.away_goals ?? m.away_goals });
    const update = () => {
      const h = Number(home.value), a = Number(away.value);
      if (!Number.isFinite(h) || !Number.isFinite(a) || h < 0 || a < 0) return;
      if (h === m.home_goals && a === m.away_goals) delete state.whatif[m.id];
      else state.whatif[m.id] = { home_goals: h, away_goals: a };
    };
    home.oninput = update; away.oninput = update;
    tb.append(el("tr", null,
      el("td", { class: "wk" }, m.week),
      el("td", { class: "h" }, teamName(m.home_team_id)),
      el("td", { class: "sc" }, home, el("span", { class: "dash" }, "–"), away),
      el("td", { class: "a" }, teamName(m.away_team_id)),
    ));
  });
  t.append(tb);
  wrap.append(t);
}

// ----- edit modal -----
function openEdit(m) {
  state.editing = m;
  $("#edit-teams").textContent = `${teamName(m.home_team_id)}  vs  ${teamName(m.away_team_id)}  ·  Week ${m.week ?? state.viewWeek}`;
  $("#edit-home").value = m.home_goals;
  $("#edit-away").value = m.away_goals;
  $("#edit-modal").showModal();
}
async function submitEdit(ev) {
  ev.preventDefault();
  const m = state.editing; if (!m) return;
  const h = Number($("#edit-home").value), a = Number($("#edit-away").value);
  if (!Number.isFinite(h) || !Number.isFinite(a) || h < 0 || a < 0) {
    status("scores must be non-negative", "error"); return;
  }
  try {
    await api("PATCH", `/api/v1/leagues/${state.league.id}/matches/${m.id}`, { home_goals: h, away_goals: a });
    $("#edit-modal").close();
    state.editing = null;
    status("match saved", "ok");
    await refresh();
  } catch (e) { status(e.message, "error"); }
}

// ----- actions -----
async function playNext() {
  const key = state.inFlightKeys.playNext || uuid();
  state.inFlightKeys.playNext = key;
  try {
    const d = await api("POST", `/api/v1/leagues/${state.league.id}/weeks/next`, undefined, { "Idempotency-Key": key });
    if (d?.week) state.viewWeek = d.week;
    status(`played week ${d?.week ?? ""}`.trim(), "ok");
    await refresh();
  } catch (e) { status(e.message, "error"); }
  finally { delete state.inFlightKeys.playNext; }
}

async function playAll() {
  const key = state.inFlightKeys.playAll || uuid();
  state.inFlightKeys.playAll = key;
  try {
    const d = await api("POST", `/api/v1/leagues/${state.league.id}/play-all`, undefined, { "Idempotency-Key": key });
    state.viewWeek = state.league.total_weeks;
    const n = d?.weeks?.length || 0;
    status(`played ${n} week${n === 1 ? "" : "s"}`, "ok");
    await refresh();
  } catch (e) { status(e.message, "error"); }
  finally { delete state.inFlightKeys.playAll; }
}

async function runPrediction() {
  const strategy = $("#pred-strategy").value;
  try {
    const d = await api("POST", `/api/v1/leagues/${state.league.id}/predictions?strategy=${encodeURIComponent(strategy)}`);
    state.lastPred = d;
    state.history.unshift(d);
    renderPrediction();
    renderHistory();
    status(`prediction (${strategy}) ready`, "ok");
  } catch (e) {
    if (e.status === 503) status("AI predictor disabled (no API key on server)", "error");
    else status(e.message, "error");
  }
}

async function runWhatIf() {
  const overrides = Object.entries(state.whatif).map(([id, v]) => ({
    match_id: Number(id), home_goals: v.home_goals, away_goals: v.away_goals,
  }));
  const strategy = $("#wi-strategy").value;
  const body = { overrides };
  if (strategy) body.strategy = strategy;
  try {
    const d = await api("POST", `/api/v1/leagues/${state.league.id}/what-if`, body);
    const out = $("#whatif-out");
    out.className = "";
    out.innerHTML = "";
    out.append(el("h4", null, "Hypothetical standings"));
    const t = el("table", { class: "standings" });
    t.append(el("thead", null, el("tr", null,
      el("th", null), el("th", null, "Team"),
      el("th", { class: "num" }, "P"), el("th", { class: "num" }, "W"),
      el("th", { class: "num" }, "D"), el("th", { class: "num" }, "L"),
      el("th", { class: "num" }, "GD"), el("th", { class: "num" }, "Pts"),
    )));
    const tb = el("tbody");
    (d.standings || []).forEach((s, i) => tb.append(el("tr", { class: i === 0 && s.played > 0 ? "lead" : null },
      el("td", null, String(i + 1)),
      el("td", { class: "team" }, s.team_name),
      el("td", { class: "num" }, s.played),
      el("td", { class: "num" }, s.won),
      el("td", { class: "num" }, s.drawn),
      el("td", { class: "num" }, s.lost),
      el("td", { class: "num" }, (s.goal_diff >= 0 ? "+" : "") + s.goal_diff),
      el("td", { class: "num pts" }, s.points),
    )));
    t.append(tb);
    out.append(t);
    if (d.prediction) {
      out.append(el("h4", null, "Hypothetical prediction"));
      const rows = d.prediction.projected_table || d.prediction.entries || [];
      const list = el("div", { class: "pred-list" });
      rows.forEach((r, i) => {
        const pct = Number(r.championship_pct ?? 0);
        list.append(el("div", { class: "pred-row" + (i === 0 ? " lead" : "") },
          el("span", { class: "ppos" }, r.projected_position),
          el("span", { class: "pname" }, r.team_name),
          el("span", { class: "ppct" }, pct.toFixed(1) + "%"),
          el("span", { class: "pbar" }, el("span", { style: `width:${Math.min(100, Math.max(0, pct))}%` })),
        ));
      });
      out.append(list);
    }
    status("what-if computed", "ok");
  } catch (e) { status(e.message, "error"); }
}

function clearWhatIf() {
  state.whatif = {};
  renderWhatIf();
  const out = $("#whatif-out");
  out.className = "muted";
  out.textContent = "No what-if yet.";
}

async function resetLeague() {
  if (!confirm("Reset all match results? Teams and fixtures stay; version bumps.")) return;
  try {
    await api("POST", `/api/v1/leagues/${state.league.id}/reset`);
    state.viewWeek = 1;
    state.lastPred = null;
    state.whatif = {};
    status("league reset", "ok");
    await refresh();
  } catch (e) { status(e.message, "error"); }
}

async function deleteLeague() {
  const name = state.league.name;
  if (!confirm(`Permanently delete "${name}"? This cannot be undone.`)) return;
  try {
    await api("DELETE", `/api/v1/leagues/${state.league.id}`);
    localStorage.removeItem(KEY_LAST);
    state.league = null;
    status("deleted", "ok");
    await loadLeagues();
  } catch (e) { status(e.message, "error"); }
}

async function createLeague(ev) {
  ev.preventDefault();
  const f = ev.target;
  const name = f.querySelector('input[name="name"]').value.trim();
  const rows = Array.from(f.querySelectorAll(".form-table tbody tr"));
  const teams = rows.map((r) => ({
    name: r.querySelector(".t-name").value.trim(),
    strength: Number(r.querySelector(".t-str").value),
  }));
  if (teams.some((t) => !t.name)) { status("all 4 team names required", "error"); return; }
  try {
    const d = await api("POST", "/api/v1/leagues", { name, teams });
    if (d?.league?.id) localStorage.setItem(KEY_LAST, String(d.league.id));
    $("#new-tray").hidden = true;
    status("league created", "ok");
    await loadLeagues();
  } catch (e) { status(e.message, "error"); }
}

// ----- wire -----
$("#league-select").onchange = (e) => selectLeague(Number(e.target.value));
$("#btn-new").onclick = () => { $("#new-tray").hidden = !$("#new-tray").hidden; };
$("#btn-cancel-new").onclick = () => { $("#new-tray").hidden = true; };
$("#new-form").onsubmit = createLeague;
$("#btn-next").onclick = playNext;
$("#btn-all").onclick = playAll;
$("#btn-pred").onclick = runPrediction;
$("#btn-toggle-detail").onclick = () => { state.showDetail = !state.showDetail; renderPrediction(); };
$("#btn-prev-week").onclick = () => { if (state.viewWeek > 1) { state.viewWeek--; renderMatches(); } };
$("#btn-next-view-week").onclick = () => { if (state.viewWeek < state.league.total_weeks) { state.viewWeek++; renderMatches(); } };
$("#btn-whatif").onclick = runWhatIf;
$("#btn-whatif-clear").onclick = clearWhatIf;
$("#btn-reset").onclick = resetLeague;
$("#btn-delete").onclick = deleteLeague;
$("#edit-form").onsubmit = submitEdit;
$("#btn-edit-cancel").onclick = () => { state.editing = null; $("#edit-modal").close(); };
$("#btn-settings").onclick = (e) => {
  e.preventDefault();
  const t = $("#settings-tray");
  t.hidden = !t.hidden;
  $("#s-base").value = get(KEY_BASE) || "";
  $("#s-key").value = get(KEY_API) || "";
};
$("#btn-save-settings").onclick = () => {
  const b = $("#s-base").value.trim().replace(/\/+$/, "");
  const k = $("#s-key").value.trim();
  if (b) localStorage.setItem(KEY_BASE, b); else localStorage.removeItem(KEY_BASE);
  if (k) localStorage.setItem(KEY_API, k); else localStorage.removeItem(KEY_API);
  $("#settings-tray").hidden = true;
  loadLeagues();
};

loadLeagues();
