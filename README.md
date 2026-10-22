# sat

Command-line client for Satellite. A single Go binary that replaces the former
collection of shell scripts (`arl`, `arr`, `ara`, `arw`, `dashb`, `sat-play`,
`wt`, `sat-notify`, `sat-login`, ...).

## Commands

```text
sat auth                                      # show authentication status
sat auth login [--replace-url] [--url-only]
sat auth get-url                              # print the base URL, for scripts

sat article read [query] [--id ID] [--raw]   # alias: sat arl
sat article edit [query] [--id ID]
sat article new

sat music sync
sat music play [query] [--id ID]
sat music pause|resume|next|previous

sat dashboard [-f|--follow [5s]]              # alias: dash
sat machine list [--json] [-f|--follow [5s]]  # alias: sat machines ls
sat machine show [name] [--json|--raw] [-f|--follow [5s]]
sat weather [place]                           # alias: wt
sat notify MESSAGE
sat run [artisan arguments...]

sat completion bash|zsh|fish|powershell
```

Run `sat <command> --help` for details. `sat arl` is short for
`sat article read`, as is `sat article arl`.

Interactive pickers are fzf-style: the prompt sits at the bottom, the best match
appears directly above it, and `up` moves away from the prompt.

### Machines

Machines report their system stats to Satellite. `sat machine list` prints
every machine sorted by name with its online state and latest readings; on a
narrow terminal the less important columns are dropped, and piped output keeps
all columns without colour. `sat machine show NAME` prints one machine's full
report; the name must match exactly, and without one `show` opens a picker on a
terminal. Machine names complete in the shell once the completion script is
loaded. A machine is online when it reported within the last two minutes, and
memory or disk usage at or above 85% is flagged as high: `!` in the list,
`(high)` in the report.

`--json` prints the `data` value of the API response, unrounded, for scripts
and `jq`; `--raw` prints only the stats modules the machine reported. `--follow`
refreshes the view like `sat dashboard --follow`; give the interval after the
name (`sat machine show alpha -f 10s`), or as `--follow=10s` when using the
picker, since a single argument is always the name. The API token needs
permission to inspect machines; the API answers HTTP 403 otherwise.

## Install

```sh
go install github.com/mantas6/sat-cli/cmd/sat@latest
```

or, from a checkout:

```sh
go build -trimpath -o sat ./cmd/sat
```

`sat --version` prints the value set with `-ldflags "-X main.version=..."`,
else the module version recorded by the Go toolchain (the tag or
pseudo-version, `+dirty` for modified checkouts), else the VCS revision, else
`dev`.

Runtime dependencies:

- `ssh` (OpenSSH) for `sat run`.
- `nvim` (Neovim) for `sat article edit` and `sat article new`.

### Shell completion

`sat completion <shell>` prints a completion script for bash, zsh, fish or
PowerShell; `sat completion <shell> --help` explains how to load it. For
example, for zsh:

```sh
sat completion zsh > "${fpath[1]}/_sat"
```

## Configuration

State lives in the first of `$SAT_JOURNAL_STATE`, `$XDG_STATE_HOME/sat`,
`~/.local/state/sat`. `sat auth login` stores the base URL and token there, and
`sat auth` prints the current authentication status (state directory, base URL,
URL and token file paths, and whether a token is configured) without ever
revealing the token value. `sat auth get-url` prints only the base URL and a
newline, for scripts such as `curl "$(sat auth get-url)/api/..."`; it prints
nothing to stdout and exits non-zero when no URL is configured. `SAT_URL_PATH`
and `SAT_TOKEN_PATH` override where the base URL and token are read from and
written to; when set, `sat auth` annotates the affected paths with
`(from SAT_URL_PATH)` / `(from SAT_TOKEN_PATH)`. Existing state files from the
shell scripts (`url`, `token`, `list`, `tracks`, `tmp/`) are reused.

Two picker caches live in the state directory:

- `list` holds the articles offered by `sat article read`. It is fetched on
  first use and dropped whenever `sat article edit` or `sat article new` saves
  an article; delete it to force a refresh.
- `tracks` holds the saved tracks offered by `sat music play`. It is only
  written by `sat music sync`, which must run before the first `sat music play`
  without `--id`.

Journals and machines are always fetched on demand and never cached.

`sat --help` lists the environment variables sat honours: `SAT_JOURNAL_STATE`,
`XDG_STATE_HOME`, `SAT_URL_PATH`, `SAT_TOKEN_PATH`, `REMOTE_HOST`,
`REMOTE_USER`, `REMOTE_ROOT` and `NO_COLOR` (disables colours in `sat machine`
output).

`sat run` honours `REMOTE_HOST`, `REMOTE_USER` and `REMOTE_ROOT`, falling back
to a host derived from the base URL and `$HOME/Sat/current` on the remote. It
forwards SIGINT/SIGTERM to the remote ssh session and exits with `128+signal`
when the session is terminated by a signal.

## Development

```sh
gofmt -l .
go vet ./...
go test -race ./...
go build -o sat ./cmd/sat
```

CI additionally runs `staticcheck` and `govulncheck`.

## License

MIT; see [LICENSE](LICENSE).
