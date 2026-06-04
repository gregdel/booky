# booky

booky is a tiny self-hosted vacation-house booking app backed by one configured Nextcloud CalDAV calendar.

The app is public and unauthenticated. It does not provide app-level auth, accounts, sessions, or access-control rules. If access should be restricted, put authentication in front of booky with deployment infrastructure such as a reverse proxy.

## Configuration

booky loads `config.yaml` by default. Use `-config <path>` to load another file.

```yaml
listen_addr: ":8080"
app_title: "Vacation House"

caldav:
  url: "https://cloud.example.com/remote.php/dav/calendars/family-house/vacation-house/"
  user: "family-house"
  pass: "app-password"
```

`caldav.url` must be the exact calendar collection URL and must end with `/`.

Keep `config.yaml` private. It contains the Nextcloud app password and is ignored by git.

## Development

Start the server:

```sh
go run ./cmd/booky -config config-example.yaml
```

Then open `http://localhost:8080/`.

Run tests:

```sh
go test ./...
```
