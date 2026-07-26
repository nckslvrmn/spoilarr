# spoilarr

Self-hosted Plex spoiler protection. It hides the stuff that spoils you before you've watched: blurs or replaces episode and season artwork, clears or rewrites descriptions, and masks episode titles. It does this automatically as new episodes show up, and it's fully reversible whenever you want the originals back.

## Why I built this

I was using Tautulli for exactly one thing: blurring the thumbnails on Survivor episodes so the artwork and titles didn't spoil who's still in the game. Then I rebuilt my Plex and really didn't feel like standing all of Tautulli back up just for that one job. So I built spoilarr instead, which does the thing I actually wanted and nothing I didn't.

It's built for the *arr homelab: a single static Go binary, SQLite plus a folder of images, no external services to babysit.

## How it works

1. Sign in to Plex once with the PIN flow and pick your server.
2. Browse a library and protect a whole show, or just specific seasons. Movies work too.
3. Per show, pick a spoiler profile (blur / generated card / solid color / leave it) plus what to do with descriptions and episode titles, and a reveal policy.
4. New episodes get protected on their own. Instantly if you set up the Plex webhook, and the background poller catches anything the webhook missed every few minutes.
5. Whatever spoilarr overwrites, it locks, so a Plex metadata refresh can't quietly undo it. When you reveal, it puts the exact originals back and unlocks everything.

The reveal policies are the part I care about most. Mine is set to `never`, so protected episodes stay hidden until I manually reveal them. But you can also have things reveal once you've watched them, or a set number of days after they aired, if that's more your speed.

## Run it

```sh
# local
go run ./cmd/spoilarr      # http://localhost:8484, data in ./data

# container
docker compose up --build  # same thing, data in ./data
```

## Config

Almost everything lives in the web UI and gets stored in SQLite. The only stuff that's environment based:

| Var                       | Default   | What it does                                          |
| ------------------------- | --------- | ----------------------------------------------------- |
| `SPOILARR_ADDR`           | `:8484`   | HTTP listen address                                   |
| `SPOILARR_DATA_DIR`       | `./data`  | Where `spoilarr.db` and `images/` live                |
| `SPOILARR_POLL_INTERVAL`  | `5m`      | How often the poller reconciles (minimum `1m`)        |
| `SPOILARR_PUBLIC_URL`     | *(unset)* | Only needed if Plex can't reach spoilarr at the same address your browser uses |
| `SPOILARR_WEBHOOK_SECRET` | *(unset)* | If set, it's required as `?secret=` on the webhook URL |
| `SPOILARR_LOG_LEVEL`      | `info`    | `debug` / `info` / `warn` / `error`                   |

## The webhook (optional)

If you want instant protection instead of within-a-few-minutes, drop the webhook URL from the setup screen into Plex under Settings → Webhooks. It needs Plex Pass.

Heads up: this URL just has to be reachable by your Plex server, not the public internet. Plex and spoilarr are on the same network, so it's a local address like `http://spoilarr:8484/plex/webhook`. The setup screen fills it in automatically based on however you're hitting spoilarr, so you probably don't have to touch `SPOILARR_PUBLIC_URL` at all. Only set that if Plex lives on a different host than the address your browser uses.

And honestly the webhook is totally optional. The poller already covers you, so skip it if you don't care about the difference between instant and a few minutes.
