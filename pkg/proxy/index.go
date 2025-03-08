package proxy

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
)

// BuildIndex builds the blob to manifest index.
// Docker only allows listing and retreiving images, not layers/blobs, so the proxy must maintain its own
// index mapping blobs to manifests.
// On startup, this index is empty, but calling BuildIndex will list all images with the provided prefixes
// (or all images if no prefixes are provided), and build this index.
// This is slow, however, any successful /v2/{name}/manifests/{reference} will also index that particular image,
// so this call should only be needed if the API is used directly to fetch blobs without first obtaining a manifest.
func (r *Registry) BuildIndex(ctx context.Context) error {
	r.indexLock.Lock()
	defer r.indexLock.Unlock()
	return r.buildIndex(ctx)
}

func (r *Registry) buildIndex(ctx context.Context) error {
	if r.Prefixes == nil {
		return r.buildIndexForPrefix(ctx, "")
	}
	for prefix := range r.Prefixes {
		err := r.buildIndexForPrefix(ctx, prefix)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) buildIndexForPrefix(ctx context.Context, prefix string) error {
	imgSums, err := r.Docker.ImageList(ctx, image.ListOptions{Filters: filters.NewArgs(filters.KeyValuePair{"reference", prefix + "*:*"})})
	if err != nil {
		return fmt.Errorf("listing images with prefix %s: %w", prefix, err)
	}
	for _, imgSum := range imgSums {
		imgSum := imgSum
		img, err := r.Docker.ImageInspect(ctx, imgSum.ID)
		if err != nil {
			return fmt.Errorf("inspecting image %s: %w", imgSum.ID, err)
		}
		r.addImageToIndex(&img)
	}
	return nil
}

func (r *Registry) addImageToIndex(img *image.InspectResponse) {
	for _, blobID := range img.RootFS.Layers {
		r.addBlobToIndex(blobID, img)
	}
	r.addBlobToIndex(img.ID, img)
}

func (r *Registry) addBlobToIndex(blobID string, img *image.InspectResponse) {
	imgs, ok := r.blobIndex[blobID]
	if !ok {
		imgs = make(map[string]*image.InspectResponse)
		r.blobIndex[blobID] = imgs
	}
	imgs[img.ID] = img
	slog.Info("indexed layer", "blobID", blobID, "imageID", img.ID)
}
