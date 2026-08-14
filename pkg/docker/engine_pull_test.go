package docker

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

type pullMockClient struct {
	inspectErr error
	pulled     []string
}

func (m *pullMockClient) ImageInspect(_ context.Context, _ string, _ ...client.ImageInspectOption) (image.InspectResponse, error) {
	if m.inspectErr != nil {
		return image.InspectResponse{}, m.inspectErr
	}
	return image.InspectResponse{}, nil
}

func (m *pullMockClient) ImagePull(_ context.Context, ref string, _ image.PullOptions) (io.ReadCloser, error) {
	m.pulled = append(m.pulled, ref)
	return io.NopCloser(bytes.NewReader(nil)), nil
}

func (m *pullMockClient) ContainerCreate(_ context.Context, _ *container.Config, _ *container.HostConfig, _ *network.NetworkingConfig, _ *v1.Platform, _ string) (container.CreateResponse, error) {
	panic("not implemented")
}
func (m *pullMockClient) ContainerStart(_ context.Context, _ string, _ container.StartOptions) error {
	panic("not implemented")
}
func (m *pullMockClient) ContainerList(_ context.Context, _ container.ListOptions) ([]container.Summary, error) {
	panic("not implemented")
}
func (m *pullMockClient) ContainerStop(_ context.Context, _ string, _ container.StopOptions) error {
	panic("not implemented")
}
func (m *pullMockClient) ContainerRemove(_ context.Context, _ string, _ container.RemoveOptions) error {
	panic("not implemented")
}
func (m *pullMockClient) ContainerInspect(_ context.Context, _ string) (container.InspectResponse, error) {
	panic("not implemented")
}

func TestClickhouseServerImage(t *testing.T) {
	t.Parallel()

	t.Run("empty image uses version alpine", func(t *testing.T) {
		t.Parallel()
		got := clickhouseServerImage(DockerOptions{Version: "25.7"})
		require.Equal(t, "clickhouse/clickhouse-server:25.7-alpine", got)
	})

	t.Run("empty image and version uses latest alpine", func(t *testing.T) {
		t.Parallel()
		got := clickhouseServerImage(DockerOptions{})
		require.Equal(t, "clickhouse/clickhouse-server:latest-alpine", got)
	})

	t.Run("explicit image overrides version alpine", func(t *testing.T) {
		t.Parallel()
		got := clickhouseServerImage(DockerOptions{
			Version: "25.7",
			Image:   "localhost/clickhouse-server-udf:26.1.3.52",
		})
		require.Equal(t, "localhost/clickhouse-server-udf:26.1.3.52", got)
	})
}

func TestEnginePull_SkipsWhenLocal(t *testing.T) {
	t.Parallel()

	mock := &pullMockClient{}
	eng := newEngine(mock)
	err := eng.Pull(context.Background(), "localhost/clickhouse-server-udf:local")
	require.NoError(t, err)
	require.Empty(t, mock.pulled)
}

func TestEnginePull_PullsWhenMissing(t *testing.T) {
	t.Parallel()

	mock := &pullMockClient{inspectErr: errors.New("No such image")}
	eng := newEngine(mock)
	ref := "clickhouse/clickhouse-server:25.7-alpine"
	err := eng.Pull(context.Background(), ref)
	require.NoError(t, err)
	require.Equal(t, []string{ref}, mock.pulled)
}
