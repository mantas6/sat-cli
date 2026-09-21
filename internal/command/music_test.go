package command

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/mantas6/sat-cli/internal/ui"
)

type musicAPI struct {
	*stubAPI
	saved   func(context.Context) ([]string, error)
	play    func(context.Context, string) error
	control func(context.Context, string) error
}

func (m *musicAPI) SavedTracks(ctx context.Context) ([]string, error) {
	if m.saved == nil {
		return nil, nil
	}
	return m.saved(ctx)
}

func (m *musicAPI) PlayTrack(ctx context.Context, id string) error {
	if m.play == nil {
		return nil
	}
	return m.play(ctx, id)
}

func (m *musicAPI) ControlPlayback(ctx context.Context, action string) error {
	if m.control == nil {
		return nil
	}
	return m.control(ctx, action)
}

type cacheConfig struct {
	*stubConfig
	lines      []string
	exists     bool
	readErr    error
	writtenKey string
	written    []string
	writeErr   error
}

func (c *cacheConfig) ReadCacheLines(name string) ([]string, bool, error) {
	return append([]string(nil), c.lines...), c.exists, c.readErr
}

func (c *cacheConfig) WriteCacheLines(name string, lines []string) error {
	c.writtenKey = name
	c.written = append([]string(nil), lines...)
	return c.writeErr
}

func newMusicTestApp(client APIClient, store *cacheConfig) (*App, *bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if store == nil {
		store = &cacheConfig{stubConfig: &stubConfig{baseURL: "https://satellite.test", token: "secret"}}
	}
	return &App{
		Config: store,
		NewAPIClient: func(string, string) (APIClient, error) {
			return client, nil
		},
		Stdin:  strings.NewReader(""),
		Stdout: stdout,
		Stderr: stderr,
	}, stdout, stderr
}

func executeMusicTestCommand(app *App, args ...string) error {
	command := NewRootCommand(app)
	command.SetArgs(args)
	return command.Execute()
}

func TestMusicSyncWritesTrackCache(t *testing.T) {
	want := []string{"id-1\tArtist\t/Album\t/01.\tTrack", "id-2\tArtist\t/Album\t/02.\tOther"}
	client := &musicAPI{stubAPI: &stubAPI{}, saved: func(context.Context) ([]string, error) {
		return want, nil
	}}
	store := &cacheConfig{stubConfig: &stubConfig{baseURL: "https://satellite.test", token: "secret"}}
	app, _, stderr := newMusicTestApp(client, store)

	if err := executeMusicTestCommand(app, "music", "sync"); err != nil {
		t.Fatal(err)
	}
	if store.writtenKey != "tracks" || !reflect.DeepEqual(store.written, want) {
		t.Fatalf("cache write = (%q, %#v), want tracks and %#v", store.writtenKey, store.written, want)
	}
	if got := stderr.String(); got != "Synced 2 tracks.\n" {
		t.Fatalf("stderr = %q, want sync summary", got)
	}
}

func TestMusicPlayWithIDSkipsSelectionAndCache(t *testing.T) {
	var gotID string
	client := &musicAPI{stubAPI: &stubAPI{}, play: func(_ context.Context, id string) error {
		gotID = id
		return nil
	}}
	app, _, _ := newMusicTestApp(client, nil)

	if err := executeMusicTestCommand(app, "music", "play", "--id", "spotify-id"); err != nil {
		t.Fatal(err)
	}
	if gotID != "spotify-id" {
		t.Fatalf("PlayTrack() ID = %q, want spotify-id", gotID)
	}
}

func TestMusicPlayMissingCache(t *testing.T) {
	app, _, _ := newMusicTestApp(&musicAPI{stubAPI: &stubAPI{}}, nil)

	err := executeMusicTestCommand(app, "music", "play")
	if err == nil || err.Error() != "track cache is missing; run `sat music sync`" {
		t.Fatalf("Execute() error = %v, want missing-cache guidance", err)
	}
}

func TestMusicPlayUsesSelectorAndPassesQuery(t *testing.T) {
	previous := runSelector
	t.Cleanup(func() { runSelector = previous })
	var gotItems []ui.Item
	var gotOptions ui.SelectOptions
	runSelector = func(_ context.Context, _ io.Reader, _ io.Writer, items []ui.Item, opts ui.SelectOptions) (ui.Item, error) {
		gotItems = items
		gotOptions = opts
		return items[1], nil
	}

	var gotID string
	client := &musicAPI{stubAPI: &stubAPI{}, play: func(_ context.Context, id string) error {
		gotID = id
		return nil
	}}
	store := &cacheConfig{
		stubConfig: &stubConfig{baseURL: "https://satellite.test", token: "secret"},
		exists:     true,
		lines: []string{
			"one\tFirst artist\t/Album\t/01.\tFirst track",
			"two\tSecond artist\t/Album\t/02.\tSecond track",
		},
	}
	app, _, _ := newMusicTestApp(client, store)

	// "track" matches both entries, so the interactive selector must run.
	if err := executeMusicTestCommand(app, "music", "play", "track"); err != nil {
		t.Fatal(err)
	}
	if gotID != "two" {
		t.Fatalf("PlayTrack() ID = %q, want two", gotID)
	}
	if gotOptions.Query != "track" {
		t.Fatalf("selector query = %q, want track", gotOptions.Query)
	}
	if len(gotItems) != 2 || gotItems[0].ID != "one" {
		t.Fatalf("selector items = %#v", gotItems)
	}
}

func TestMusicPlaySingleMatchSkipsSelectorWithoutTerminal(t *testing.T) {
	var gotID string
	client := &musicAPI{stubAPI: &stubAPI{}, play: func(_ context.Context, id string) error {
		gotID = id
		return nil
	}}
	store := &cacheConfig{
		stubConfig: &stubConfig{baseURL: "https://satellite.test", token: "secret"},
		exists:     true,
		lines: []string{
			"one\tFirst artist\t/Album\t/01.\tFirst track",
			"two\tSecond artist\t/Album\t/02.\tSecond track",
		},
	}
	app, _, _ := newMusicTestApp(client, store)

	// Streams are buffers (not a terminal) and runSelector is the real one,
	// so only the non-interactive fast path can succeed here.
	if err := executeMusicTestCommand(app, "music", "play", "second", "track"); err != nil {
		t.Fatal(err)
	}
	if gotID != "two" {
		t.Fatalf("PlayTrack() ID = %q, want two", gotID)
	}

	err := executeMusicTestCommand(app, "music", "play", "track")
	if !errors.Is(err, errSelectorNeedsTerminal) {
		t.Fatalf("ambiguous query without terminal error = %v", err)
	}
}

func TestMusicControlsMapActions(t *testing.T) {
	tests := map[string]string{
		"pause":    "pause",
		"resume":   "play",
		"next":     "next",
		"previous": "previous",
	}
	for command, wantAction := range tests {
		t.Run(command, func(t *testing.T) {
			var gotAction string
			client := &musicAPI{stubAPI: &stubAPI{}, control: func(_ context.Context, action string) error {
				gotAction = action
				return nil
			}}
			app, _, _ := newMusicTestApp(client, nil)

			if err := executeMusicTestCommand(app, "music", command); err != nil {
				t.Fatal(err)
			}
			if gotAction != wantAction {
				t.Fatalf("ControlPlayback() action = %q, want %q", gotAction, wantAction)
			}
		})
	}
}

func TestMusicSpotifyErrorPropagates(t *testing.T) {
	wantErr := &api.SpotifyError{Message: "No active device"}
	client := &musicAPI{stubAPI: &stubAPI{}, play: func(context.Context, string) error {
		return wantErr
	}}
	app, _, _ := newMusicTestApp(client, nil)

	err := executeMusicTestCommand(app, "music", "play", "--id", "track")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want Spotify error", err)
	}
}

func TestMusicCancellationReturnsExitCode130(t *testing.T) {
	previous := runSelector
	t.Cleanup(func() { runSelector = previous })
	runSelector = func(context.Context, io.Reader, io.Writer, []ui.Item, ui.SelectOptions) (ui.Item, error) {
		return ui.Item{}, ui.ErrCancelled
	}
	store := &cacheConfig{
		stubConfig: &stubConfig{baseURL: "https://satellite.test", token: "secret"},
		exists:     true,
		lines:      []string{"one\tArtist\t/Album\t/01.\tTrack"},
	}
	app, _, _ := newMusicTestApp(&musicAPI{stubAPI: &stubAPI{}}, store)

	err := executeMusicTestCommand(app, "music", "play")
	var exitError ExitError
	if !errors.As(err, &exitError) || exitError.Code != 130 || !errors.Is(err, ui.ErrCancelled) {
		t.Fatalf("Execute() error = %#v, want cancellation exit code 130", err)
	}
}

func TestMusicSelectionRequiresTerminalWithoutHook(t *testing.T) {
	previous := runSelector
	t.Cleanup(func() { runSelector = previous })
	runSelector = ui.Select
	store := &cacheConfig{
		stubConfig: &stubConfig{baseURL: "https://satellite.test", token: "secret"},
		exists:     true,
		lines:      []string{"one\tArtist\t/Album\t/01.\tTrack"},
	}
	app, _, _ := newMusicTestApp(&musicAPI{stubAPI: &stubAPI{}}, store)

	err := executeMusicTestCommand(app, "music", "play")
	if err == nil || err.Error() != "interactive selection requires a terminal on stdin and stdout" {
		t.Fatalf("Execute() error = %v, want terminal requirement", err)
	}
}

func TestParseTrackLines(t *testing.T) {
	items := parseTrackLines([]string{
		"",
		" \t ",
		"id-only",
		"track-id\tArtist\t/Album\t/03.\tTitle",
	})
	want := []ui.Item{
		{ID: "id-only", Columns: []string{}},
		{ID: "track-id", Columns: []string{"Artist", "/Album", "/03.", "Title"}},
	}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("parseTrackLines() = %#v, want %#v", items, want)
	}
}
