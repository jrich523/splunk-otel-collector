// Copyright Splunk, Inc.
// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package configprovider

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// Build builds the ConfigSource objects according to the given ConfigSettings.
func Build(ctx context.Context, configSourcesSettings map[string]Source, params CreateParams, factories Factories) (map[string]ConfigSource, error) {
	cfgSources := make(map[string]ConfigSource, len(configSourcesSettings))
	for fullName, cfgSrcSettings := range configSourcesSettings {
		// If we have the setting we also have the factory.
		factory, ok := factories[cfgSrcSettings.ID().Type()]
		if !ok {
			return nil, fmt.Errorf("unknown %s config source type for %s", cfgSrcSettings.ID().Type(), fullName)
		}

		params.Logger = params.Logger.With(zap.String("config_source", fullName))
		cfgSrc, err := factory.CreateConfigSource(ctx, params, cfgSrcSettings)
		if err != nil {
			return nil, fmt.Errorf("failed to create config source %s: %w", fullName, err)
		}

		if cfgSrc == nil {
			return nil, fmt.Errorf("factory for %q produced a nil extension", fullName)
		}

		cfgSources[fullName] = cfgSrc
	}

	return cfgSources, nil
}
