
(() => {
"use strict";
if (!window.fetch || !window.DOMParser) return;
let busy = false, baseline = null;

const logSize = () => fetch("/events", {method:"HEAD", cache:"no-store"})
  .then(r => r.headers.get("content-length")).catch(() => null);

async function refresh(toBottom) {
  const res = await fetch(location.href, {cache:"no-store"});
  if (!res.ok) return;
  const doc = new DOMParser().parseFromString(await res.text(), "text/html");
  const ae = document.activeElement;
  const keep = ae && ae.name ? {name: ae.name, value: ae.value} : null;
  const y = scrollY;
  const swap = () => {
    document.body.replaceWith(doc.body);
    if (toBottom && document.querySelector(".msg")) {
      [...document.querySelectorAll(".msg")].slice(-2).forEach(m => m.classList.add("rise"));
      scrollTo(0, document.body.scrollHeight);
    } else scrollTo(0, y);
    if (keep) {
      const el = document.querySelector("[name=\"" + keep.name.replace(/"/g, "") + "\"]");
      if (el) { if (!toBottom) el.value = keep.value; el.focus(); }
    }
  };
  if (document.startViewTransition) document.startViewTransition(swap); else swap();
}

document.addEventListener("submit", e => {
  const f = e.target;
  const action = new URL(f.getAttribute("action") || "", location.href);
  if (!action.pathname.startsWith("/run/")) return;
  e.preventDefault();
  if (busy) return;
  busy = true;
  f.classList.add("busy");
  const body = new URLSearchParams(new FormData(f));
  const first = f.querySelector("input,textarea");
  const text = first ? first.value : "";
  let ghost, think;
  const anchor = [...document.querySelectorAll(".msg")].pop();
  if (anchor && text.trim()) { // a conversation: show the turn in flight, clearly pending
    const label = (role) => { // borrow the who label the projection already uses
      const whos = document.querySelectorAll(".msg." + role + " .who");
      return whos.length ? "<span class='who'></span>" : "";
    };
    ghost = document.createElement("div");
    ghost.className = "msg user pending rise";
    ghost.innerHTML = label("user");
    ghost.appendChild(document.createTextNode(text));
    think = document.createElement("div");
    think.className = "msg assistant pending rise";
    think.innerHTML = label("assistant") + "<span class='dots'><i></i><i></i><i></i></span>";
    for (const role of ["user", "assistant"]) {
      const whos = document.querySelectorAll(".msg." + role + " .who");
      const target = (role === "user" ? ghost : think).querySelector(".who");
      if (whos.length && target) target.textContent = whos[whos.length - 1].textContent;
    }
    anchor.after(ghost, think);
    first.value = "";
    scrollTo({top: document.body.scrollHeight, behavior: "smooth"});
  }
  fetch(action, {method: "POST", body}).then(async r => {
    if (!r.ok) throw new Error((await r.text()).trim() || r.status);
    baseline = null;
    await refresh(true);
  }).catch(err => { // degrade honestly: say what failed, give the words back
    if (ghost) { think.remove(); ghost.remove(); first.value = text; }
    const p = document.createElement("p");
    p.className = "card danger rise";
    p.textContent = "could not run " + action.pathname.slice(5) + ": " + err.message;
    f.before(p);
    setTimeout(() => p.remove(), 8000);
  }).finally(() => { busy = false; f.classList.remove("busy"); });
});

async function tick() {
  if (document.hidden || busy) return;
  const n = await logSize();
  if (n == null) return;
  if (baseline != null && n !== baseline) {
    const nearBottom = innerHeight + scrollY > document.body.scrollHeight - 120;
    await refresh(nearBottom && !!document.querySelector(".msg"));
  }
  baseline = n;
}
setInterval(tick, 2500);
addEventListener("visibilitychange", () => { if (!document.hidden) tick(); });
addEventListener("DOMContentLoaded", tick);
})();
