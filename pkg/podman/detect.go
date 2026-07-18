package podman

import (
	"context"
	"os"
	"strings"
	"sync"

	F "github.com/IBM/fp-go/v2/function"
	O "github.com/IBM/fp-go/v2/option"
	"github.com/pseudomuto/housekeeper/pkg/docker"
)

const housekeeperRuntimeEnv = "HOUSEKEEPER_RUNTIME"

// AutoDetectResolver picks the right AddressResolver on first use and caches it.
//
// Detection order (highest to lowest priority):
//  1. HOUSEKEEPER_RUNTIME=podman|docker — explicit override, always wins.
//  2. CONTAINER_HOST / DOCKER_HOST — if URI contains "podman":
//     - rootless (/run/user/…) → DockerAddressResolver (localhost + published ports)
//     - rootful → PodmanAddressResolver (bridge IP)
//  3. Rootful Podman socket on disk → PodmanAddressResolver.
//  4. Default: DockerAddressResolver.
type AutoDetectResolver struct {
	once     sync.Once
	resolved docker.AddressResolver
}

// NewAutoDetectResolver creates an AddressResolver that detects the container runtime
// automatically on first use.
func NewAutoDetectResolver() docker.AddressResolver {
	return &AutoDetectResolver{}
}

func (r *AutoDetectResolver) Resolve(ctx context.Context, client docker.DockerClient, containerName string) (*docker.ContainerAddress, error) {
	r.once.Do(func() { r.resolved = detectResolver() })
	return r.resolved.Resolve(ctx, client, containerName)
}

func detectResolver() docker.AddressResolver {
	return F.Pipe4(
		resolverFromRuntime(),
		O.Alt(envToAddressResolver("CONTAINER_HOST")),
		O.Alt(envToAddressResolver("DOCKER_HOST")),
		O.Alt(func() O.Option[docker.AddressResolver] {
			// Only rootful Podman sockets need bridge-IP resolution. A rootless
			// user socket alone must fall through to DockerAddressResolver
			// (localhost + published ports) — pasta has no host-reachable bridge IP.
			return O.MonadMap(
				O.FromNonZero[bool]()(rootfulPodmanSocketExists()),
				F.Constant1[bool, docker.AddressResolver](&PodmanAddressResolver{}),
			)
		}),
		O.GetOrElse(func() docker.AddressResolver { return &docker.DockerAddressResolver{} }),
	)
}

func resolverFromRuntime() O.Option[docker.AddressResolver] {
	switch strings.ToLower(os.Getenv(housekeeperRuntimeEnv)) {
	case "podman":
		return O.Some[docker.AddressResolver](&PodmanAddressResolver{})
	case "docker":
		return O.Some[docker.AddressResolver](&docker.DockerAddressResolver{})
	default:
		return O.None[docker.AddressResolver]()
	}
}

// envToAddressResolver maps a container-socket env var to the correct AddressResolver.
// Rootless Podman (pasta) exposes published ports on 127.0.0.1 but leaves bridge
// NetworkSettings.IPAddress empty/unreachable — DockerAddressResolver is correct there.
// Rootful Podman needs PodmanAddressResolver (bridge IP).
func envToAddressResolver(envVar string) func() O.Option[docker.AddressResolver] {
	return func() O.Option[docker.AddressResolver] {
		s := os.Getenv(envVar)
		if s == "" {
			return O.None[docker.AddressResolver]()
		}
		lower := strings.ToLower(s)
		if !strings.Contains(lower, "podman") {
			return O.None[docker.AddressResolver]()
		}
		if strings.Contains(lower, "/run/user/") {
			return O.Some[docker.AddressResolver](&docker.DockerAddressResolver{})
		}
		return O.Some[docker.AddressResolver](&PodmanAddressResolver{})
	}
}

// rootfulPodmanSocketExists probes the system Podman socket only.
// Rootless paths under /run/user/… are intentionally excluded (see detectResolver).
func rootfulPodmanSocketExists() bool {
	candidates := []string{
		"/run/podman/podman.sock",
		"/var/run/podman/podman.sock",
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}
