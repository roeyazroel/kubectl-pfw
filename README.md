# kubectl-pfw

A kubectl plugin for port-forwarding multiple services or pods at once.

## Features

- Select and port-forward multiple services or pods simultaneously
- Interactive multi-select interface for easy selection
- Support for services, pods, deployments, and statefulsets
- Auto-reconnect and retry on connection failures
- Ephemeral port allocation (let the system choose available ports)
- Configuration files for reusable port forwarding setups
- Remains active until terminated with Ctrl+C
- Forwards to the correct target container port for services (handling named ports)

## How It Works

### Service Port Forwarding

When you select a service to port-forward, the plugin:

1. Finds pods that match the service's selector
2. Selects one of the matching pods
3. Uses the pod's container port (the service's targetPort) to establish the port-forward
4. The connection appears as if it's directly to the service

This approach is more reliable than using service proxy endpoints, which may not work correctly with certain protocols.

### Deployment/StatefulSet Port Forwarding

When you select a deployment or statefulset to port-forward, the plugin:

1. Finds pods that are managed by the deployment/statefulset
2. Selects one of the matching pods
3. Establishes a port-forward connection to the pod
4. Maps the container port to the local port (either specified or automatically assigned)

### Pod Port Forwarding

When you select a pod to port-forward, the plugin:

1. Directly establishes a port-forward connection to the pod
2. Maps the container port to the same local port number by default, or to a specified port

## Installation

### Prerequisites

- Go 1.21 or later
- kubectl

### Installing from source

1. Clone the repository:

```bash
git clone https://github.com/roeyazroel/kubectl-pfw.git
cd kubectl-pfw
```

2. Build the plugin:

```bash
go build -o kubectl-pfw cmd/kubectl-pfw/main.go
```

3. Move the binary to a directory in your PATH:

```bash
chmod +x kubectl-pfw
mv kubectl-pfw /usr/local/bin/
```

4. Verify the installation:

```bash
kubectl plugin list
```

You should see `kubectl-pfw` in the list of available plugins.

## Usage

### Port forward services

```bash
kubectl pfw
```

This will display an interactive list of services in the current namespace. Use the arrow keys to navigate, space to select services, and enter to confirm your selection.

### Port forward services in a specific namespace

```bash
kubectl pfw -n mynamespace
```

### Port forward pods instead of services

```bash
kubectl pfw --pods
```

### Show version information

```bash
kubectl pfw -v
# or
kubectl pfw --version
```

### Generate a configuration file for reuse

```bash
kubectl pfw --generate-config --output my-config.yaml
```

This will guide you through selecting resources interactively and specifying port mappings, then save the configuration to a file for later use.

You can also generate configs for pods:

```bash
kubectl pfw --pods --generate-config
```

### Use a configuration file for consistent port-forwarding

```bash
kubectl pfw -f my-config.yaml
```

Example configuration file:

```yaml
# Optional: specify a default namespace
defaultNamespace: my-namespace
# List of resources to forward
resources:
  - resourceType: service # service, pod, deployment, or statefulset
    name: my-service
    # Optional: namespace (overrides defaultNamespace)
    namespace: custom-namespace
    ports:
      - localPort: 8080 # Local port to use (0 for auto-assign)
        remotePort: 80 # Remote port on the resource
  - resourceType: pod
    name: my-pod
    ports:
      - localPort: 0 # Auto-assign a local port
        remotePort: 8080
  - resourceType: deployment
    name: my-deployment
    ports:
      - localPort: 5000
        remotePort: 5000
```

## Troubleshooting

### Port Already In Use

If you see errors like:

```
Unable to listen on port 8080: Listeners failed to create with the following errors: [unable to create listener: Error listen tcp4 127.0.0.1:8080: bind: address already in use]
```

The port is already being used by another process. You can:

1. Use a different port by entering a different local port when prompted
2. Specify `0` for the local port when prompted to let the system auto-assign an available port
3. Use a configuration file with `localPort: 0` to enable auto-assignment

### Service Port-Forwarding Issues

For service port-forwarding to work properly:

- The service must have a selector to find backing pods
- At least one pod matching the selector must be running

## Development

### Project Structure

```
kubectl-pfw/
├── cmd/
│   └── kubectl-pfw/           # Main entry point
│       └── main.go
├── pkg/
│   ├── cli/                   # Command-line interface handling
│   ├── portforward/           # Port forwarding logic
│   │   ├── portforward.go     # Service/pod forwarding implementation
│   │   ├── manager.go         # Manages multiple port forwards
│   │   └── port_allocator.go  # Dynamic port allocation
│   ├── config/                # Configuration file handling
│   │   └── config.go          # Configuration file parsing/validation
│   ├── k8s/                   # Kubernetes client interactions
│   │   ├── client.go          # Client setup
│   │   ├── services.go        # Service listing/selection
│   │   ├── pods.go            # Pod listing/selection
│   │   ├── deployments.go     # Deployment handling
│   │   └── statefulsets.go    # StatefulSet handling
│   └── ui/                    # User interface components
│       └── selector.go        # Multi-select implementation
```

### Building

```bash
go build -o kubectl-pfw cmd/kubectl-pfw/main.go
```

## License

MIT License

## Releasing

This project uses [GoReleaser](https://goreleaser.com/) to build and release new versions.

### Release Process

1. Make your changes and commit them
2. Update version references in the code if necessary
3. Create and push a new tag:
   ```bash
   git tag -a v0.1.0 -m "First release"
   git push origin v0.1.0
   ```
4. GitHub Actions will automatically build and publish:
   - Cross-platform binaries (Linux, macOS, Windows)
   - Homebrew formula
   - Krew plugin manifest
   - Release artifacts on GitHub Releases page

### Installing Released Versions

#### Homebrew (macOS)

```bash
brew tap roeyazroel/tap
brew install kubectl-pfw
```

#### Manual Installation

Download the appropriate binary for your platform from the [releases page](https://github.com/roeyazroel/kubectl-pfw/releases).
