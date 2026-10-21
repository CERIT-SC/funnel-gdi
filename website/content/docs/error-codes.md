---
gdi: true
title: Error codes
menu:
  main:
    weight: 15
---

# Error codes

Errors the Funnel API returns to a client start with a code when they are
listed on this page:

```
FUNNEL-<AREA>-<NNN>: <message>
```

The code identifies the error independently of the message text, so scripts
can match it and the text can be translated. The HTTP status code (or gRPC
status code) of the response is unchanged and follows the TES specification.
When you ask for help ([Getting help](/gdi/deployment/#getting-help)),
include the code.

The messages are kept outside the code, in
[`errcodes/messages.en.yaml`](https://github.com/CERIT-SC/funnel-gdi/blob/master/errcodes/messages.en.yaml).
Only English ships. A translation is a copy of that file named
`messages.<language>.yaml` with the same codes and translated messages;
selecting a language is not implemented yet.

| Area | Errors about |
|---|---|
| `GEN` | Funnel itself |

## Codes

| Code | HTTP status | Message | Meaning and what to do |
|---|---|---|---|
| `FUNNEL-GEN-000` | 500 | internal error (unknown code …) | Funnel used an error code that is not in its catalog. This is a bug: please [report it](/gdi/deployment/#getting-help). |
