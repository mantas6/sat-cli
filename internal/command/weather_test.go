package command

import (
	"context"
	"errors"
	"testing"
)

func TestWeatherForwardsEmptyPlaceAndWritesOutput(t *testing.T) {
	gotPlace := "unset"
	client := &stubAPI{weather: func(_ context.Context, place string) (string, error) {
		gotPlace = place
		return "Cloudy", nil
	}}
	app, output := newWeatherNotifyTestApp(client)

	if err := executeWeatherNotifyTestCommand(app, "weather"); err != nil {
		t.Fatal(err)
	}
	if gotPlace != "" {
		t.Fatalf("place = %q, want %q", gotPlace, "")
	}
	if got := output.String(); got != "Cloudy\n" {
		t.Fatalf("output = %q, want %q", got, "Cloudy\n")
	}
}

func TestWeatherForwardsCustomPlaceAndPreservesTrailingNewline(t *testing.T) {
	var gotPlace string
	client := &stubAPI{weather: func(_ context.Context, place string) (string, error) {
		gotPlace = place
		return "Sunny\n", nil
	}}
	app, output := newWeatherNotifyTestApp(client)

	if err := executeWeatherNotifyTestCommand(app, "weather", "new york"); err != nil {
		t.Fatal(err)
	}
	if gotPlace != "new york" {
		t.Fatalf("place = %q, want %q", gotPlace, "new york")
	}
	if got := output.String(); got != "Sunny\n" {
		t.Fatalf("output = %q, want %q", got, "Sunny\n")
	}
}

func TestWeatherAPIErrorPropagates(t *testing.T) {
	wantErr := errors.New("weather failed")
	client := &stubAPI{weather: func(context.Context, string) (string, error) {
		return "", wantErr
	}}
	app, _ := newWeatherNotifyTestApp(client)

	err := executeWeatherNotifyTestCommand(app, "weather")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

func TestWeatherMissingURLErrorPropagates(t *testing.T) {
	wantErr := errors.New("base URL is not configured")
	app, _ := newWeatherNotifyTestApp(&stubAPI{})
	app.Config = &stubConfig{baseErr: wantErr}

	err := executeWeatherNotifyTestCommand(app, "weather")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

func TestWeatherRejectsTooManyArguments(t *testing.T) {
	app, _ := newWeatherNotifyTestApp(&stubAPI{})
	if err := executeWeatherNotifyTestCommand(app, "weather", "vilnius", "kaunas"); err == nil {
		t.Fatal("Execute() error = nil, want nonzero result")
	}
}
