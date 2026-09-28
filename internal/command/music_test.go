package command

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/mantas6/sat-cli/internal/ui"
)

// trackConfig returns a configured fakeConfig whose track cache holds lines.
func trackConfig(t *testing.T, lines ...string) *fakeConfig {
	t.Helper()
	cfg := newFakeConfig(t)
	cfg.caches = map[string][]string{trackCacheName: lines}
	return cfg
}

var twoTracks = []string{
	"one\tFirst artist\t/Album\t/01.\tFirst track",
	"two\tSecond artist\t/Album\t/02.\tSecond track",
}

func TestMusicSyncWritesTrackCache(t *testing.T) {
	t.Parallel()
	want := []string{"id-1\tArtist\t/Album\t/01.\tTrack", "id-2\tArtist\t/Album\t/02.\tOther"}
	client := &fakeAPI{savedTracks: func(context.Context) ([]string, error) {
		return want, nil
	}}
	cfg := newFakeConfig(t)
	app, _, stderr := newTestApp(t, withAPI(client), withConfig(cfg))

	if err := run(t, app, "music", "sync"); err != nil {
		t.Fatal(err)
	}
	if got := cfg.caches[trackCacheName]; !reflect.DeepEqual(got, want) {
		t.Fatalf("track cache = %#v, want %#v", got, want)
	}
	if got := stderr.String(); got != "Synced 2 tracks.\n" {
		t.Fatalf("stderr = %q, want sync summary", got)
	}
}

func TestMusicSyncKeepsEachTrackOnOneCacheLine(t *testing.T) {
	t.Parallel()
	client := &fakeAPI{savedTracks: func(context.Context) ([]string, error) {
		return []string{"id-1\tArtist\t/Album\t/01.\tTwo\nlines\r\nhere\r"}, nil
	}}
	cfg := newFakeConfig(t)
	app, _, _ := newTestApp(t, withAPI(client), withConfig(cfg))

	if err := run(t, app, "music", "sync"); err != nil {
		t.Fatal(err)
	}
	want := []string{"id-1\tArtist\t/Album\t/01.\tTwo lines here "}
	if got := cfg.caches[trackCacheName]; !reflect.DeepEqual(got, want) {
		t.Fatalf("track cache = %#v, want %#v", got, want)
	}
}

func TestMusicPlayWithIDSkipsSelectionAndCache(t *testing.T) {
	t.Parallel()
	var gotID string
	client := &fakeAPI{playTrack: func(_ context.Context, id string) error {
		gotID = id
		return nil
	}}
	app, _, _ := newTestApp(t, withAPI(client))

	if err := run(t, app, "music", "play", "--id", "spotify-id"); err != nil {
		t.Fatal(err)
	}
	if gotID != "spotify-id" {
		t.Fatalf("PlayTrack() ID = %q, want spotify-id", gotID)
	}
}

func TestMusicPlayRejectsEmptyID(t *testing.T) {
	t.Parallel()
	app, _, _ := newTestApp(t)
	if err := run(t, app, "music", "play", "--id", " "); err == nil || err.Error() != "track ID must not be empty" {
		t.Fatalf("Execute() error = %v, want empty ID error", err)
	}
}

func TestMusicPlayMissingCache(t *testing.T) {
	t.Parallel()
	app, _, _ := newTestApp(t)

	err := run(t, app, "music", "play")
	if err == nil || err.Error() != "track cache is missing; run `sat music sync`" {
		t.Fatalf("Execute() error = %v, want missing-cache guidance", err)
	}
}

func TestMusicPlayUsesSelectorAndPassesQuery(t *testing.T) {
	t.Parallel()
	var gotID string
	client := &fakeAPI{playTrack: func(_ context.Context, id string) error {
		gotID = id
		return nil
	}}
	app, _, _ := newTestApp(t, withAPI(client), withConfig(trackConfig(t, twoTracks...)), withTTY())
	var gotItems []ui.Item
	var gotOpts ui.SelectOptions
	app.Select = selectIndex(1, &gotItems, &gotOpts)

	// "track" matches both entries, so the interactive selector must run.
	if err := run(t, app, "music", "play", "track"); err != nil {
		t.Fatal(err)
	}
	if gotID != "two" {
		t.Fatalf("PlayTrack() ID = %q, want two", gotID)
	}
	if gotOpts.Query != "track" || gotOpts.Title != "Saved tracks" {
		t.Fatalf("selector options = %#v", gotOpts)
	}
	if !reflect.DeepEqual(gotItems, parseCacheLines(twoTracks)) {
		t.Fatalf("selector items = %#v, want the cache in order", gotItems)
	}
}

func TestMusicPlaySingleMatchSkipsSelectorWithoutTerminal(t *testing.T) {
	t.Parallel()
	var gotID string
	client := &fakeAPI{playTrack: func(_ context.Context, id string) error {
		gotID = id
		return nil
	}}
	// Streams are not terminals and Select fails the test if it runs, so
	// only the non-interactive fast path can succeed here.
	app, _, _ := newTestApp(t, withAPI(client), withConfig(trackConfig(t, twoTracks...)))

	if err := run(t, app, "music", "play", "second", "track"); err != nil {
		t.Fatal(err)
	}
	if gotID != "two" {
		t.Fatalf("PlayTrack() ID = %q, want two", gotID)
	}

	if err := run(t, app, "music", "play", "track"); !errors.Is(err, errSelectorNeedsTerminal) {
		t.Fatalf("ambiguous query without terminal error = %v", err)
	}
}

func TestMusicPlayNoMatchFails(t *testing.T) {
	t.Parallel()
	app, _, _ := newTestApp(t, withConfig(trackConfig(t, twoTracks...)))
	if err := run(t, app, "music", "play", "zzz"); err == nil || err.Error() != `no items match "zzz"` {
		t.Fatalf("Execute() error = %v, want no match", err)
	}
}

func TestMusicControlsMapActions(t *testing.T) {
	t.Parallel()
	tests := map[string]api.PlaybackAction{
		"pause":    api.Pause,
		"resume":   api.Play,
		"next":     api.Next,
		"previous": api.Previous,
	}
	for name, wantAction := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var gotAction api.PlaybackAction
			client := &fakeAPI{controlPlayback: func(_ context.Context, action api.PlaybackAction) error {
				gotAction = action
				return nil
			}}
			app, _, _ := newTestApp(t, withAPI(client))

			if err := run(t, app, "music", name); err != nil {
				t.Fatal(err)
			}
			if gotAction != wantAction {
				t.Fatalf("ControlPlayback() action = %q, want %q", gotAction, wantAction)
			}
		})
	}
}

func TestMusicSpotifyErrorPropagates(t *testing.T) {
	t.Parallel()
	wantErr := &api.SpotifyError{Message: "No active device"}
	client := &fakeAPI{playTrack: func(context.Context, string) error {
		return wantErr
	}}
	app, _, _ := newTestApp(t, withAPI(client))

	if err := run(t, app, "music", "play", "--id", "track"); !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want Spotify error", err)
	}
}

func TestMusicCancellationReturnsExitCode130(t *testing.T) {
	t.Parallel()
	app, _, _ := newTestApp(t, withConfig(trackConfig(t, twoTracks[0])), withTTY())
	app.Select = func(context.Context, io.Reader, io.Writer, []ui.Item, ui.SelectOptions) (ui.Item, error) {
		return ui.Item{}, ui.ErrCancelled
	}

	err := run(t, app, "music", "play")
	var exitError ExitError
	if !errors.As(err, &exitError) || exitError.Code != 130 || !errors.Is(err, ui.ErrCancelled) {
		t.Fatalf("Execute() error = %#v, want cancellation exit code 130", err)
	}
}

func TestMusicSelectorErrorIsWrapped(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("tty gone")
	app, _, _ := newTestApp(t, withConfig(trackConfig(t, twoTracks[0])), withTTY())
	app.Select = func(context.Context, io.Reader, io.Writer, []ui.Item, ui.SelectOptions) (ui.Item, error) {
		return ui.Item{}, wantErr
	}

	err := run(t, app, "music", "play")
	if !errors.Is(err, wantErr) || err.Error() != "select item: tty gone" {
		t.Fatalf("Execute() error = %v, want wrapped selector error", err)
	}
}

func TestMusicSelectionRequiresTerminal(t *testing.T) {
	t.Parallel()
	app, _, _ := newTestApp(t, withConfig(trackConfig(t, twoTracks[0])))

	err := run(t, app, "music", "play")
	if err == nil || err.Error() != "interactive selection requires a terminal on stdin and stdout" {
		t.Fatalf("Execute() error = %v, want terminal requirement", err)
	}
}
