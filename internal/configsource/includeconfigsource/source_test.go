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

package includeconfigsource

import (
	"context"
	"os"
	"path"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/experimental/configsource"
	"go.opentelemetry.io/collector/confmap"

	"github.com/signalfx/splunk-otel-collector/internal/configprovider"
)

func TestIncludeConfigSource_Session(t *testing.T) {
	tests := []struct {
		defaults map[string]any
		params   map[string]any
		expected any
		wantErr  error
		name     string
		selector string
	}{
		{
			name:     "missing_file",
			selector: "not_to_be_found",
			wantErr:  &os.PathError{},
		},
		{
			name:     "scalar_data_file",
			selector: "scalar_data_file",
			expected: []byte("42"),
		},
		{
			name:     "no_params_template",
			selector: "no_params_template",
			expected: []byte("bool_field: true"),
		},
		{
			name:     "param_template",
			selector: "param_template",
			params: map[string]any{
				"glob_pattern": "myPattern",
			},
			expected: []byte("logs_path: myPattern"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, err := newConfigSource(configprovider.CreateParams{}, &Config{})
			require.NoError(t, err)
			require.NotNil(t, source)

			ctx := context.Background()
			defer func() {
				assert.NoError(t, source.Close(ctx))
			}()

			file := path.Join("testdata", tt.selector)
			r, err := source.Retrieve(ctx, file, confmap.NewFromStringMap(tt.params))
			if tt.wantErr != nil {
				assert.Nil(t, r)
				require.IsType(t, tt.wantErr, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, r)
			assert.Equal(t, tt.expected, r.Value())
		})
	}
}

func TestIncludeConfigSource_WatchFileClose(t *testing.T) {
	s, err := newConfigSource(configprovider.CreateParams{}, &Config{WatchFiles: true})
	require.NoError(t, err)
	require.NotNil(t, s)

	ctx := context.Background()
	defer func() {
		assert.NoError(t, s.Close(ctx))
	}()

	// Write out an initial test file
	f, err := os.CreateTemp("", "watch_file_test")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, os.Remove(f.Name()))
	}()
	_, err = f.Write([]byte("val1"))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// Perform initial retrieve
	r, err := s.Retrieve(ctx, f.Name(), nil)
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, []byte("val1"), r.Value())

	watched, ok := r.(configsource.Watchable)
	assert.True(t, ok)

	// Close current source.
	require.NoError(t, s.Close(context.Background()))
	watcherErr := watched.WatchForUpdate()
	require.ErrorIs(t, watcherErr, configsource.ErrSessionClosed)

}

func TestIncludeConfigSource_WatchFileUpdate(t *testing.T) {
	s, err := newConfigSource(configprovider.CreateParams{}, &Config{WatchFiles: true})
	require.NoError(t, err)
	require.NotNil(t, s)

	ctx := context.Background()
	defer func() {
		assert.NoError(t, s.Close(ctx))
	}()

	// Write out an initial test file
	f, err := os.CreateTemp("", "watch_file_test")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, os.Remove(f.Name()))
	}()
	_, err = f.Write([]byte("val1"))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// Perform initial retrieve
	r, err := s.Retrieve(ctx, f.Name(), nil)
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, []byte("val1"), r.Value())
	watched, ok := r.(configsource.Watchable)
	assert.True(t, ok)

	// Write update to file
	err = os.WriteFile(f.Name(), []byte("val2"), 0600)
	require.NoError(t, err)
	watcherErr := watched.WatchForUpdate()
	require.ErrorIs(t, watcherErr, configsource.ErrValueUpdated)

	// Check updated file after waiting for update
	r, err = s.Retrieve(ctx, f.Name(), nil)
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, []byte("val2"), r.Value())
}

func TestIncludeConfigSource_DeleteFile(t *testing.T) {
	s, err := newConfigSource(configprovider.CreateParams{}, &Config{DeleteFiles: true})
	require.NoError(t, err)
	require.NotNil(t, s)

	ctx := context.Background()
	defer func() {
		assert.NoError(t, s.Close(ctx))
	}()

	// Copy test file
	src := path.Join("testdata", "scalar_data_file")
	contents, err := os.ReadFile(src)
	require.NoError(t, err)
	dst := path.Join("testdata", "copy_scalar_data_file")
	require.NoError(t, os.WriteFile(dst, contents, 0600))
	t.Cleanup(func() {
		// It should be removed prior to this so an error is expected.
		assert.Error(t, os.Remove(dst))
	})

	r, err := s.Retrieve(ctx, dst, nil)

	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, []byte("42"), r.Value())

	_, ok := r.(configsource.Watchable)
	assert.False(t, ok)
}

func TestIncludeConfigSource_DeleteFileError(t *testing.T) {
	if runtime.GOOS != "windows" {
		// Locking the file is trivial on Windows, but not on *nix given the
		// golang API, run the test only on Windows.
		t.Skip("Windows only test")
	}

	source, err := newConfigSource(configprovider.CreateParams{}, &Config{DeleteFiles: true})
	require.NoError(t, err)
	require.NotNil(t, source)

	ctx := context.Background()
	defer func() {
		assert.NoError(t, source.Close(ctx))
	}()

	// Copy test file
	src := path.Join("testdata", "scalar_data_file")
	contents, err := os.ReadFile(src)
	require.NoError(t, err)
	dst := path.Join("testdata", "copy_scalar_data_file")
	require.NoError(t, os.WriteFile(dst, contents, 0600))
	f, err := os.OpenFile(dst, os.O_RDWR, 0)
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, f.Close())
		assert.NoError(t, os.Remove(dst))
	})

	r, err := source.Retrieve(ctx, dst, nil)
	assert.IsType(t, &errFailedToDeleteFile{}, err)
	assert.Nil(t, r)
}
