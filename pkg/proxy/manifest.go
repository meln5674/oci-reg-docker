package proxy

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/docker/docker/api/types/image"

	ociimage "github.com/opencontainers/image-spec/specs-go/v1"
)

type cachedManifest struct {
	JSON     json.RawMessage
	Manifest ociimage.Manifest
}

func (r *Registry) getManifest(ctx context.Context, img *image.InspectResponse) (manifest cachedManifest, err error) {
	imgTarStream, err := r.Docker.ImageSave(ctx, []string{img.ID})
	if err != nil {
		err = fmt.Errorf("requesting upstream tarball: %w", err)
		return
	}
	imgTar := tar.NewReader(imgTarStream)
	var h *tar.Header

	smallBlobs := make(map[string][]byte)
	const smallBlobCap = 512 * 1024 * 1024 // 512KiB

	var layout ociimage.ImageLayout
	var index ociimage.Index
	for {
		h, err = imgTar.Next()
		if errors.Is(err, io.EOF) {
			err = nil
			break
		}
		if err != nil {
			err = fmt.Errorf("reading upstream tarball: %w", err)
			return
		}
		slog.Info("upstream tarball entry", "name", h.Name, "type", h.Typeflag)
		switch h.Name {
		case ociimage.ImageLayoutFile:
			err = json.NewDecoder(imgTar).Decode(&layout)
			if err != nil {
				err = fmt.Errorf("daemon save tarball contained invalid layout: %w", err)
				return
			}
		case ociimage.ImageIndexFile:
			err = json.NewDecoder(imgTar).Decode(&index)
			if err != nil {
				err = fmt.Errorf("daemon save tarball contained invalid index: %w", err)
				return
			}
		default:
			if h.Size > smallBlobCap {
				continue
			}
			if !strings.HasPrefix(h.Name, ociimage.ImageBlobsDir+"/") {
				continue
			}
			digest := strings.ReplaceAll(strings.TrimPrefix(h.Name, ociimage.ImageBlobsDir+"/"), "/", ":")
			var buf bytes.Buffer
			_, err = io.Copy(&buf, imgTar)
			if err != nil {
				err = fmt.Errorf("failed reading potential manifest blob: %w", err)
				return
			}
			smallBlobs[digest] = buf.Bytes()
		}
	}

	if len(index.Manifests) == 0 {
		err = fmt.Errorf("upstream tarball did not contain any manifests")
		return
	}
	slog.Info("upstream tarball manifests", "manifests", index.Manifests)

	for _, descriptor := range index.Manifests {
		manifestJSON, ok := smallBlobs[string(descriptor.Digest)]
		if !ok {
			err = fmt.Errorf("daemon save tarball did not contain manifest blob %s or it was very big", descriptor.Digest)
			return
		}
		possibleManifest := cachedManifest{
			JSON: json.RawMessage(manifestJSON),
		}
		err = json.Unmarshal(manifestJSON, &possibleManifest.Manifest)
		if err != nil {
			err = fmt.Errorf("daemon save tarball contained invalid manifest blob %s: %w", descriptor.Digest, err)
			return
		}
		if string(possibleManifest.Manifest.Config.Digest) == img.ID {
			manifest = possibleManifest
			slog.Info("found manifest", "id", img.ID, "manifest", manifest)
			return
		}
	}
	err = fmt.Errorf("upstream tarball did not contain expected manifest with ID %s", img.ID)
	return
}

func (r *Registry) getAndCacheManifest(ctx context.Context, img *image.InspectResponse) (manifest cachedManifest, err error) {
	r.cacheLock.RLock()
	defer r.cacheLock.RUnlock()
	var ok bool
	manifest, ok = r.manifestCache[img.ID]
	if ok {
		return
	}
	r.cacheLock.RUnlock()
	defer r.cacheLock.RLock()
	r.cacheLock.Lock()
	defer r.cacheLock.Unlock()
	manifest, err = r.getManifest(ctx, img)
	if err != nil {
		return
	}
	r.manifestCache[img.ID] = manifest
	r.indexLock.Lock()
	defer r.indexLock.Unlock()
	r.addImageToIndex(img)

	return
}
