package proxy

import (
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/meln5674/minimux"

	"github.com/docker/docker/api/types/image"
	docker "github.com/docker/docker/client"
)

type Config struct {
	// Docker is the client connected to the docker daemon
	Docker *docker.Client
	// Prefixes is a set of image ref prefixes that are proxied by this registry
	Prefixes map[string]struct{}
}

type Registry struct {
	Config
	// blobIndex is a map from image layer blob IDs to a second map from image IDs to the image metadata.
	blobIndex map[string]map[string]*image.InspectResponse
	// manifestCache is a map from image ID to the parsed oci image manifest descriptor
	manifestCache map[string]cachedManifest
	// indexLock must be held when using the index
	indexLock sync.RWMutex
	// cacheLock must be held when using the cache
	cacheLock sync.RWMutex
}

func New(cfg Config) *Registry {
	return &Registry{
		Config:        cfg,
		blobIndex:     map[string]map[string]*image.InspectResponse{},
		manifestCache: map[string]cachedManifest{},
	}
}

func (r *Registry) HasAllowedPrefix(name string) bool {
	if r.Prefixes == nil {
		return true
	}
	for prefix := range r.Prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func (r *Registry) BuildHandler() http.Handler {
	mux := minimux.Mux{
		DefaultHandler: minimux.NotFound,
		PreProcess:     minimux.PreProcessorChain(minimux.CancelWhenDone, minimux.LogPendingRequest(os.Stderr)),
		PostProcess:    minimux.LogCompletedRequest(os.Stderr),
		Routes: []minimux.Route{
			minimux.
				LiteralPath("/v2/").
				WithMethods(http.MethodGet).
				IsHandledByFunc(r.end_1),
			minimux.
				PathWithVars("/v2/(.+)/blobs/([^/]+)", "name", "digest").
				WithMethods(http.MethodGet, http.MethodHead).
				IsHandledByFunc(r.end_2),
			minimux.
				PathWithVars("/v2/(.+)/manifests/([^/]+)", "name", "reference").
				WithMethods(http.MethodGet, http.MethodHead).
				IsHandledByFunc(r.end_3),
			minimux.
				PathWithVars("/v2/(.+)/blobs/uploads/", "name").
				WithMethods(http.MethodPost).
				IsHandledByFunc(r.end_4a_4b_11),
			minimux.
				PathWithVars("/v2/(.+)/blobs/uploads/([^/]+)", "name", "reference").
				WithMethods(http.MethodPatch).
				IsHandledByFunc(r.end_5),
			minimux.
				PathWithVars("/v2/(.+)/blobs/uploads/([^/]+)", "name").
				WithMethods(http.MethodPut).
				IsHandledByFunc(r.end_6),
			minimux.
				PathWithVars("/v2/(.+)/manifests/([^/]+)", "name", "reference").
				WithMethods(http.MethodPut).
				IsHandledByFunc(r.end_7),
			minimux.
				PathWithVars("/v2/(.+)/tags", "name").
				WithMethods(http.MethodGet).
				IsHandledByFunc(r.end_8a_8b),
			minimux.
				PathWithVars("/v2/(.+)/manifests/([^/]+)", "name", "reference").
				WithMethods(http.MethodDelete).
				IsHandledByFunc(r.end_9),
			minimux.
				PathWithVars("/v2/(.+)/blobs/([^/]+)", "name", "digest").
				WithMethods(http.MethodDelete).
				IsHandledByFunc(r.end_12a_12b),
			minimux.
				PathWithVars("/v2/(.+)/blobs/uploads/([^/]+)", "name", "reference").
				WithMethods(http.MethodDelete).
				IsHandledByFunc(r.end_13),
		},
	}
	// TODO: Do we need this?
	// mux.HandleFunc("GET /v2/{name}/referrers/{digest}", r.end_12a_12b)

	return &mux
}
