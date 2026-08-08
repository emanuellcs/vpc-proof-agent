package cli

import (
	"context"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/config"
	"github.com/emanuellcs/vpc-proof-agent/internal/diagnostic"
	"github.com/emanuellcs/vpc-proof-agent/internal/history"
	"github.com/emanuellcs/vpc-proof-agent/internal/observability"
	"github.com/emanuellcs/vpc-proof-agent/internal/probe"
	"github.com/emanuellcs/vpc-proof-agent/pkg/metadata"
	"github.com/emanuellcs/vpc-proof-agent/pkg/netutil"
)

// appDeps lets tests substitute the external adapters that probes depend on.
// A nil field falls back to the production implementation.
type appDeps struct {
	metadataClient    metadata.Client
	resolver          netutil.Resolver
	routeReader       netutil.RouteTableReader
	interfaceProvider netutil.InterfaceProvider
	echoHTTPClient    *http.Client
	fileReader        func(string) ([]byte, error)
}

// App is the dependency container shared by the CLI commands. It is built
// once during the root command's bootstrap and holds every initialized
// service.
type App struct {
	config            *config.Config
	logger            *observability.Logger
	metadata          metadata.Client
	routeReader       netutil.RouteTableReader
	interfaceProvider netutil.InterfaceProvider
	runner            *probe.Runner
	diagnostics       *diagnostic.Engine
	history           *history.Store
}

// buildApp assembles the probes from the configuration and wraps them in a
// Runner. No network I/O happens here, so a metadata-unreachable environment
// (such as LocalStack) still bootstraps successfully.
func buildApp(cfg *config.Config, logger *observability.Logger, deps *appDeps) (*App, error) {
	meta := deps.metadataClient
	if meta == nil {
		meta = metadata.New(metadata.Options{})
	}

	resolver := deps.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	routeReader := deps.routeReader
	if routeReader == nil {
		routeReader = netutil.NewProcRouteTableReader()
	}

	ifaceProvider := deps.interfaceProvider
	if ifaceProvider == nil {
		ifaceProvider = netutil.OSInterfaceProvider{}
	}

	httpClient := deps.echoHTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Probes.Timeout.Value()}
	}

	fileReader := deps.fileReader
	if fileReader == nil {
		fileReader = os.ReadFile
	}

	echoURL := ""
	if len(cfg.Probes.EchoURLs) > 0 {
		echoURL = cfg.Probes.EchoURLs[0]
	}

	probes := []probe.Probe{
		probe.NewMetadataProbe(meta, logger),
		probe.NewVPCOwnershipProbe(meta, cfg.Probes.VpcCIDR, logger),
		probe.NewSubnetOwnershipProbe(meta, cfg.Probes.SubnetCIDR, logger),
		probe.NewDefaultRouteProbe(routeReader, ifaceProvider, logger),
		probe.NewDNSProbe(resolver, cfg.Probes.DNSHost, logger),
		probe.NewInternetHTTPSProbe(httpClient, echoURL, cfg.Probes.MaxRetries, cfg.Probes.Timeout.Value(), logger),
		probe.NewPublicIPConsistencyProbe(meta, httpClient, echoURL, cfg.Probes.Timeout.Value(), logger),
		probe.NewSystemResourcesProbe(fileReader, logger),
		probe.NewClockSkewProbe(httpClient, echoURL, time.Now, logger),
	}

	runner := probe.NewRunner(
		probes,
		probe.WithLogger(logger),
		probe.WithProbeTimeout(cfg.Probes.Timeout.Value()),
	)

	historyStore := history.New(history.Options{
		MaxEntries:    cfg.History.MaxEntries,
		DiskPath:      cfg.History.DiskPath,
		FlushInterval: cfg.History.FlushInterval.Value(),
	})

	return &App{
		config:            cfg,
		logger:            logger,
		metadata:          meta,
		routeReader:       routeReader,
		interfaceProvider: ifaceProvider,
		runner:            runner,
		diagnostics:       diagnostic.New(),
		history:           historyStore,
	}, nil
}

// RunProbes executes the full probe suite and records a summary in the
// history store.
func (a *App) RunProbes(ctx context.Context) probe.Report {
	report := a.runner.Run(ctx)
	if a.history != nil {
		entry := history.FromReport(report)
		a.history.Append(&entry)
	}
	return report
}

// Diagnose translates a probe report into troubleshooting hints.
func (a *App) Diagnose(report probe.Report) []diagnostic.Hint {
	return a.diagnostics.Analyze(report)
}

// instanceInfo holds the metadata summary shown by the status command.
type instanceInfo struct {
	InstanceID       string
	AvailabilityZone string
	PrivateIP        string
	PublicIP         string
	// MetadataError is set when the metadata service could not be reached.
	MetadataError error
}

// fetchInstanceInfo gathers the metadata summary, tolerating an unreachable
// metadata service.
func (a *App) fetchInstanceInfo(ctx context.Context) instanceInfo {
	info := instanceInfo{}
	info.InstanceID, info.MetadataError = a.metadata.InstanceID(ctx)
	if info.MetadataError != nil {
		return info
	}
	info.AvailabilityZone, _ = a.metadata.AvailabilityZone(ctx)
	info.PrivateIP, _ = a.metadata.PrivateIP(ctx)
	info.PublicIP, _ = a.metadata.PublicIP(ctx)
	return info
}

// routeSummary holds the default-route information shown by the status
// command.
type routeSummary struct {
	Gateway   string
	Interface string
	// Available is false when the routing table could not be read.
	Available bool
}

// fetchRouteSummary reads the routing table, tolerating read failures.
func (a *App) fetchRouteSummary(ctx context.Context) routeSummary {
	routes, err := a.routeReader.ReadRouteTable(ctx)
	if err != nil {
		return routeSummary{Available: false}
	}
	route, ok := netutil.DefaultRoute(routes)
	if !ok {
		return routeSummary{Available: true}
	}
	return routeSummary{
		Gateway:   route.Gateway.String(),
		Interface: route.Interface,
		Available: true,
	}
}
