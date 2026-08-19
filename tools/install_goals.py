#!/usr/bin/env python3
import json
import os
import subprocess

COMMAND = r'''#!/usr/bin/env python3
import json, os, sys, shlex

args = sys.argv[1:]
if not args:
    print("usage: goal create|update|milestone|kpi|close ...", file=sys.stderr)
    raise SystemExit(2)
kind = args[0]
if kind == "create" and len(args) >= 4:
    title = args[2]
    outcome = args[3]
    project = args[4] if len(args) > 4 else ""
    parent = args[5] if len(args) > 5 else ""
    print(json.dumps({"name":"goal.created","payload":{"goal":args[1],"title":title,"outcome":outcome,"project":project,"parent":parent,"status":"active"}}))
elif kind == "update" and len(args) >= 3:
    print(json.dumps({"name":"goal.updated","payload":{"goal":args[1],"note":" ".join(args[2:])}}))
elif kind == "progress" and len(args) >= 3:
    print(json.dumps({"name":"goal.progress","payload":{"goal":args[1],"report":" ".join(args[2:])}}))
elif kind == "context" and len(args) >= 2:
    goal = args[1]
    events=[]
    for line in sys.stdin:
        try: events.append(json.loads(line))
        except: pass
    rows=[e.get("payload",{}) for e in events if isinstance(e.get("payload"),dict) and e.get("payload",{}).get("goal")==goal]
    if not rows:
        print(json.dumps({"name":"goal.context","payload":{"goal":goal,"found":False,"text":"No goal found: %s" % goal}}))
    else:
        first=next((r for r in rows if r.get("title")), rows[0])
        text="GOAL: %s\nTITLE: %s\nOUTCOME: %s\nPROJECT: %s\nPARENT: %s" % (goal, first.get("title",""), first.get("outcome",""), first.get("project",""), first.get("parent",""))
        text += "\n\nRECENT STATE:\n" + "\n".join("- %s" % json.dumps(row, sort_keys=True) for row in rows[-12:])
        text += "\n\nOPERATING CONTRACT:\nWork only on this goal in the current worktree. Record meaningful updates with: SELF_HOME=$HOME/.self-goals self run goal progress %s '<change; evidence; blockers; next action>'. Before stopping, leave a handoff in that progress report. Do not claim completion until code is pushed and the worktree is removed." % goal
        print(json.dumps({"name":"goal.context","payload":{"goal":goal,"found":True,"text":text}}))
elif kind == "project" and len(args) >= 3:
    if args[1].lower() in ("delete", "remove"):
        print(json.dumps({"name":"project.removed","payload":{"project":args[2],"reason":" ".join(args[3:]) or "removed"}}))
    else:
        print(json.dumps({"name":"project.registered","payload":{"project":args[1],"repo":args[2],"default_base":args[3] if len(args) > 3 else ""}}))
elif kind == "complete" and len(args) >= 4:
    print(json.dumps({"name":"goal.completion.requested","payload":{"goal":args[1],"branch":args[2],"commit":args[3],"worktree_removed":args[4].lower() == "true" if len(args) > 4 else False,"note":" ".join(args[5:])}}))
elif kind == "milestone" and len(args) >= 4:
    print(json.dumps({"name":"goal.milestone","payload":{"goal":args[1],"milestone":args[2],"status":" ".join(args[3:])}}))
elif kind == "kpi" and len(args) >= 5:
    print(json.dumps({"name":"goal.kpi","payload":{"goal":args[1],"name":args[2],"value":args[3],"target":" ".join(args[4:])}}))
elif kind == "close" and len(args) >= 2:
    print(json.dumps({"name":"goal.updated","payload":{"goal":args[1],"status":"closed","note":" ".join(args[2:]) or "closed"}}))
elif kind in ("delete", "remove") and len(args) >= 2:
    print(json.dumps({"name":"goal.removed","payload":{"goal":args[1],"reason":" ".join(args[2:]) or "removed"}}))
else:
    print("invalid goal command", file=sys.stderr)
    raise SystemExit(2)
'''

DISPATCH = r'''#!/usr/bin/env python3
import json, os, re, subprocess, sys, time

args = sys.argv[1:]
if len(args) < 4:
    print("usage: dispatch <goal> <name> <kind> <repo> [prompt]", file=sys.stderr)
    raise SystemExit(2)
goal, name, kind, repo = args[:4]
requested_name = name
def slug(value):
    value = re.sub(r"[^a-zA-Z0-9_-]+", "-", value).strip("-").lower()
    return value[:32] or "agent"
name = slug(name)
branch = "goal/" + slug(goal) + "/" + name
cwd = repo
prompt = " ".join(args[4:]) or "Inspect the goal context and continue the assigned work."
prompt = ("You are an agent working in a Herdr-created worktree. Do not wait for injected context. First run: SELF_HOME=$HOME/.self-goals self run goal context %s. Read that mission, inspect the repository, and execute the useful next step. Use the shared self-goals instance for progress reports. If the work decomposes, create recursive subgoals and report their identifiers.\n\nAssignment:\n" % goal) + prompt
base = {"goal":goal,"agent":name,"requested_agent":requested_name,"kind":kind,"repo":repo,"branch":branch,"prompt":prompt}
if os.environ.get("HERDR_ENV") != "1":
    base["error"] = "HERDR_ENV is not 1; dispatch must run from a Herdr-managed environment"
    print(json.dumps({"name":"agent.requested","payload":base}))
    print(json.dumps({"name":"agent.failed","payload":base}))
    raise SystemExit(0)
try:
    worktree = subprocess.run(["herdr","worktree","create","--cwd",repo,"--branch",branch,"--no-focus"], text=True, capture_output=True, check=True)
    worktree_result = json.loads(worktree.stdout).get("result", {})
    cwd = worktree_result.get("worktree", {}).get("path", repo)
    pane = worktree_result.get("root_pane", {}).get("pane_id")
    workspace = worktree_result.get("workspace", {}).get("workspace_id")
    if not pane:
        raise RuntimeError("herdr worktree creation returned no root pane")
    start = None
    for attempt in range(6):
        candidate = subprocess.run(["herdr","agent","start",name,"--kind",kind,"--pane",pane], text=True, capture_output=True)
        if candidate.returncode == 0:
            start = candidate
            break
        start = candidate
        time.sleep(1)
    if start.returncode != 0:
        detail = (start.stderr or start.stdout or "agent start failed").strip()
        raise RuntimeError(detail)
    result = json.loads(start.stdout).get("result", {})
    payload = dict(base, worktree=cwd, pane=pane, workspace=workspace, herdr=result)
    print(json.dumps({"name":"agent.requested","payload":base}))
    print(json.dumps({"name":"agent.started","payload":payload}))
    prompt = prompt + "\n\nShared goal instance: /Users/wouterbeets/.self-goals. Worktree: " + cwd + ". Repository: " + repo + "."
    subprocess.run(["herdr","agent","prompt",name,prompt], check=False,
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
except Exception as exc:
    base["error"] = str(exc)
    if "cwd" in locals():
        base["worktree"] = cwd
    if "workspace" in locals():
        base["workspace"] = workspace
    print(json.dumps({"name":"agent.requested","payload":base}))
    print(json.dumps({"name":"agent.failed","payload":base}))
'''

def projector(name, description, consumes, body):
    return {"name":"projector.declared","payload":{"name":name,"description":description,"consumes":consumes}}, {"name":"script.authored","payload":{"type":"projector","name":name,"script":body}}

def emit(events):
    data = "\n".join(json.dumps(e, separators=(",", ":")) for e in events) + "\n"
    subprocess.run(["self"], input=data, text=True, check=True, env=os.environ)

events = [
 {"name":"identity","payload":{"text":"I am a goal-oriented work control room. Goals define outcomes; agents execute bounded assignments through Herdr; milestones, KPI observations, and evidence keep progress honest. The former ~/.self-work instance is preserved as an archive."}},
  {"name":"command.declared","payload":{"name":"goal","description":"Create, remove, inspect, and update projects, goals, progress reports, milestones, KPI observations, and status.","params":{"operation":"string","goal":"string","title":"string","outcome":"string","project":"string","parent":"string"},"event":{"name":"project.registered|project.removed|goal.created|goal.removed|goal.updated|goal.progress|goal.milestone|goal.kpi","fields":{}}}},
 {"name":"script.authored","payload":{"type":"command","name":"goal","script":COMMAND}},
  {"name":"command.declared","payload":{"name":"dispatch","description":"Request and start a Herdr-managed agent for a project goal in a new worktree.","params":{"goal":"string","name":"string","kind":"string","repo":"string","prompt":"string"},"event":{"name":"agent.started","fields":{}}}},
 {"name":"script.authored","payload":{"type":"command","name":"dispatch","script":DISPATCH}},
]

def page(title, inner):
    return f'''#!/usr/bin/env python3\nimport html,json,sys\ne=[]\nfor line in sys.stdin:\n try:\n  x=json.loads(line); e.append(x)\n except: pass\ndef p(x): return x.get("payload") if isinstance(x.get("payload"),dict) else {{}}\ndef q(x): return html.escape(str(x if x is not None else ""),quote=True)\nprint("<h1>{title}</h1>")\n{inner}\n'''

events += list(projector("goals", "Goal workspace with projects, recursive hierarchy, progress, milestones, KPIs, and dispatch controls.", ["project.registered","project.removed","goal.created","goal.removed","goal.updated","goal.progress","goal.milestone","goal.kpi"], page("Goals", '''projects={}; goals={}; removed_projects=set(); removed_goals=set()
for x in e:
 a=p(x); name=x.get("name")
 if name=="project.registered": projects[a.get("project")]=a; removed_projects.discard(a.get("project"))
 elif name=="project.removed": removed_projects.add(a.get("project"))
 elif name=="goal.removed": removed_goals.add(a.get("goal"))
 elif name=="goal.created": goals.setdefault(a.get("goal"),[]).append(a)
 elif name in ("goal.updated","goal.progress","goal.milestone","goal.kpi") and a.get("goal"): goals.setdefault(a.get("goal"),[]).append(a)
for key in list(projects):
 if key in removed_projects: del projects[key]
for key in list(goals):
 if key in removed_goals: del goals[key]
if not goals: print("<p>Create a project, then create its first goal from <a href=/projects>/projects</a>.</p>")
def latest(goal): return goals[goal][-1] if goal in goals else {}
def children(parent): return [g for g, rows in goals.items() if any(a.get("parent")==parent for a in rows)]
def render(goal, depth=0):
 rows=goals[goal]; a=latest(goal); project=next((item.get("project") for item in rows if item.get("project")),""); pad="&nbsp;"*min(depth,6)*4
 print("<article><h2>"+pad+q(goal)+"</h2><p><span class=tag>"+q(a.get("status","active"))+"</span> "+q(a.get("title",a.get("outcome",a.get("note",""))))+"</p>")
 if project: print("<p class=muted>project <a href=/projects>"+q(project)+"</a></p>")
 print("<ul>")
 for item in rows[-12:]:
  kind=item.get("status",item.get("milestone",item.get("name","progress")))
  text=item.get("report",item.get("note",item.get("outcome",item.get("value",item.get("target","")))))
  print("<li><code>"+q(kind)+"</code> "+q(text)+"</li>")
 repo=projects.get(project,{}).get("repo","")
 dispatch="<form method=post action=/run/dispatch><input type=hidden name=goal value='"+q(goal)+"'><input name=name placeholder=agent required><input name=kind value=opencode><input type=hidden name=repo value='"+q(repo)+"'><input name=prompt placeholder=assignment required><button>Dispatch in worktree</button></form>" if repo else "<p class=card>Dispatch unavailable: register this goal's project repository on <a href=/projects>/projects</a>.</p>"
 print("</ul><form method=post action=/run/goal><input type=hidden name=operation value=delete><input type=hidden name=goal value="+q(goal)+"><input name=values placeholder=reason><button class=danger>Remove goal</button></form><details><summary>Update goal</summary><form method=post action=/run/goal><input type=hidden name=operation value=progress><input type=hidden name=goal value="+q(goal)+"><textarea name=values required placeholder='What changed, evidence, blockers, next action'></textarea><button>Record progress</button></form>"+dispatch+"</details></article>")
 for child in children(goal): render(child, depth+1)
roots=[g for g in goals if not any(a.get("parent") for a in goals[g])]
for g in roots: render(g)
print("<h2>New goal</h2><p class=muted>Goals belong to a project. To add a repository, use <a href=/projects>/projects</a>.</p><form method=post action=/run/goal><input type=hidden name=operation value=create><label>Identifier <input name=goal placeholder=goal-id required></label><label>Title <input name=title placeholder='Short outcome name' required></label><label>Outcome <textarea name=outcome placeholder='What does success look like?' required></textarea></label><label>Project <select name=project><option value=''>Choose a project</option>"+"".join("<option value='"+q(k)+"'>"+q(k)+"</option>" for k in projects)+"</select></label><label>Parent goal <select name=parent><option value=''>Top-level goal</option>"+"".join("<option value='"+q(k)+"'>"+q(k)+"</option>" for k in goals)+"</select></label><button>Create goal</button></form>")''')))
events += list(projector("agents", "Agent assignments and observed Herdr lifecycle for each goal.", ["agent.requested","agent.started","agent.progress","agent.ended","agent.failed"], page("Agents", '''if not e: print("<p>No agent assignments yet.</p>")\nfor x in reversed(e):\n a=p(x); print("<article><h2>"+q(a.get("agent","unknown"))+"</h2><p><span class=tag>"+q(x.get("name"))+"</span> goal: "+q(a.get("goal"))+"</p><p>"+q(a.get("prompt",a.get("error","")))+"</p></article>")\nprint("<form method=post action=/run/dispatch><input name=goal placeholder=goal required><input name=name placeholder=agent required><input name=kind value=opencode><input name=cwd placeholder=/path required><input name=prompt placeholder=assignment><button>Dispatch via Herdr</button></form>")''')))
events += list(projector("projects", "Projects with repositories and their goal trees.", ["project.registered","project.removed","goal.created","goal.updated","goal.progress","goal.milestone","goal.kpi"], page("Projects", '''projects={}; goals={}; removed=set()
for x in e:
 a=p(x)
 if x.get("name")=="project.registered": projects[a.get("project")]=a; removed.discard(a.get("project"))
 if x.get("name")=="project.removed": removed.add(a.get("project"))
 if a.get("goal"): goals.setdefault(a.get("goal"),[]).append(a)
for key in list(projects):
 if key in removed: del projects[key]
if not projects: print("<p>Register a repository to create the first project.</p>")
for key, project in projects.items():
 print("<article><h2>"+q(key)+"</h2><p><code>"+q(project.get("repo"))+"</code></p>")
 print("<h3>Goals</h3><ul>")
 for goal, rows in goals.items():
  if any(a.get("project")==key for a in rows): print("<li><a href=/goals>"+q(goal)+"</a>: "+q(rows[-1].get("title",rows[-1].get("note",rows[-1].get("status",""))))+"</li>")
 print("</ul><form method=post action=/run/goal><input type=hidden name=operation value=create><input name=goal placeholder=goal required><input name=values placeholder='title outcome --project "+q(key)+"' required><button>Create goal</button></form>")
 print("<form method=post action=/run/goal><input type=hidden name=operation value=project><input type=hidden name=goal value=delete><input type=hidden name=values value="+q(key)+"><button class=danger>Remove project</button></form>")
 print("<form method=post action=/run/dispatch><input name=goal placeholder=goal required><input name=name placeholder=agent required><input name=kind value=opencode><input type=hidden name=repo value="+q(project.get("repo"))+"><input name=prompt placeholder=assignment><button>Dispatch agent</button></form></article>")
print("<h2>Register project</h2><form method=post action=/run/goal><input type=hidden name=operation value=project><input name=goal placeholder=project-key required><input name=values placeholder=repository-path required><button>Add project</button></form>")''')))
events += list(projector("metrics", "KPI observations grouped by goal, with latest value and target.", ["goal.kpi"], page("Metrics", '''rows=[p(x) for x in e]
if not rows: print("<p>No KPI observations yet.</p>")
for a in rows: print("<article><strong>"+q(a.get("goal"))+" / "+q(a.get("name"))+"</strong>: "+q(a.get("value"))+" <span class=muted>target "+q(a.get("target"))+"</span></article>")''')))
events += list(projector("brief", "Cold-start control room combining goals, agents, and KPI observations.", ["goal.created","goal.updated","goal.milestone","goal.kpi","agent.requested","agent.started","agent.progress","agent.ended","agent.failed"], page("Control Room", '''print("<p><a href=/goals>Goals</a> · <a href=/agents>Agents</a> · <a href=/metrics>Metrics</a></p>")\nprint("<p>"+q(len([x for x in e if x.get("name")=="goal.created"]))+" goals · "+q(len([x for x in e if x.get("name")=="agent.started"]))+" active-start receipts · "+q(len([x for x in e if x.get("name")=="goal.kpi"]))+" KPI observations</p>")''')))
emit(events)
