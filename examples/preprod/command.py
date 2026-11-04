#!/usr/bin/env python3
'''preprod - test a merged change on preprod as Wouter: log in, screenshot, record a verdict on the goal and the PR.

usage: self run preprod login [--as fan|creator]
       self run preprod shot <url|/path> <name> [--as fan|creator] [--anon] [--out <dir>] [--wait <ms>]
       self run preprod verdict <goal> pass|fail|skip <evidence…> [--shot <png>]…

Account: wouter.beets@mym.fans on https://preprod-v2.mym.fans, password from
`pass show preprod-v2.mym.fans/wouter.beets@mym.fans` (SELF_PREPROD_PASS_ENTRY
overrides). Sessions are Playwright storageState files in ~/.self-preprod/
(0600, outside every repo), one per role.

login: headless Chromium signs in at /app/sign-in on the fan or creator tab
and saves the session. It fails loudly when it lands back on a sign-in page.

shot: opens the URL (a path is joined to https://preprod-v2.mym.fans) with
the saved session (logging in first when there is none, or when the page
bounces to sign-in), hides cookie banners and the Symfony toolbar, and writes
<name>-desktop1440.png and <name>-mobile390.png (2x) into --out (default
~/.self-preprod/shots). It prints both paths and the final URL. Read every
PNG before calling a change verified: screenshots catch what assertions miss.
--anon shoots logged out.

verdict: appends preprod.verified {goal, verdict, evidence, pr?, shots[]}
and, when the goal is bound to a PR (ship.tracked), uploads the shots to the
pr-assets branch (pr-<n>-preprod/<file>) and posts one PR comment: verdict,
evidence, screenshots. The ship view reads the verdict: pass or skip lets
the goal notify prod-ready and close; fail holds it and alerts Wouter.

Runtime: node + playwright from SELF_PREPROD_NODE_PATH (default
~/mym/frontends/business/node_modules), its bundled headless Chromium.
Source: ~/self examples/preprod/command.py.
'''
import base64
import json
import os
import pwd
import re
import shutil
import subprocess
import sys
import tempfile
import time

HOME = pwd.getpwuid(os.getuid()).pw_dir
ENV = dict(os.environ, HOME=HOME)
ENV["PATH"] = os.pathsep.join([HOME + "/.local/bin", "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin",
                               ENV.get("PATH", "")])
GH = os.environ.get("SELF_GH_BIN") or shutil.which("gh", path=ENV["PATH"]) or "gh"
NODE = shutil.which("node", path=ENV["PATH"]) or "node"
PASS = shutil.which("pass", path=ENV["PATH"]) or "pass"
BASE = os.environ.get("SELF_PREPROD_URL", "https://preprod-v2.mym.fans").rstrip("/")
EMAIL = os.environ.get("SELF_PREPROD_EMAIL", "wouter.beets@mym.fans")
ENTRY = os.environ.get("SELF_PREPROD_PASS_ENTRY", "preprod-v2.mym.fans/" + EMAIL)
NODE_PATH = os.environ.get("SELF_PREPROD_NODE_PATH", HOME + "/mym/frontends/business/node_modules")
STATE_DIR = HOME + "/.self-preprod"
REPO_SLUG = "MYM-Tech/monorepo"
CWD = os.environ.get("SELF_CWD", ".")

JS = r'''
const { chromium } = require('playwright');
const [mode, role, statePath, target, prefix, waitMs, anon] = process.argv.slice(2);
const BASE = process.env.PP_BASE;
const signIn = u => /\/(app\/)?(sign-in|login)\b/.test(new URL(u).pathname);
async function login(browser) {
  const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 }, locale: 'fr-FR' });
  const p = await ctx.newPage();
  await p.goto(BASE + '/app/sign-in', { timeout: 90000 });
  await p.waitForTimeout(3000);
  await p.getByRole('button', { name: /^(deny|refuser|tout refuser)$/i }).click({ timeout: 3000 }).catch(() => {});
  await p.getByRole('button', { name: role, exact: true }).first().click({ timeout: 5000 }).catch(() => {});
  await p.locator('input[name=username]').fill(process.env.PP_EMAIL);
  await p.locator('input[name=password]').fill(process.env.PP_PASSWORD);
  await p.locator('button[type=submit]').first().click();
  for (let i = 0; i < 30 && signIn(p.url()); i++) await p.waitForTimeout(1000);
  if (signIn(p.url())) {
    await p.screenshot({ path: statePath + '.failed.png' });
    throw new Error('still on sign-in after submit: ' + p.url() + ' (see ' + statePath + '.failed.png)');
  }
  await p.waitForTimeout(2000);
  await ctx.storageState({ path: statePath });
  console.log(JSON.stringify({ loggedIn: p.url() }));
  await ctx.close();
}
async function shoot(browser, w, h, scale, mobile, file) {
  const opts = { viewport: { width: w, height: h }, deviceScaleFactor: scale, isMobile: mobile, hasTouch: mobile, locale: 'fr-FR' };
  if (anon !== '1') opts.storageState = statePath;
  const ctx = await browser.newContext(opts);
  const p = await ctx.newPage();
  await p.goto(target, { timeout: 90000 });
  await p.waitForTimeout(+waitMs);
  if (anon !== '1' && signIn(p.url())) { await ctx.close(); return { bounced: p.url() }; }
  await p.addStyleTag({ content: '.sf-toolbar,.sf-minitoolbar,[id^=sfwdt],[id^=sfMiniToolbar],#didomi-host,#axeptio_overlay{display:none!important}' }).catch(() => {});
  await p.getByRole('button', { name: /^(deny|refuser|tout refuser|continuer sans accepter)$/i }).click({ timeout: 2000 }).catch(() => {});
  await p.waitForTimeout(800);
  await p.screenshot({ path: file, fullPage: true });
  const url = p.url();
  await ctx.close();
  return { file, url };
}
(async () => {
  const browser = await chromium.launch();
  try {
    if (mode === 'login') return await login(browser);
    const out = [];
    for (const [w, h, s, m, tag] of [[1440, 900, 1, false, 'desktop1440'], [390, 844, 2, true, 'mobile390']]) {
      const r = await shoot(browser, w, h, s, m, prefix + '-' + tag + '.png');
      if (r.bounced) { console.log(JSON.stringify({ bounced: r.bounced })); return; }
      out.push(r);
    }
    console.log(JSON.stringify({ shots: out }));
  } finally { await browser.close(); }
})().catch(e => { console.error(String(e && e.message || e)); process.exit(1); });
'''


def say(msg):
    print("preprod: " + msg, file=sys.stderr)


def fail(msg, code=1):
    say(msg)
    raise SystemExit(code)


def take(args, valued, flags=()):
    opts, rest, multi = {}, [], {}
    i = 0
    while i < len(args):
        a = args[i]
        if a in valued and i + 1 < len(args):
            multi.setdefault(a, []).append(args[i + 1])
            opts[a] = args[i + 1]
            i += 2
        elif a in flags:
            opts[a] = True
            i += 1
        else:
            rest.append(a)
            i += 1
    return opts, rest, multi


def node(*argv, password=""):
    os.makedirs(STATE_DIR, mode=0o700, exist_ok=True)
    script = os.path.join(STATE_DIR, "driver.js")
    if not os.path.exists(script) or open(script).read() != JS:
        with open(script, "w") as f:
            f.write(JS)
    env = dict(ENV, NODE_PATH=NODE_PATH, PP_BASE=BASE, PP_EMAIL=EMAIL, PP_PASSWORD=password)
    r = subprocess.run([NODE, script, *argv], capture_output=True, text=True, env=env, timeout=300)
    if r.returncode:
        fail("browser: " + (r.stderr.strip() or r.stdout.strip())[-600:])
    lines = [l for l in r.stdout.splitlines() if l.startswith("{")]
    return json.loads(lines[-1]) if lines else {}


def state(role):
    return os.path.join(STATE_DIR, "state-%s.json" % role)


def do_login(role):
    r = subprocess.run([PASS, "show", ENTRY], capture_output=True, text=True, env=ENV, timeout=30)
    password = r.stdout.splitlines()[0] if r.returncode == 0 and r.stdout.strip() else ""
    if not password:
        fail("no password at pass %s: add it with `pass insert %s`" % (ENTRY, ENTRY))
    out = node("login", role, state(role), "", "", "0", "0", password=password)
    os.chmod(state(role), 0o600)
    say("logged in as %s (%s), session %s, landed on %s" % (EMAIL, role, state(role), out.get("loggedIn")))


events = {"tracked": {}}
if not sys.stdin.isatty():
    for raw in sys.stdin:
        try:
            e = json.loads(raw)
        except ValueError:
            continue
        p = e.get("payload") or {}
        if e.get("name") == "ship.tracked" and p.get("goal"):
            events["tracked"][p["goal"]] = p.get("pr")

args = sys.argv[1:]
cmd = args[0] if args else ""

if cmd == "login":
    opts, rest, _ = take(args[1:], {"--as"})
    role = opts.get("--as", "fan")
    if role not in ("fan", "creator") or rest:
        fail("usage: preprod login [--as fan|creator]", 2)
    do_login(role)

elif cmd == "shot":
    opts, rest, _ = take(args[1:], {"--as", "--out", "--wait"}, {"--anon"})
    if len(rest) != 2 or not re.match(r"^[a-z0-9][a-z0-9._-]{0,60}$", rest[1]):
        fail("usage: preprod shot <url|/path> <name> [--as fan|creator] [--anon] [--out <dir>] [--wait <ms>]", 2)
    role = opts.get("--as", "fan")
    target = rest[0] if rest[0].startswith("http") else BASE + "/" + rest[0].lstrip("/")
    if "preprod" not in target:
        fail("refusing a non-preprod URL: " + target, 2)
    out_dir = os.path.abspath(os.path.join(CWD, opts["--out"])) if opts.get("--out") else STATE_DIR + "/shots"
    os.makedirs(out_dir, exist_ok=True)
    anon = "1" if opts.get("--anon") else "0"
    if anon == "0" and not os.path.exists(state(role)):
        do_login(role)
    prefix = os.path.join(out_dir, rest[1])
    res = node("shot", role, state(role), target, prefix, opts.get("--wait", "5000"), anon)
    if res.get("bounced"):
        say("session expired (bounced to %s); logging in again" % res["bounced"])
        do_login(role)
        res = node("shot", role, state(role), target, prefix, opts.get("--wait", "5000"), anon)
        if res.get("bounced"):
            fail("still bounced to sign-in after a fresh login: %s" % res["bounced"])
    for s in res.get("shots", []):
        print("%s\t%s" % (s["file"], s["url"]), file=sys.stderr)

elif cmd == "verdict":
    opts, rest, multi = take(args[1:], {"--shot"})
    if len(rest) < 3 or rest[1] not in ("pass", "fail", "skip"):
        fail("usage: preprod verdict <goal> pass|fail|skip <evidence…> [--shot <png>]…", 2)
    goal, verdict, evidence = rest[0], rest[1], " ".join(rest[2:]).strip()
    shots = [os.path.abspath(os.path.join(CWD, s)) for s in multi.get("--shot", [])]
    missing = [s for s in shots if not os.path.isfile(s)]
    if missing:
        fail("no such screenshot: " + ", ".join(missing), 2)
    pr = events["tracked"].get(goal)
    payload = {"goal": goal, "verdict": verdict, "evidence": evidence, "shots": [os.path.basename(s) for s in shots]}
    if pr:
        payload["pr"] = pr
        number = re.search(r"/pull/(\d+)", pr).group(1)
        tok = subprocess.run([GH, "auth", "token", "--user", "wouterbeets-cmyk"], capture_output=True, text=True,
                             env=ENV).stdout.strip()
        genv = dict(ENV, GH_TOKEN=tok) if tok else ENV
        stamp = time.strftime("%Y%m%d%H%M%S")
        cells = []
        for s in shots:
            path = "pr-%s-preprod/%s-%s" % (number, stamp, os.path.basename(s))
            with open(s, "rb") as f:
                content = base64.b64encode(f.read()).decode()
            body = json.dumps({"message": "preprod verification for #%s" % number, "content": content,
                               "branch": "pr-assets"})
            for attempt in range(4):
                r = subprocess.run([GH, "api", "-X", "PUT", "repos/%s/contents/%s" % (REPO_SLUG, path), "--input", "-"],
                                   input=body, capture_output=True, text=True, env=genv, timeout=120)
                if r.returncode == 0:
                    break
                time.sleep(2 + attempt * 3)
            if r.returncode:
                say("upload of %s failed: %s" % (s, r.stderr.strip()[-200:]))
                continue
            width = 200 if "mobile" in s else 480
            cells.append('<img src="https://github.com/%s/blob/pr-assets/%s?raw=true" width="%d"/>'
                         % (REPO_SLUG, path, width))
        mark = {"pass": "✅ verified on preprod", "fail": "❌ failed on preprod", "skip": "⏭️ not testable on preprod"}[verdict]
        text = "**%s**\n\n%s" % (mark, evidence)
        if cells:
            text += "\n\n" + " ".join(cells)
        with tempfile.NamedTemporaryFile("w", suffix=".md", delete=False) as f:
            f.write(text)
        r = subprocess.run([GH, "pr", "comment", number, "--repo", REPO_SLUG, "--body-file", f.name],
                           capture_output=True, text=True, env=genv, timeout=60)
        os.unlink(f.name)
        if r.returncode == 0:
            payload["comment"] = r.stdout.strip()
        else:
            say("PR comment failed: " + r.stderr.strip()[-200:])
    print(json.dumps({"name": "preprod.verified", "payload": payload}, ensure_ascii=False, sort_keys=True))

else:
    fail("usage: preprod login|shot|verdict … (self brief preprod)", 2)
