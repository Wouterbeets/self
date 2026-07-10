# zine — Lena's page

## who this is for

Lena, 15. She had a Tumblr for eight months, an Instagram she's suspicious
of, and a notes app with forty-three unfinished drafts. She writes about
the bands she loves, her sourdough starter (named Clive), and being
fifteen. She wants a page that is hers: no likes, no follower count, no
algorithm deciding whether her Tuesday matters. And she has discovered, in
a school library book, what a zine actually is — photocopied, stapled,
handed to people — and wants that too.

## surface

- `self run post <title> <body…>` appends one `zine.post` event. An
  optional image rides along (Clive at hour six, a gig ticket, a drawing
  photographed on her desk), its hash in the event.
- `self run mood <word>` appends one `zine.mood` event — a single word,
  as often as she likes.
- `self run retract <title>` appends one `zine.retracted` event naming a
  post's title, for the entries fifteen-year-olds regret by Thursday.
- `self run issue <name> <post-titles…>` makes an actual zine: the named
  posts, their images, their moods between them, gathered into one
  complete standalone HTML file — styled for print, hers to photocopy —
  deposited into the store and recorded by one `zine.issue` event carrying
  its hash and the chosen titles.
- `self run print <issue-name> <copies>` sends an issue to the household
  printer (plain `lp`, nothing configured beyond the printer being there)
  and appends one `zine.printed` event: which issue, how many copies, what
  the printer said. The library photocopier keeps its job for big runs;
  Tuesday-night runs of five happen from her bed.
- `/zine` renders posts newest first, skipping any post whose title has a
  later retraction. Between posts, moods appear as a small run of words —
  a weather report of the days when she didn't write. At the bottom, the
  issues: "issue two — six posts, made in march", each a link to its file.

## constraints

- Retraction hides; it never deletes. The page must say so, once, quietly,
  in its footer ("retracted posts leave the page, not the log") — Lena
  deserves to know exactly what this machine remembers, because the ones
  that lied to her about that are why she's here.
- An issue is frozen the moment it is made — a file, not a page. Retracting
  a post later removes it from `/zine`, not from an issue already made,
  same as paper: the page must say that too, next to the issues. If she
  regrets an issue, she makes a new one and stops printing the old.
- The issue file is a complete document and may carry its own styles — it
  is user content served from the store, not a projection, so the shell's
  no-CSS rule does not bind it. Black on white, readable photocopied, no
  scripts.
- A post named in `issue` that doesn't exist (or is retracted) is skipped,
  and the issue's event records what was skipped — never an error that eats
  the zine.
- Post bodies render with line breaks preserved and all HTML escaped; a
  pasted mess of emoji and lyrics is inert and intact. Images render as
  images, never resized, never filtered.
- An empty log renders the two forms and nothing broken — a blank first
  page is an invitation, not an error.

## anti-goals

- No counts of any kind: no likes, views, streaks, or word counts. Nothing
  on the page may make writing feel like scoring.
- No feed of anyone else. One author. The internet already has the other
  thing.
- The issue file is for paper, not for platforms: no share buttons, no
  embeds, no "post this". She hands it to people. That's the medium.

## what good looks like

Lena posts at 22:40 on a school night, titles it "clive rose and so did
my hopes", attaches the photo of the jar, and adds a mood: `okay`. In
March she picks six posts, runs `issue`, and hears the printer downstairs
wake up — five copies, stapled crooked at the kitchen table, the other
twelve done at the library at lunch, gone by Friday. Two
weeks later she retracts a post about a friend and the page forgets it
gracefully; the four paper copies in other people's bags are a lesson
about paper, not a bug. Years from now the log still holds every word and
every issue — hers to keep, replay, or leave sealed.
