## v0.5.0 (2026-11-07)

### Feat

- add link to health check API from version string in app footer

### Fix

- strip trailing release group tags from series titles

## v0.4.1 (2026-11-06)

### Fix

- increase custom player buttons size

## v0.4.0 (2026-11-05)

### Feat

- update recycle bin retention logic (#14)
- add favicon and update app icon
- move recycle-bin retention to settings with UTC midnight auto-purge

### Fix

- hey-api not finding the openapi schema
- continuous HLS encoder with pause throttle and original-first Auto quality

## v0.3.0 (2026-11-03)

### Feat

- Improved form validation and error handling throughout the application
- Added translations for backend error messages
- Remember sidebar states across sessions

### Fix

- fix playback issues and enhance UX (#13)

### Refactor

- use native shadcn components

## v0.2.0 (2026-10-14)

### Feat

- skip intro chapter button and intro audio fingerprint detection job
- stereo downmixing algorithms
- user langauge preferences
- cancel maintenance jobs and show execution history on schedule
- inject build revision into binary

### Fix

- player episode picker S/E specials handling
- stop seek-back on watch progress save
- favourite button not working
- harden SMTP against content injection
- validate HLS access_token before reflection

### Refactor

- catalog denorm for O(result) list/show/season queries

## v0.1.0 (2026-09-22)

### Feat

- initial public release
