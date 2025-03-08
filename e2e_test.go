package main_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
	docker "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"

	"github.com/meln5674/go-tlstest"
	"github.com/meln5674/oci-reg-docker/pkg/proxy"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestOciRegDocker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OCI Registry from Docker Suite")
}

var (
	testImage = os.Getenv("TEST_IMAGE")
	cert      = tlstest.Cert{
		IPStrings: []string{"127.0.0.1"},
	}
)

var _ = BeforeSuite(func() {
	tmp := GinkgoT().TempDir()
	cert.CertPath = filepath.Join(tmp, "srv.pem")
	Expect(cert.Generate()).To(Succeed())
})

var _ = Describe("OCI Registry", func() {
	var innerClient, outerClient *docker.Client
	var reg *proxy.Registry
	var srv *httptest.Server
	var srvHost string
	BeforeEach(func(ctx context.Context) {
		tmp := GinkgoT().TempDir()
		GinkgoLogr.Info("tmp", "path", tmp)
		DeferCleanup(func() {
			Expect(os.RemoveAll(tmp)).To(Succeed())
		})

		// GinkgoLogr.Info("starting inner podman")
		// innerSocket := startPodman(ctx, filepath.Join(tmp, "inner"), true)
		GinkgoLogr.Info("starting outer podman")
		outerSocket := startPodman(ctx, filepath.Join(tmp, "outer"), false)

		var err error
		innerClient, err = docker.NewClientWithOpts(
			docker.FromEnv,
			// docker.WithHost(innerSocket.String()),
		)
		Expect(err).ToNot(HaveOccurred())

		outerClient, err = docker.NewClientWithOpts(
			docker.WithHost(outerSocket.String()),
		)
		Expect(err).ToNot(HaveOccurred())

		reg = proxy.New(proxy.Config{
			Docker: innerClient,
		})

		srv = cert.NewHTTPSServer(reg.BuildHandler())
		DeferCleanup(srv.Close)
		srvURL, err := url.Parse(srv.URL)
		Expect(err).ToNot((HaveOccurred()))
		srvHost = srvURL.Host
	})

	It("should pull an image indexed by the inner docker", func(ctx context.Context) {
		// f, err := os.Open("test-image.tar")
		// Expect(err).ToNot(HaveOccurred())
		// defer f.Close()
		// _, err = innerClient.ImageLoad(ctx, f)
		// Expect(err).ToNot(HaveOccurred())

		toPull := srvHost + "/" + testImage
		GinkgoLogr.Info("pulling", "image", toPull)

		resp, err := outerClient.ImagePull(ctx, toPull, image.PullOptions{})
		Expect(err).ToNot(HaveOccurred())
		defer resp.Close()
		dec := json.NewDecoder(io.TeeReader(resp, GinkgoWriter))
		var msg jsonmessage.JSONMessage
		for {
			err := dec.Decode(&msg)
			if errors.Is(err, io.EOF) {
				break
			}
			Expect(err).ToNot(HaveOccurred())
			Expect(msg.Error).To(BeNil())
		}
	})
})

func startPodman(ctx context.Context, tmp string, inner bool) (podmanSocket *url.URL) {
	podmanSocket = &url.URL{Scheme: "unix", Path: filepath.Join(tmp, "podman.sock")}

	podmanRoot := filepath.Join(tmp, "/var/lib/podman")
	podmanRunRoot := filepath.Join(tmp, "/var/run/podman")
	podmanTmp := filepath.Join(tmp, "/tmp/podman")
	Expect(os.MkdirAll(podmanRoot, 0o700)).To(Succeed())
	Expect(os.MkdirAll(podmanRunRoot, 0o700)).To(Succeed())
	podmanCtx, stopPodman := context.WithCancel(context.Background())
	DeferCleanup(stopPodman)
	podman := exec.CommandContext(
		podmanCtx,
		"podman", "system", "service",
		"--root", podmanRoot,
		"--runroot", podmanRunRoot,
		"--tmpdir", podmanTmp,
		"--log-level", "debug",
		"--time", "0",
		podmanSocket.String(),
	)
	podman.Stdout = GinkgoWriter
	podman.Stderr = GinkgoWriter
	podman.Dir = tmp
	if !inner {
		podman.Env = append(podman.Env, os.Environ()...)
		podman.Env = append(podman.Env, "SSL_CERT_FILE="+cert.CertPath)
	}

	Expect(podman.Start()).To(Succeed())

	DeferCleanup(func(ctx context.Context) {
		podmanReset := exec.CommandContext(
			ctx,
			"podman",
			"system", "reset", "--force",
			"--root", podmanRoot,
			"--runroot", podmanRunRoot,
			"--tmpdir", podmanTmp,
		)
		podmanReset.Stdout = GinkgoWriter
		podmanReset.Stderr = GinkgoWriter
		Expect(podmanReset.Run()).To(Succeed())
	})

	Eventually(func() error {
		_, err := os.Stat(podmanSocket.Path)
		if err != nil {
			return err
		}
		ctx, stop := context.WithDeadline(ctx, time.Now().Add(5*time.Second))
		defer stop()
		docker := exec.CommandContext(ctx, "docker", "version")
		copy(docker.Env, os.Environ())
		docker.Env = append(docker.Env, fmt.Sprintf("DOCKER_HOST=%s", podmanSocket.String()))
		docker.Stdout = GinkgoWriter
		docker.Stderr = GinkgoWriter
		return docker.Run()
	}, "30s", "5s").Should(Succeed())

	return
}
