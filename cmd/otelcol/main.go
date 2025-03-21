// Copyright Splunk, Inc.
// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Program otelcol is the OpenTelemetry Collector that collects stats
// and traces and exports to a configured backend.
package main

import (
	"fmt"
	"log"
	"os"

	flag "github.com/spf13/pflag"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/otelcol"
	"go.uber.org/zap"

	"github.com/signalfx/splunk-otel-collector/internal/components"
	"github.com/signalfx/splunk-otel-collector/internal/configconverter"
	"github.com/signalfx/splunk-otel-collector/internal/configprovider"
	"github.com/signalfx/splunk-otel-collector/internal/configsources"
	"github.com/signalfx/splunk-otel-collector/internal/confmapprovider/discovery"
	"github.com/signalfx/splunk-otel-collector/internal/settings"
	"github.com/signalfx/splunk-otel-collector/internal/version"
)

func main() {
	// TODO: Use same format as the collector
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	collectorSettings, err := settings.New(os.Args[1:])
	if err != nil {
		// Exit if --help flag was supplied and usage help was displayed.
		if err == flag.ErrHelp {
			os.Exit(0)
		}
		log.Fatalf(`invalid settings detected: %v. Use "--help" to show valid usage`, err)
	}

	factories, err := components.Get()
	if err != nil {
		log.Fatalf("failed to build default components: %v", err)
	}

	info := component.BuildInfo{
		Command: "otelcol",
		Version: version.Version,
	}

	confMapConverters := collectorSettings.ConfMapConverters()
	configServer := configconverter.NewConfigServer()
	dryRun := configconverter.NewDryRun(collectorSettings.IsDryRun())
	confMapConverters = append(confMapConverters, dryRun, configServer)

	discovery, err := discovery.New()
	if err != nil {
		log.Fatalf("failed to create discovery provider: %v", err)
	}

	hooks := []configprovider.Hook{configServer, dryRun}
	envProvider := envprovider.New()
	fileProvider := fileprovider.New()
	serviceConfigProvider, err := otelcol.NewConfigProvider(
		otelcol.ConfigProviderSettings{
			ResolverSettings: confmap.ResolverSettings{
				URIs: collectorSettings.ResolverURIs(),
				Providers: map[string]confmap.Provider{
					discovery.ConfigDScheme(): configprovider.NewConfigSourceConfigMapProvider(
						discovery.ConfigDProvider(),
						zap.NewNop(), // The service logger is not available yet, setting it to Nop.
						info, hooks, configsources.Get()...,
					),
					discovery.DiscoveryModeScheme(): configprovider.NewConfigSourceConfigMapProvider(
						discovery.DiscoveryModeProvider(), zap.NewNop(), info, hooks, configsources.Get()...,
					),
					envProvider.Scheme(): configprovider.NewConfigSourceConfigMapProvider(
						envProvider, zap.NewNop(), info, hooks, configsources.Get()...,
					),
					fileProvider.Scheme(): configprovider.NewConfigSourceConfigMapProvider(
						fileProvider, zap.NewNop(), info, hooks, configsources.Get()...,
					),
				}, Converters: confMapConverters,
			},
		})
	if err != nil {
		log.Fatal(err)
	}

	serviceSettings := otelcol.CollectorSettings{
		BuildInfo:      info,
		Factories:      factories,
		ConfigProvider: serviceConfigProvider,
	}

	os.Args = append(os.Args[:1], collectorSettings.ColCoreArgs()...)
	if err = run(serviceSettings); err != nil {
		log.Fatal(err)
	}
}

func runInteractive(settings otelcol.CollectorSettings) error {
	cmd := otelcol.NewCommand(settings)
	if err := cmd.Execute(); err != nil {
		return fmt.Errorf("application run finished with error: %w", err)
	}

	return nil
}
