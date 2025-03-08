# OCI Registry From Docker (Name Pending)

## What

This tool implements the [OCI Distribution Specification](https://github.com/opencontainers/distribution-spec) by using a docker daemon as storage.

In other words, this tool turns your Docker Daemon into a Docker Registry.

Currently, this tool only supports read operations on already pulled images. Fetching images
from other registries and pushing new images are not yet supported.

## Why

This tool is primarily intended for use in testing scenarios to avoid re-pulling images 
that already exist in the local docker storage, while also avoiding storing a duplicate
copy in a locally deployed registry, when using a second, often nested, container runtime,
such as when testing Kubernetes using KinD or K3d.

## How

### Command line tool

To build and run the a stand-alone proxy registry, run

```
go build .
./oci-reg-docker
```

or run in a docker container with

```
docker build -t oci-reg-docker .
docker run -d -v /var/run/docker.sock:/var/run/docker.sock -p 8080:8080 oci-reg-docker
```

Use the following environment variables for configuration

| Variable | Use | Default |
| -------- | --- | ------- |
| REGISTRY_LISTEN_ADDR | Hostname:Port to listen on | 127.0.0.1:8080 |
| REGISTRY_KEY_PATH | Path to pem formatted private key file for TLS. TLS is disabled if not provided. | |
| REGISTRY_CERT_PATH | Path to pem formatted certificate file for TLS. Required if private key is provided. | |
| REGISTRY_PREFIXES | Space separated list of image name prefixes to allow. Requests for images that do not start with one of these prefixes will return 404. Omit to allow all images | |

Additionally, [These variables](https://pkg.go.dev/github.com/docker/docker/client#FromEnv) can be used to configure
the connection to the docker daemon, including a remote one.


### Golang Library

The registry is available to import as a golang library. See the following example for usage:

```go
package foo

import (
	docker "github.com/docker/docker/client"

  "github.com/meln5674/oci-reg-docker/pkg/proxy"
)

func example() error {
    // Connect to the docker daemon
    client, err := docker.NewClientWithOpts(docker.FromEnv)
    if err != nil { return err }

    // Configure the registry
    reg := proxy.New(proxy.Config{
      Docker: client,
      // Limit to certain image prefixes
      // Prefixes: map[string]struct{} { "docker.io/my-repo/": struct{}{} }
    })

    // This is not needed in normal usage, but if you are going to attempt to
    // fetch a blob before fetching the matching manifest, you will get a 404.
    // This function builds the mapping of layers to images so that blobs can be
    // fetched immediately after startup.
    // reg.BuildIndex(context.Background()) 

    // Start the server
    return http.ListenAndServe(":8080", reg.BuildHandler())
}

```
