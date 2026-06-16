# booky

booky is a small self-hosted calendar for booking a shared place or resource.

I built it for one very specific job: sharing the schedule for a family home. The
real calendar lives in Nextcloud, and booky gives people a simple public page
where they can see what is already booked and add their own stay.

That is the whole idea. booky is not trying to become a full scheduling system.
It is a Go HTTP server with the static frontend assets embedded in the binary.
It talks to one CalDAV calendar, stores bookings as all-day events, and lets the
calendar backend remain the source of truth. In that sense it is mostly a nicer,
dumber proxy in front of a real calendar.

## What it is

- A public booking calendar for one shared thing
- A single Go binary once built
- A tiny web UI backed by a CalDAV calendar
- No local database
- No account system
- No app-level authentication

Nextcloud Calendar is the setup I use and the one documented here. Other CalDAV
servers may work if they expose a direct calendar collection URL and support the
same basic operations, but Nextcloud is the known path.

## What it is not

booky is public and unauthenticated by design. It does not provide users,
sessions, permissions, invites, approval flows, or access-control rules.

If the calendar should not be open to anyone who has the URL, put authentication
in front of booky.

## Requirements

- Go
- Bun
- `rsvg-convert` from librsvg, used to generate installable app PNG icons at build time
- A Nextcloud calendar
- A Nextcloud app password for the user that owns or can edit that calendar

## Nextcloud setup

Create or choose a dedicated calendar in Nextcloud Calendar. Then copy the exact
CalDAV calendar collection URL.

The URL must point to the calendar collection itself and must end with `/`, for
example:

```text
https://cloud.example.com/remote.php/dav/calendars/family-house/vacation-house/
```

Watch the calendar path casing. Nextcloud may lowercase it, so a calendar named
`Booking` can have a URL ending in `/booking/`.

Use a Nextcloud app password instead of your account password.

## Configuration

Copy the example config and edit it:

```sh
cp config-example.yaml config.yaml
```

```yaml
listen_addr: ":8080"
app_title: "Vacation House"
public_path: ""

caldav:
  url: "https://cloud.example.com/remote.php/dav/calendars/family-house/vacation-house/"
  user: "family-house"
  pass: "app-password"
```

`config.yaml` contains credentials and is ignored by git.

`public_path` is optional. Leave it empty for local development at `/`. If you
publish booky on the open internet without real authentication, you can set it to
a long unguessable path such as `/REPLACE_WITH_LONG_RANDOM_PATH`. That hides the
app from casual discovery, but it is not authentication.

## Development

Run the app:

```sh
sh scripts/booky.sh run
```

Then open `http://localhost:8080/`.

Run tests:

```sh
sh scripts/booky.sh test
```

## Build

Build a release-ready Linux static binary:

```sh
sh scripts/booky.sh build
```

The binary is written to:

```text
bin/booky
```

Run it with:

```sh
bin/booky -config config.yaml
```

The frontend must be built before the Go binary is built. The `build` command
does this automatically.

## Publishing behind nginx

booky can be exposed only under a long unguessable URL path. This can be useful
for a public, unauthenticated deployment, but it is still only obscurity. Add
nginx auth or another access-control layer if you need real access control.

Use placeholder values in the examples below:

- `booky.example.com`: your public hostname
- `/REPLACE_WITH_LONG_RANDOM_PATH`: a long random path, for example a UUID
- `127.0.0.1:8080`: the local address where booky listens

Set `public_path` in `config.yaml` to the same path nginx exposes:

```yaml
listen_addr: "127.0.0.1:8080"
app_title: "Vacation House"
public_path: "/REPLACE_WITH_LONG_RANDOM_PATH"
```

Example nginx site:

```nginx
server {
    listen 80;
    server_name booky.example.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl;
    server_name booky.example.com;

    ssl_certificate /etc/letsencrypt/live/booky.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/booky.example.com/privkey.pem;

    location /REPLACE_WITH_LONG_RANDOM_PATH {
        proxy_pass http://127.0.0.1:8080;
    }

    location / {
        return 404;
    }
}
```

Keep `proxy_pass` without a trailing slash. booky needs to receive the full
public path so it can enforce `public_path` and generate matching asset and API
URLs.

Do not expose `config.yaml`.
