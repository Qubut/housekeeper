package podman_test

import (
	"context"
	"os"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/pseudomuto/housekeeper/pkg/docker"
	"github.com/pseudomuto/housekeeper/pkg/podman"
	"github.com/stretchr/testify/require"
)

// skipIfRootfulPodmanSocket skips when a rootful Podman socket exists.
// Rootful sockets still select PodmanAddressResolver ahead of the Docker default.
func skipIfRootfulPodmanSocket(t *testing.T) {
	t.Helper()
	for _, p := range []string{"/run/podman/podman.sock", "/var/run/podman/podman.sock"} {
		if _, err := os.Stat(p); err == nil {
			t.Skipf("rootful Podman socket at %s — probe supersedes Docker DOCKER_HOST fallback", p)
		}
	}
}

// podmanInspect returns an InspectResponse with a bridge IP, mimicking Podman rootless.
func podmanInspect(ip string) container.InspectResponse {
	return container.InspectResponse{
		NetworkSettings: &container.NetworkSettings{
			DefaultNetworkSettings: container.DefaultNetworkSettings{
				IPAddress: ip,
			},
		},
	}
}

// dockerInspect returns an InspectResponse with port bindings, mimicking Docker Engine.
func dockerInspect(nativeHost, httpHost string) container.InspectResponse {
	return container.InspectResponse{
		NetworkSettings: &container.NetworkSettings{
			NetworkSettingsBase: container.NetworkSettingsBase{
				Ports: nat.PortMap{
					nat.Port("9000/tcp"): []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: nativeHost}},
					nat.Port("8123/tcp"): []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: httpHost}},
				},
			},
		},
	}
}

func TestAutoDetectResolver_PodmanRuntime_UsesBridgeIP(t *testing.T) {
	// HOUSEKEEPER_RUNTIME=podman must select PodmanAddressResolver, which reads the
	// container's direct bridge IP (not the mapped host port).
	t.Setenv("HOUSEKEEPER_RUNTIME", "podman")
	t.Setenv("CONTAINER_HOST", "")
	t.Setenv("DOCKER_HOST", "")

	cli := &mockDockerClient{
		inspectFn: func(_ context.Context, _ string) (container.InspectResponse, error) {
			return podmanInspect("10.88.0.5"), nil
		},
	}

	resolver := podman.NewAutoDetectResolver()
	addr, err := resolver.Resolve(context.Background(), cli, "ch-test")

	require.NoError(t, err)
	require.Equal(t, "10.88.0.5", addr.Host)
	require.Equal(t, docker.DefaultClickHousePort, addr.NativePort)
	require.Equal(t, docker.DefaultClickHouseHTTPPort, addr.HTTPPort)
}

func TestAutoDetectResolver_DockerRuntime_UsesPortBindings(t *testing.T) {
	// HOUSEKEEPER_RUNTIME=docker must select DockerAddressResolver, which reads
	// mapped host ports and returns 127.0.0.1:<hostPort> (IPv4-only rootless publish).
	t.Setenv("HOUSEKEEPER_RUNTIME", "docker")
	t.Setenv("CONTAINER_HOST", "")
	t.Setenv("DOCKER_HOST", "")

	cli := &mockDockerClient{
		inspectFn: func(_ context.Context, _ string) (container.InspectResponse, error) {
			return dockerInspect("54321", "54322"), nil
		},
	}

	resolver := podman.NewAutoDetectResolver()
	addr, err := resolver.Resolve(context.Background(), cli, "ch-test")

	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", addr.Host)
	require.Equal(t, 54321, addr.NativePort)
	require.Equal(t, 54322, addr.HTTPPort)
}

func TestAutoDetectResolver_RootlessPodmanSocket_UsesPortBindings(t *testing.T) {
	// Rootless DOCKER_HOST (/run/user/…) must NOT activate PodmanAddressResolver —
	// pasta has no host-reachable bridge IP; use published localhost ports.
	t.Setenv("HOUSEKEEPER_RUNTIME", "")
	t.Setenv("CONTAINER_HOST", "")
	t.Setenv("DOCKER_HOST", "unix:///run/user/1000/podman/podman.sock")

	cli := &mockDockerClient{
		inspectFn: func(_ context.Context, _ string) (container.InspectResponse, error) {
			return dockerInspect("40173", "34561"), nil
		},
	}

	resolver := podman.NewAutoDetectResolver()
	addr, err := resolver.Resolve(context.Background(), cli, "ch-test")

	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", addr.Host)
	require.Equal(t, 40173, addr.NativePort)
	require.Equal(t, 34561, addr.HTTPPort)
}

func TestAutoDetectResolver_RootfulDockerHostPodmanSocket_UsesBridgeIP(t *testing.T) {
	// Rootful DOCKER_HOST (/run/podman/…) activates the Podman bridge-IP resolver.
	t.Setenv("HOUSEKEEPER_RUNTIME", "")
	t.Setenv("CONTAINER_HOST", "")
	t.Setenv("DOCKER_HOST", "unix:///run/podman/podman.sock")

	cli := &mockDockerClient{
		inspectFn: func(_ context.Context, _ string) (container.InspectResponse, error) {
			return podmanInspect("10.90.0.3"), nil
		},
	}

	resolver := podman.NewAutoDetectResolver()
	addr, err := resolver.Resolve(context.Background(), cli, "ch-test")

	require.NoError(t, err)
	require.Equal(t, "10.90.0.3", addr.Host)
}

func TestAutoDetectResolver_DockerSocket_UsesPortBindings(t *testing.T) {
	// A Docker socket URI in DOCKER_HOST does NOT activate Podman — falls back to Docker.
	// Skip only when a *rootful* Podman socket exists (rootless no longer selects Podman).
	skipIfRootfulPodmanSocket(t)
	t.Setenv("HOUSEKEEPER_RUNTIME", "")
	t.Setenv("CONTAINER_HOST", "")
	t.Setenv("DOCKER_HOST", "unix:///var/run/docker.sock")

	cli := &mockDockerClient{
		inspectFn: func(_ context.Context, _ string) (container.InspectResponse, error) {
			return dockerInspect("32768", "32769"), nil
		},
	}

	resolver := podman.NewAutoDetectResolver()
	addr, err := resolver.Resolve(context.Background(), cli, "ch-test")

	require.NoError(t, err)
	require.Equal(t, "127.0.0.1", addr.Host)
	require.Equal(t, 32768, addr.NativePort)
}

func TestAutoDetectResolver_CachesResult(t *testing.T) {
	// sync.Once guarantees detection fires exactly once; calls after the first
	// must reuse the cached resolver — verified by flipping the env var midway.
	t.Setenv("HOUSEKEEPER_RUNTIME", "podman")
	t.Setenv("CONTAINER_HOST", "")
	t.Setenv("DOCKER_HOST", "")

	cli := &mockDockerClient{
		inspectFn: func(_ context.Context, _ string) (container.InspectResponse, error) {
			return podmanInspect("10.88.0.9"), nil
		},
	}

	resolver := podman.NewAutoDetectResolver()

	// First call triggers detection (podman selected).
	addr1, err := resolver.Resolve(context.Background(), cli, "ch-test")
	require.NoError(t, err)
	require.Equal(t, "10.88.0.9", addr1.Host)

	// Switch env var — second call must still use the cached Podman resolver.
	t.Setenv("HOUSEKEEPER_RUNTIME", "docker")
	addr2, err := resolver.Resolve(context.Background(), cli, "ch-test")
	require.NoError(t, err)
	require.Equal(t, "10.88.0.9", addr2.Host, "cached resolver must ignore env change")
}
