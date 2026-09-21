package command

// apiClient builds an authenticated API client from the persisted configuration.
// It returns actionable errors when the URL or token is missing.
func apiClient(app *App) (APIClient, error) {
	baseURL, err := app.Config.BaseURL()
	if err != nil {
		return nil, err
	}
	token, err := app.Config.Token()
	if err != nil {
		return nil, err
	}
	return app.NewAPIClient(baseURL, token)
}

// publicClient builds an API client that only needs the base URL, for
// unauthenticated endpoints such as the weather forecast.
func publicClient(app *App) (APIClient, error) {
	baseURL, err := app.Config.BaseURL()
	if err != nil {
		return nil, err
	}
	return app.NewAPIClient(baseURL, "")
}
