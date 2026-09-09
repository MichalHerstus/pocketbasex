# Login flow with dual collection check

**Session ID:** ses_05c50efd2ffeNciPLMdKvDeAl7
**Created:** 7/27/2026, 3:06:26 PM
**Updated:** 7/27/2026, 3:13:28 PM

---

## User

Change the way how /login works. Check the credentials against both Users and Superusers collections. If logged-in users is regular user, i.e. is in Users colection, display /app endpoint. If it is superuser, displa /pbx-setup endpoint.

---

## Assistant

**Tool: read**

---

## Assistant

---

## User

Change the way how /login works. Check the credentials against both Users and Superusers collections. If logged-in users is regular user, i.e. is in Users colection, display /app endpoint. If it is superuser, displa /pbx-setup endpoint.

---

## Assistant

---

## User

Change the way how /login works. Check the credentials against both Users and Superusers collections. If logged-in users is regular user, i.e. is in Users colection, display /app endpoint. If it is superuser, displa /pbx-setup endpoint.

---

## Assistant

**Tool: edit**

---

## Assistant





**Tool: bash**

---

## Assistant

Done. `handleLoginPost` now:

1. First tries `FindAuthRecordByEmail("users", ...)` — if password matches, redirects to `/app`
2. If that fails, tries `FindAuthRecordByEmail("superusers", ...)` — if password matches, redirects to `/pbx-setup`
3. If both fail, returns "Invalid name or password" error

---

## User

the login with superuser credential does not work

---

## Assistant

Let me investigate the PocketBase superusers collection name.



**Tool: task**

---

## Assistant

The collection is named `_superusers` (with underscore), not `superusers`.



**Tool: edit**

---

## Assistant





**Tool: bash**

---

## Assistant

Build passes. The collection name is `_superusers` (not `superusers`) — fixed at `main.go:245`.

---

