package proxy

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"

	ocidist "github.com/opencontainers/distribution-spec/specs-go/v1"
)

func (r *Registry) end_1(_ context.Context, w http.ResponseWriter, rq *http.Request, pathVars map[string]string, formErr error) error {
	w.WriteHeader(http.StatusOK)
	return nil
}

func (r *Registry) end_2(ctx context.Context, w http.ResponseWriter, rq *http.Request, pathVars map[string]string, formErr error) error {
	name := pathVars["name"]
	digest := pathVars["digest"]

	if !r.HasAllowedPrefix(name) {
		w.WriteHeader(http.StatusNotFound)
		return fmt.Errorf("does not have allowed prefix")
	}

	r.indexLock.RLock()
	defer r.indexLock.RUnlock()
	imgs, ok := r.blobIndex[digest]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return fmt.Errorf("digest not in blob index")
	}

	var imgID string
imgs:
	for id, img := range imgs {
		for _, tag := range img.RepoTags {
			switch strings.Count(tag, "/") {
			case 0:
				tag = "docker.io/library/" + tag
			case 1:
				tag = "docker.io/" + tag
			}
			if strings.HasPrefix(tag, name+":") {
				imgID = id
				break imgs
			}
		}
	}
	if imgID == "" {
		w.WriteHeader(http.StatusNotFound)
		return fmt.Errorf("digest was not indexed as belonging to this repo")
	}

	imgTar, err := r.Docker.ImageSave(ctx, []string{imgID})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return err
	}
	defer imgTar.Close()
	imgTarR := tar.NewReader(imgTar)
	for {
		h, err := imgTarR.Next()
		if errors.Is(err, io.EOF) {
			w.WriteHeader(http.StatusInternalServerError)
			err := fmt.Errorf("saved image tarball did not contain expected blob")
			w.Write([]byte(err.Error()))
			return err
		}
		if h.Name != "blobs/"+strings.ReplaceAll(digest, ":", "/") {
			continue
		}
		w.Header().Add("Content-Length", fmt.Sprintf("%d", h.Size))
		_, err = io.Copy(w, imgTarR)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return err
		}
		return nil
	}
}

func (r *Registry) end_3(ctx context.Context, w http.ResponseWriter, rq *http.Request, pathVars map[string]string, formErr error) error {
	name := pathVars["name"]
	reference := pathVars["reference"]

	if !r.HasAllowedPrefix(name) {
		w.WriteHeader(http.StatusNotFound)
		return fmt.Errorf("does not have allowed prefix")
	}

	var imgID string
	if strings.HasPrefix(reference, "sha256:") {
		imgID = name + "@" + reference
	} else {
		imgID = name + ":" + reference
	}

	img, err := r.Docker.ImageInspect(ctx, imgID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return err
	}

	manifest, err := r.getAndCacheManifest(ctx, &img)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return err
	}

	w.Header().Add("Content-Length", fmt.Sprintf("%d", len(manifest.JSON)))
	w.Header().Add("Content-Type", manifest.Manifest.MediaType)
	_, err = w.Write(manifest.JSON)
	return err
}

func (r *Registry) end_4a_4b_11(_ context.Context, w http.ResponseWriter, rq *http.Request, pathVars map[string]string, formErr error) error {
	w.WriteHeader(http.StatusForbidden)
	return nil
}
func (r *Registry) end_5(_ context.Context, w http.ResponseWriter, rq *http.Request, pathVars map[string]string, formErr error) error {
	w.WriteHeader(http.StatusForbidden)
	return nil
}
func (r *Registry) end_6(_ context.Context, w http.ResponseWriter, rq *http.Request, pathVars map[string]string, formErr error) error {
	w.WriteHeader(http.StatusForbidden)
	return nil
}
func (r *Registry) end_7(_ context.Context, w http.ResponseWriter, rq *http.Request, pathVars map[string]string, formErr error) error {
	w.WriteHeader(http.StatusForbidden)
	return nil
}
func (r *Registry) end_8a_8b(ctx context.Context, w http.ResponseWriter, rq *http.Request, pathVars map[string]string, formErr error) error {
	name := pathVars["name"]

	if !r.HasAllowedPrefix(name) {
		w.WriteHeader(http.StatusNotFound)
		return fmt.Errorf("does not have allowed prefix")
	}

	q := rq.URL.Query()
	nStr := q.Get("n")
	var n int
	if nStr != "" {
		_, err := fmt.Sscanf(nStr, "%d", &n)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return err
		}
	}
	// TODO: Implement last and Link
	// last := q.Get("last")

	imgSums, err := r.Docker.ImageList(ctx, image.ListOptions{Filters: filters.NewArgs(filters.KeyValuePair{"reference", name + ":*"})})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return err
	}

	tags := ocidist.TagList{Name: name}
	tagSet := make(map[string]struct{})
	for _, imgSum := range imgSums {
		for _, repoTag := range imgSum.RepoTags {
			if !strings.HasPrefix(repoTag, name+":") {
				continue
			}
			tag := strings.TrimPrefix(repoTag, name+":")
			if _, ok := tagSet[tag]; ok {
				continue
			}
			tagSet[tag] = struct{}{}
			tags.Tags = append(tags.Tags, tag)
		}
		if n != 0 && len(tagSet) >= n {
			break
		}
	}

	return json.NewEncoder(w).Encode(&tags)
}
func (r *Registry) end_9(_ context.Context, w http.ResponseWriter, _ *http.Request, _ map[string]string, _ error) error {
	w.WriteHeader(http.StatusForbidden)
	return nil
}
func (r *Registry) end_10(_ context.Context, w http.ResponseWriter, _ *http.Request, _ map[string]string, _ error) error {
	w.WriteHeader(http.StatusForbidden)
	return nil
}
func (r *Registry) end_11(_ context.Context, w http.ResponseWriter, _ *http.Request, _ map[string]string, _ error) error {
	w.WriteHeader(http.StatusForbidden)
	return nil
}
func (r *Registry) end_12a_12b(_ context.Context, w http.ResponseWriter, _ *http.Request, _ map[string]string, _ error) error {
	w.WriteHeader(http.StatusForbidden)
	return nil
}
func (r *Registry) end_13(_ context.Context, w http.ResponseWriter, _ *http.Request, _ map[string]string, _ error) error {
	w.WriteHeader(http.StatusForbidden)
	return nil
}
