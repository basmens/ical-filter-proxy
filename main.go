package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
)

var version = "development"

type Options struct {
	ConfigFile     string
	DebugLogging   bool
	JSONLogging    bool
	ListenAddress  string
	ValidateConfig bool
	PrintVersion   bool
}

func parseFlags() Options {
	var opts Options

	flag.StringVar(&opts.ConfigFile, "config", "config.yaml", "config file")
	flag.BoolVar(&opts.DebugLogging, "debug", false, "enable debug logging")
	flag.BoolVar(&opts.PrintVersion, "version", false, "print version and exit")
	flag.BoolVar(&opts.JSONLogging, "json", false, "output logging in JSON format")
	flag.StringVar(&opts.ListenAddress, "address", ":8080", "listening address for api")
	flag.BoolVar(&opts.ValidateConfig, "validate", false, "validate config and exit")

	flag.Parse()
	return opts
}

func setupLogger(opts Options) *slog.Logger {
	level := slog.LevelInfo // default loglevel
	if opts.DebugLogging {
		level = slog.LevelDebug // debug logging enabled
	}

	handlerOpts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	if opts.JSONLogging {
		handler = slog.NewJSONHandler(os.Stdout, handlerOpts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, handlerOpts)
	}
	return slog.New(handler)
}

func main() {

	opts := parseFlags()

	// print version and exit
	if opts.PrintVersion {
		fmt.Println("version:", version)
		os.Exit(0)
	}

	logger := setupLogger(opts)
	slog.SetDefault(logger)

	// load configuration
	slog.Debug("reading config", "configFile", opts.ConfigFile)
	var config Config
	if !config.LoadConfig(opts.ConfigFile) {
		os.Exit(1) // fail if config is not valid
	}
	slog.Debug("loaded config")

	// print a message and exit if validate arg was specified
	if opts.ValidateConfig {
		slog.Info("configuration was validated successfully")
		os.Exit(0)
	}

	// iterate through calendars in the config and setup a handler for each
	// todo: consider refactor to route requests dynamically?
	for _, calendarConfig := range config.Calendars {

		cal := calendarConfig // copy to new var

		// configure HTTP endpoint
		httpPath := "/calendars/" + cal.Name + "/feed"
		slog.Debug("Configuring endpoint", "calendar", cal.Name, "http_path", httpPath)
		http.HandleFunc(httpPath, func(w http.ResponseWriter, r *http.Request) {

			slog.Debug("Received request for calendar", "http_path", httpPath, "calendar", cal.Name, "client_ip", r.RemoteAddr)

			// validate token
			token := r.URL.Query().Get("token")
			if token != cal.Token {
				slog.Warn("Unauthorized access attempt", "client_ip", r.RemoteAddr)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// fetch and filter upstream calendar
			feed, err := cal.fetch()
			if err != nil {
				slog.Error("Error fetching and filtering feed", "error", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			// return calendar
			w.Header().Set("Content-Type", "text/calendar")
			_, err = w.Write(feed)
			if err != nil {
				slog.Error("Error writing response", "error", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			slog.Info("Calendar request processed", "http_path", httpPath, "calendar", cal.Name, "client_ip", r.RemoteAddr)
		})

	}

	// add a readiness and liveness check endpoint (return blank 200 OK response)
	http.HandleFunc("/liveness", func(w http.ResponseWriter, r *http.Request) {})
	http.HandleFunc("/readiness", func(w http.ResponseWriter, r *http.Request) {})

	// start the webserver
	slog.Info("Starting web server", "address", opts.ListenAddress)
	if err := http.ListenAndServe(opts.ListenAddress, nil); err != nil {
		slog.Error("Error starting web server", "error", err)
	}

}
