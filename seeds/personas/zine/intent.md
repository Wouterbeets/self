# zine — Lena's page

## who this is for

Lena, 15. She had a Tumblr for eight months, an Instagram she's suspicious
of, and a notes app with forty-three unfinished drafts. She writes about
the bands she loves, her sourdough starter (named Clive), and being
fifteen. She wants a page that is hers: no likes, no follower count, no
algorithm deciding whether her Tuesday matters.

## surface

- `self run post <title> <body…>` appends one `zine.post` event.
- `self run mood <word>` appends one `zine.mood` event — a single word,
  as often as she likes.
- `self run retract <title>` appends one `zine.retracted` event naming a
  post's title, for the entries fifteen-year-olds regret by Thursday.
- `/zine` renders posts newest first, skipping any post whose title has a
  later retraction. Between posts, moods appear as a small run of words —
  a weather report of the days when she didn't write.

## constraints

- Retraction hides; it never deletes. The page must say so, once, quietly,
  in its footer ("retracted posts leave the page, not the log") — Lena
  deserves to know exactly what this machine remembers, because the ones
  that lied to her about that are why she's here.
- Post bodies render with line breaks preserved and all HTML escaped; a
  pasted mess of emoji and lyrics is inert and intact.
- An empty log renders the two forms and nothing broken — a blank first
  page is an invitation, not an error.

## anti-goals

- No counts of any kind: no likes, views, streaks, or word counts. Nothing
  on the page may make writing feel like scoring.
- No feed of anyone else. One author. The internet already has the other
  thing.

## what good looks like

Lena posts at 22:40 on a school night, titles it "clive rose and so did
my hopes", and adds a mood: `okay`. Two weeks later she retracts a post
about a friend and the page forgets it gracefully. Years from now the log
still holds every word — hers to keep, replay, or leave sealed.
