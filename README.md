# booky

booky is a tiny self-hosted booking app for a vacation house. It stores bookings as all-day events in one configured Nextcloud CalDAV calendar.

The server is written in Go. The browser app is TypeScript built with Bun, then embedded into the final Go binary. There is no database: Nextcloud Calendar is the source of truth.

booky is public and unauthenticated. It does not provide app-level auth, accounts, sessions, or access-control rules. If access should be restricted, put authentication in front of booky at the reverse proxy.

## Requirements

- Go
- Bun
- `rsvg-convert` from librsvg, used to generate installable app PNG icons at build time
- A Nextcloud calendar
- A Nextcloud app password for the user that owns or can edit that calendar

## Nextcloud Setup

Create or choose a dedicated calendar in Nextcloud Calendar. Then copy the exact CalDAV calendar collection URL.

The URL must point to the calendar collection itself and must end with `/`, for example:

```text
https://cloud.example.com/remote.php/dav/calendars/family-house/vacation-house/
```

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

`public_path` is optional. Leave it empty for local development at `/`, or set it to a long unguessable path such as `/REPLACE_WITH_LONG_RANDOM_PATH` when publishing booky behind a reverse proxy.

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

Build a release-ready static binary:

```sh
sh scripts/booky.sh build
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

The frontend must be built before the Go binary is built. The `build` command does this automatically.

## nginx Reverse Proxy

booky can be exposed only under a long unguessable URL path. This is useful for a public, unauthenticated deployment, but it is not real authentication. Add nginx auth or another access-control layer if you need actual access control.

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
    http2 on;
    listen 443 ssl;
    server_name booky.example.com;

    ssl_certificate /etc/letsencrypt/live/booky.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/booky.example.com/privkey.pem;

    location = /REPLACE_WITH_LONG_RANDOM_PATH/ {
        return 301 /REPLACE_WITH_LONG_RANDOM_PATH;
    }

    location = /REPLACE_WITH_LONG_RANDOM_PATH {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location ^~ /REPLACE_WITH_LONG_RANDOM_PATH/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location / {
        return 404;
    }
}
```

Keep `proxy_pass` without a trailing slash. booky needs to receive the full public path so it can enforce `public_path` and generate matching asset and API URLs. Do not expose `config.yaml`.

## Backups

booky creates no database files. Back up the configured Nextcloud calendar using your normal Nextcloud backup process, or export the calendar from Nextcloud Calendar.

## Limitations

- Public and unauthenticated by design
- One configured CalDAV calendar
- No CalDAV discovery
- No recurring-event support
- Overlapping bookings are allowed
- No conflict prevention beyond CalDAV ETag handling for updates and deletes
- No local database or offline mode
