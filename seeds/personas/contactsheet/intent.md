# contactsheet — Ana's selects

## who this is for

Ana, 29, shoots film on weekends and a mirrorless at her friends' weddings.
Her keepers live in seventeen folders named `final`, `final2`, and
`FINAL-print`, on two laptops. She does not want a photo manager — she has
one and never opens it. She wants the fifty photos she is proud of this
year in one place, tagged in her own words, visible from her phone when
someone at a dinner says "show me".

## surface

- `self run keep <photo> <tags…>` appends one `contactsheet.kept` event:
  the photo (a file — from the form's file input; the event's `photo` field
  carries its sha256) and free-word tags ("céline-wedding bw grain").
- `self run note <sha256-or-recent> <text…>` appends one
  `contactsheet.noted` event: a caption or darkroom note attached to an
  already-kept photo.
- `/contactsheet` renders the wall: every kept photo newest first as
  `<img src="/files/<sha256>/<original-name>">` inside a figure with its
  tags and any notes. Each distinct tag becomes a section anchor at the top
  of the page, so "show me the black-and-whites" is one tap on `#bw`.
- `contactsheet/roll` renders a plain chronological list — filename, date
  kept, tags, size — the inventory view, one link down from the wall.

## constraints

- The photo's bytes live in the instance's file store; the log carries only
  the hash. The projection links `/files/<sha256>/<name>` and never inlines
  image data.
- Tags are free words stored exactly as typed, lowercased for grouping.
  Nobody curates a vocabulary; a typo is just a small section.
- A `contactsheet.noted` event naming a hash no `contactsheet.kept` event
  mentions still renders, grouped under "notes without photos" on the roll —
  a lost event is worse than an untidy page.
- An empty log renders the wall with the keep form and nothing else — an
  invitation, not an error.

## anti-goals

- No thumbnails, no resizing, no EXIF parsing. The browser scales images;
  the original bytes are the only bytes.
- Not an archive. Ana's raw shoots stay wherever they are; this holds only
  what she chooses to keep, one photo at a time.
- No albums, no ordering UI. Tags and time are the whole organization.

## what good looks like

Sunday night, Ana develops a roll, scans it, keeps three frames with tags
`portra street winter`. At dinner on Thursday she opens `/contactsheet#bw`
on her phone and hands it over. In January she opens `/contactsheet/roll`,
sees ninety-one keepers for the year, and orders prints of the ones with a
note that says "print this".
