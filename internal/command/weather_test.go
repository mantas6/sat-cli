package command

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestWeatherForwardsEmptyPlaceAndWritesOutput(t *testing.T) {
	t.Parallel()
	gotPlace := "unset"
	client := &fakeAPI{weather: func(_ context.Context, place string) (string, error) {
		gotPlace = place
		return "Cloudy", nil
	}}
	app, stdout, _ := newTestApp(t, withAPI(client))

	if err := run(t, app, "weather"); err != nil {
		t.Fatal(err)
	}
	if gotPlace != "" {
		t.Fatalf("place = %q, want %q", gotPlace, "")
	}
	if got := stdout.String(); got != "Cloudy\n" {
		t.Fatalf("output = %q, want %q", got, "Cloudy\n")
	}
}

func TestWeatherForwardsCustomPlaceAndPreservesTrailingNewline(t *testing.T) {
	t.Parallel()
	var gotPlace string
	client := &fakeAPI{weather: func(_ context.Context, place string) (string, error) {
		gotPlace = place
		return "Sunny\n", nil
	}}
	app, stdout, _ := newTestApp(t, withAPI(client))

	if err := run(t, app, "wt", "new york"); err != nil {
		t.Fatal(err)
	}
	if gotPlace != "new york" {
		t.Fatalf("place = %q, want %q", gotPlace, "new york")
	}
	if got := stdout.String(); got != "Sunny\n" {
		t.Fatalf("output = %q, want %q", got, "Sunny\n")
	}
}

func TestWeatherUsesUnauthenticatedClient(t *testing.T) {
	t.Parallel()
	cfg := newFakeConfig(t)
	cfg.token = ""
	app, _, _ := newTestApp(t, withConfig(cfg))
	gotToken := "unset"
	app.NewAPIClient = func(_, token string) (APIClient, error) {
		gotToken = token
		return &fakeAPI{}, nil
	}

	if err := run(t, app, "weather"); err != nil {
		t.Fatal(err)
	}
	if gotToken != "" {
		t.Fatalf("NewAPIClient() token = %q, want none", gotToken)
	}
}

func TestWeatherAPIErrorPropagates(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("weather failed")
	client := &fakeAPI{weather: func(context.Context, string) (string, error) {
		return "", wantErr
	}}
	app, _, _ := newTestApp(t, withAPI(client))

	if err := run(t, app, "weather"); !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

func TestWeatherMissingURLErrorPropagates(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("base URL is not configured")
	cfg := newFakeConfig(t)
	cfg.baseURLErr = wantErr
	app, _, _ := newTestApp(t, withConfig(cfg))

	if err := run(t, app, "weather"); !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

func TestWeatherRejectsTooManyArguments(t *testing.T) {
	t.Parallel()
	app, _, _ := newTestApp(t)
	err := run(t, app, "weather", "vilnius", "kaunas")
	if err == nil || !strings.Contains(err.Error(), "accepts at most 1 arg(s), received 2") {
		t.Fatalf("Execute() error = %v, want too many arguments", err)
	}
}
