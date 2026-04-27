// healthcheck is a minimal binary used as the Docker HEALTHCHECK for the proxy
// container. distroless images have no shell or curl, so a dedicated binary is
// required. It exits 0 if the metrics server responds 200, 1 otherwise.
package main

import (
	"net/http"
	"os"
)

func main() {
	resp, err := http.Get("http://localhost:9090/healthz")
	if err != nil || resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
}
