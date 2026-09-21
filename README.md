# sat

Command-line client for Satellite. A single Go binary that replaces the former
collection of shell scripts (`arl`, `arr`, `ara`, `arw`, `dashb`, `sat-play`,
`wt`, `sat-notify`, `sat-login`, ...).

## Commands

```text
sat login [--replace-url] [--url-only]

sat article read [query] [--id ID] [--raw]
sat article edit [query] [--id ID]
sat article new

sat music sync
sat music play [query] [--id ID]
sat music pause|resume|next|previous

sat dashboard [--follow [5s]]
sat weather [place]
sat notify MESSAGE
sat run [artisan arguments...]
```

Run `sat <command> --help` for details.

## Build

```sh
go build -trimpath -o sat .
```

Requires Go and, for `sat article edit|new`, Neovim. The version shown by
`sat --version` can be set with `-ldflags "-X main.version=..."`.

## Configuration

State lives in the first of `$SAT_JOURNAL_STATE`, `$XDG_STATE_HOME/sat`,
`~/.local/state/sat`. `sat login` stores the base URL and token there.
`SAT_URL_PATH` and `SAT_TOKEN_PATH` override where the base URL and token are
read from and written to. Existing state files from the shell scripts (`url`,
`token`, `journals`, `list`, `tracks`, `tmp/`) are reused.

`sat run` honours `REMOTE_HOST`, `REMOTE_USER` and `REMOTE_ROOT`, falling back
to a host and release directory derived from the base URL. It forwards
SIGINT/SIGTERM to the remote ssh session and exits with `128+signal` when the
session is terminated by a signal.

## Development

```sh
go test ./...
go vet ./...
go build .
```
