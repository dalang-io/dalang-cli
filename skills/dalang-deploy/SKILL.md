---
name: dalang-deploy
description: Deploy and manage apps on Dalang.io with the `dalang` CLI, without the dashboard. Use when the user asks to deploy, host, publish, redeploy or manage something on Dalang.io - a GitHub repository with a Dockerfile (rebuilds on every push), a database or ready-made app (PostgreSQL, Redis, WordPress, n8n...), a VPS, or extra storage - or to change an app's environment variables or port, read its build/runtime logs, or rebuild it.
---

# Deploy on Dalang.io with the `dalang` CLI

Use this when you (an AI agent) must put something online on Dalang.io, or change what is already there.
Everything below runs from a shell. You do not need the dashboard, and you never handle a payment card:
services are paid **from the account's credits**.

## 0. Preflight (do this first, once per task)

```bash
dalang version                 # must know the `app` command; see below if not
dalang auth status             # logged in?
dalang credit --json           # balance, in rupiah
```

- `dalang app --help` prints help => the CLI is recent enough. If it says `unknown command`, the installed
  binary predates `app` / `container` / `volume`: run `dalang update`, and if that does not add them, **stop and tell
  the user** that a newer CLI is needed. Do not fall back to guessing the dashboard's HTTP API.
- Not logged in => `dalang auth` prints a code the **user** enters at dalang.io/auth/cli. Ask them; do not try to
  log in yourself.
- Short of credits => `dalang credit add <thousands>` prints a payment link **for the user** to pay. Tell them the
  amount you need; do not wait silently.
- Deploying a GitHub repository needs GitHub linked to the account once (a browser login with GitHub). If
  `dalang app repos` says it is not connected, ask the user to connect it in the dashboard.

## 1. Rules that keep this safe

- Use `--json` and parse **stdout only** (progress goes to stderr). Exit code `0` = success, `1` = failure.
- Anything that **costs money or destroys data needs `--yes`** when you have no terminal; without it the command
  fails with `refusing to continue without confirmation`. Pass `--yes` only after the user has agreed to the spend.
- Tell the user the price **before** you pay, and set `--max-price <rupiah, incl. PPN>` to what they approved. The CLI
  asks the API for the price and refuses a total above `--max-price`. Never compute a price yourself.
  (Published prices at the time of writing: a GitHub app Rp 25.000/month or Rp 300.000/year plus 11% PPN, i.e.
  Rp 27.750 or Rp 333.000; container plans are quoted by the API. The API's number is the one that counts.)
- A name that matches several services is refused with their ids: pass the id.
- **Never delete or recreate** an app, container or volume unless the user asked for that specific thing.

## 2. Deploy a GitHub repository (the common case)

A GitHub app is built from the repository's **`Dockerfile`** and rebuilt by itself on every push to its branch.

**Check the repository first** (read the files; this saves a failed deployment):

1. A file named exactly `Dockerfile` in the **repository root** (a Dockerfile in a subfolder is not read).
2. The final image is **Alpine-based**: `FROM node:22-alpine`, `python:3.12-alpine`, `php:8.3-cli-alpine`, or
   `alpine:latest` as the last stage of a multi-stage build. A Debian/Ubuntu base (`node:22`, `php:8.3-apache`,
   `python:3.12-slim`) **builds but cannot start**. If it is not Alpine, change it (`apk add ...` for system
   packages) and tell the user.
3. `EXPOSE <port>` with the port the app really listens on, and the app listens on `0.0.0.0` (not only `127.0.0.1`).
4. The app keeps no data on its own disk: the container is **stateless** and rebuilt on every build.
5. Everything is pushed to the branch you will deploy.

**Deploy:**

```bash
dalang app repos --search myrepo --json
dalang app create --repo owner/name --env KEY=VALUE --months 1 --max-price 40000 --yes --json
# -> {"id":"...","status":"running","url":"https://name-xxxxxxxx.svc.dalang.io","branch":"main", ...}
```

`create` checks the repository can be built (Dockerfile, port) **before** it creates or charges anything, pays the
quoted price, waits for the build, and returns the URL. If the Dockerfile has no `EXPOSE`, pass `--port N`.

**Verify:** `curl -sS -o /dev/null -w "%{http_code}\n" <url>` should print 200 (or the app's normal code).
A deploy is not done until you have looked at the URL.

**Update:** just `git push` to the app's branch; it rebuilds by itself (needs an active subscription). To build
without a push: `dalang app rebuild <name>`.

## 3. Operate an app

```bash
dalang app list --json
dalang app info <name> --json                    # status, url, port, subscription
dalang app logs <name> --build                   # newest build log: read this when a build fails
dalang app logs <name> --lines 200               # runtime log
dalang app env <name> --set KEY=VALUE --unset OLD --rebuild
dalang app port <name> 8000 --rebuild            # change the port traffic goes to
dalang app extend <name> --months 1 --yes        # pay for another period
dalang shell <name>                              # interactive; for scripts prefer logs
```

- **Environment variables and the port apply from the NEXT build.** Saving does not touch the running container,
  so follow a change with `--rebuild` (or `dalang app rebuild`).
- The port is read from `EXPOSE` when the app is created and then follows a changed `EXPOSE` unless a port was set
  by hand. If an app is "running" but answers 502, the port is almost always the cause: read the build log for a
  `WARNING:` about it, then `dalang app port`.
- Environment values are secrets: do not print them back to the user or into logs (`app env` hides them unless
  `--reveal`).

## 4. Other services

```bash
dalang container templates --json                # 20 types, default versions, what must be set
dalang container create --type postgresql --name orders-db --env POSTGRES_PASSWORD=... --max-price 70000 --yes --json
dalang container info orders-db --json           # host, port, connection_string
dalang volume create --name data --size 10 --yes # extra storage for a VPS
dalang service create --name web --cpu 2 --ram 2G --storage 20G --image ubuntu:24.04 --yes --json   # a VPS
```

`container create` refuses what a template cannot start without (required `--env`, the database an app is bundled
with) before it charges. Put an app's data in a container database, not on the app's own disk.

## 5. When something fails

| You see | Cause | Do |
|---|---|---|
| `has no Dockerfile in its root` | missing/misnamed/in a subfolder | add `Dockerfile` to the root, push, retry |
| build `failed` | error in the Dockerfile or the code | `dalang app logs <name> --build`, fix, push |
| build ok, app never `running` | non-Alpine base image | switch to the `-alpine` variant |
| `running` but 502 | wrong port, or listening only on 127.0.0.1 | `dalang app port`, listen on `0.0.0.0` |
| a change has no effect | not rebuilt yet | `--rebuild` / `dalang app rebuild` |
| `not enough credits` | balance too low | tell the user the shortfall; `dalang credit add` |
| `above --max-price` | the API quoted more than approved | report the quoted total to the user and ask |
| `timed out ... waiting` | slow build | `dalang app info <name>` later; it may still finish |

## 6. Full guide for the user

If the user wants to read along: https://dalang.io/blog/panduan-app-hosting-github-dockerfile (Indonesian).
