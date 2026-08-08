package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

// echoResponse mirrors the /api/v1/echo endpoint payload.
type echoResponse struct {
	IP         string `json:"ip"`
	UserAgent  string `json:"user_agent"`
	ReceivedAt string `json:"received_at"`
}

// newEchoClientCommand acts as an HTTP client for the echo endpoint.
func newEchoClientCommand() *cobra.Command {
	var url string

	cmd := &cobra.Command{
		Use:   "echo-client",
		Short: "Query the API echo endpoint",
		Long: `Perform a GET request against the /api/v1/echo endpoint and print the
reflected IP address, User-Agent, and request time. Useful for testing the
API without curl.`,
		Example: `  vpc-proof echo-client
  vpc-proof echo-client --url http://localhost:8080`,
		GroupID: "administration",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appFrom(cmd)
			if app == nil {
				return fmt.Errorf("echo-client: application is not initialized")
			}

			base := url
			if base == "" {
				base = defaultServerURL(app)
			}
			target := strings.TrimRight(base, "/") + "/api/v1/echo"

			client := &http.Client{Timeout: app.config.Probes.Timeout.Value()}
			resp, err := client.Get(target)
			if err != nil {
				return fmt.Errorf("echo-client: GET %s: %w", target, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("echo-client: %s returned status %d: %s", target, resp.StatusCode, strings.TrimSpace(string(body)))
			}

			var echo echoResponse
			if err := json.NewDecoder(resp.Body).Decode(&echo); err != nil {
				return fmt.Errorf("echo-client: decode response: %w", err)
			}

			cmd.Printf("IP          : %s\n", echo.IP)
			cmd.Printf("User-Agent  : %s\n", echo.UserAgent)
			cmd.Printf("Received at : %s\n", echo.ReceivedAt)
			return nil
		},
	}

	cmd.Flags().StringVar(&url, "url", "", "base URL of the API (defaults to the configured server address)")

	return cmd
}

// defaultServerURL derives the API base URL from the server configuration,
// mapping wildcard bind addresses to localhost.
func defaultServerURL(app *App) string {
	addr := app.config.Server.Addr
	if addr == "" || addr == "0.0.0.0" || addr == "::" {
		addr = "localhost"
	}
	return fmt.Sprintf("http://%s:%d", addr, app.config.Server.Port)
}
