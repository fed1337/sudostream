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

- fix playback issues and enhance UX (#13)

## v0.2.0 (2026-10-14)

### Feat

- skip intro, audio downmixing, user langauge preferences, cancel active jobs
- skip intro chapter button and intro audio fingerprint detection job
- audio downmix admin settings
- user audio language preferences
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
