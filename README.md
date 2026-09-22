# sat

Command-line client for Satellite. A single Go binary that replaces the former
collection of shell scripts (`arl`, `arr`, `ara`, `arw`, `dashb`, `sat-play`,
`wt`, `sat-notify`, `sat-login`, ...).

## Commands

```text
sat auth                                      # show authentication status
sat auth login [--replace-url] [--url-only]

sat article read [query] [--id ID] [--raw]   # alias: arl
sat article edit [query] [--id ID]
sat article new

sat music sync
sat music play [query] [--id ID]
sat music pause|resume|next|previous

sat dashboard [--follow [5s]]                 # alias: dash
sat weather [place]                           # alias: wt
sat notify MESSAGE
sat run [artisan arguments...]
```

Run `sat <command> --help` for details.

Interactive pickers are fzf-style: the prompt sits at the bottom, the best match
appears directly above it, and `up` moves away from the prompt.

## Build

```sh
go build -trimpath -o sat .
```

Requires Go and, for `sat article edit|new`, Neovim. The version shown by
`sat --version` can be set with `-ldflags "-X main.version=..."`.

## Configuration

State lives in the first of `$SAT_JOURNAL_STATE`, `$XDG_STATE_HOME/sat`,
`~/.local/state/sat`. `sat auth login` stores the base URL and token there, and
`sat auth` prints the current authentication status (state directory, base URL,
URL and token file paths, and whether a token is configured) without ever
revealing the token value. `SAT_URL_PATH` and `SAT_TOKEN_PATH` override where the
base URL and token are read from and written to; when set, `sat auth` annotates
the affected paths with `(from SAT_URL_PATH)` / `(from SAT_TOKEN_PATH)`. Existing
state files from the shell scripts (`url`, `token`, `journals`, `list`, `tracks`,
`tmp/`) are reused.

`sat --help` lists the environment variables sat honours: `SAT_JOURNAL_STATE`,
`XDG_STATE_HOME`, `SAT_URL_PATH`, `SAT_TOKEN_PATH`, `REMOTE_HOST`, `REMOTE_USER`
and `REMOTE_ROOT`.

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
