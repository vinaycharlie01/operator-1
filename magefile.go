//go:build mage

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/magefile/mage/sh"
)

var (
	// Default build variables
	// Default = Build
	ldflags = ""
)

func init() {
	// Set default LDFLAGS
	version := getVersion()
	commitID := getCommitID()
	ldflags = fmt.Sprintf("-s -w -X github.com/minio/operator/pkg.Version=%s -X github.com/minio/operator/pkg.ShortCommitID=%s", version, commitID)
}

// Build builds both operator and gateway-controller binaries for current platform
func Build() error {
	fmt.Println("Building operator and gateway-controller...")
	if err := BuildOperatorBinary(); err != nil {
		return err
	}
	return BuildGatewayBinary()
}

// BuildOperatorBinary builds the operator binary for current platform
func BuildOperatorBinary() error {
	return buildOperatorBinary(context.Background(), runtime.GOOS, runtime.GOARCH, false)
}

// BuildGatewayBinary builds the gateway-controller binary for current platform
func BuildGatewayBinary() error {
	return buildGatewayBinary(context.Background(), runtime.GOOS, runtime.GOARCH, false)
}

// buildOperatorBinary builds the operator binary for the given os and arch
func buildOperatorBinary(ctx context.Context, goos, goarch string, debug bool) error {
	fmt.Printf("Building operator binary for %s/%s (debug=%v)...\n", goos, goarch, debug)

	binary := "minio-operator"
	if debug {
		binary += "-debug"
	}

	outputPath := filepath.Join("dist", fmt.Sprintf("%s-%s-%s", binary, goos, goarch))

	env := map[string]string{
		"CGO_ENABLED": "0",
		"GOOS":        goos,
		"GOARCH":      goarch,
	}

	flags := ldflags
	if debug {
		flags = strings.Replace(flags, "-s -w", "", 1) // Remove strip flags for debug
	}

	args := []string{
		"build",
		"-trimpath",
		"-ldflags", flags,
		"-o", outputPath,
		"./cmd/operator",
	}

	fmt.Printf("version: %s\n", getVersion())
	return sh.RunWith(env, "go", args...)
}

// BuildOperatorBinaries builds operator binaries for all target platforms
func BuildOperatorBinaries() error {
	platforms := []struct{ os, arch string }{
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"linux", "s390x"},
		{"linux", "ppc64le"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
	}

	for _, p := range platforms {
		if err := buildOperatorBinary(context.Background(), p.os, p.arch, false); err != nil {
			return err
		}
	}
	return nil
}

// buildGatewayBinary builds the gateway-controller binary for the given os and arch
func buildGatewayBinary(ctx context.Context, goos, goarch string, debug bool) error {
	fmt.Printf("Building gateway-controller binary for %s/%s (debug=%v)...\n", goos, goarch, debug)

	binary := "gateway-controller"
	if debug {
		binary += "-debug"
	}

	outputPath := filepath.Join("dist", fmt.Sprintf("%s-%s-%s", binary, goos, goarch))

	env := map[string]string{
		"CGO_ENABLED": "0",
		"GOOS":        goos,
		"GOARCH":      goarch,
	}

	flags := ldflags
	if debug {
		flags = strings.Replace(flags, "-s -w", "", 1)
	}

	args := []string{
		"build",
		"-trimpath",
		"-ldflags", flags,
		"-o", outputPath,
		"./cmd/gateway-controller",
	}

	fmt.Printf("version: %s\n", getVersion())
	return sh.RunWith(env, "go", args...)
}

// BuildGatewayBinaries builds gateway-controller binaries for all target platforms
func BuildGatewayBinaries() error {
	platforms := []struct{ os, arch string }{
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"linux", "s390x"},
		{"linux", "ppc64le"},
	}

	for _, p := range platforms {
		if err := buildGatewayBinary(context.Background(), p.os, p.arch, false); err != nil {
			return err
		}
	}
	return nil
}

// BuildImages builds Docker images for operator and gateway-controller
func BuildImages() error {
	if err := BuildOperatorImage(); err != nil {
		return err
	}
	return BuildGatewayImage()
}

// BuildOperatorImage builds the operator Docker image
func BuildOperatorImage() error {
	fmt.Println("Building operator Docker image...")

	// Build binary first
	if err := buildOperatorBinary(context.Background(), "linux", runtime.GOARCH, false); err != nil {
		return err
	}

	version := getVersion()
	registry := getRegistry()
	tag := fmt.Sprintf("%s/minio-operator:%s", registry, version)

	return sh.Run("docker", "build", "-t", tag, "-f", "Dockerfile", ".")
}

// BuildGatewayImage builds the gateway-controller Docker image
func BuildGatewayImage() error {
	fmt.Println("Building gateway-controller Docker image...")

	// Build binary first for linux/amd64
	if err := buildGatewayBinary(context.Background(), "linux", "amd64", false); err != nil {
		return err
	}

	version := getVersion()
	registry := getRegistry()
	tag := fmt.Sprintf("%s/gateway-controller:%s", registry, version)
	latestTag := fmt.Sprintf("%s/gateway-controller:latest", registry)

	// Build with version tag
	if err := sh.Run("docker", "build", "-t", tag, "-f", "Dockerfile.gateway", "."); err != nil {
		return err
	}

	// Also tag as latest
	return sh.Run("docker", "tag", tag, latestTag)
}

// PushOperatorImage pushes the operator Docker image to registry
func PushOperatorImage() error {
	fmt.Println("Pushing operator Docker image...")

	version := getVersion()
	registry := getRegistry()
	tag := fmt.Sprintf("%s/minio-operator:%s", registry, version)

	return sh.Run("docker", "push", tag)
}

// PushGatewayImage pushes the gateway-controller Docker image to registry
func PushGatewayImage() error {
	fmt.Println("Pushing gateway-controller Docker image...")

	version := getVersion()
	registry := getRegistry()
	tag := fmt.Sprintf("%s/gateway-controller:%s", registry, version)
	latestTag := fmt.Sprintf("%s/gateway-controller:latest", registry)

	// Push version tag
	if err := sh.Run("docker", "push", tag); err != nil {
		return err
	}

	// Push latest tag
	fmt.Println("Pushing latest tag...")
	return sh.Run("docker", "push", latestTag)
}

// PushImages pushes both operator and gateway-controller images
func PushImages() error {
	if err := PushOperatorImage(); err != nil {
		return err
	}
	return PushGatewayImage()
}

// BuildAndPushImages builds and pushes both images
func BuildAndPushImages() error {
	if err := BuildImages(); err != nil {
		return err
	}
	return PushImages()
}

// BuildAndPushOperatorImage builds and pushes operator image
func BuildAndPushOperatorImage() error {
	if err := BuildOperatorImage(); err != nil {
		return err
	}
	return PushOperatorImage()
}

// BuildAndPushGatewayImage builds and pushes gateway-controller image
func BuildAndPushGatewayImage() error {
	if err := BuildGatewayImage(); err != nil {
		return err
	}
	return PushGatewayImage()
}

// Test runs all tests
func Test() error {
	fmt.Println("Running tests...")
	return sh.Run("go", "test", "-race", "./...")
}

// TestCoverage runs tests with coverage
func TestCoverage() error {
	fmt.Println("Running tests with coverage...")
	return sh.Run("go", "test", "-race", "-coverprofile=coverage.out", "-covermode=atomic", "./...")
}

// Lint runs golangci-lint
func Lint() error {
	fmt.Println("Running linter...")
	return sh.Run("golangci-lint", "run", "--timeout=5m", "--config", ".golangci.yml")
}

// Vet runs go vet
func Vet() error {
	fmt.Println("Running go vet...")
	return sh.Run("go", "vet", "./...")
}

// Fmt formats all Go files
func Fmt() error {
	fmt.Println("Formatting Go files...")
	return sh.Run("gofmt", "-w", ".")
}

// Clean removes build artifacts
func Clean() error {
	fmt.Println("Cleaning build artifacts...")
	return sh.Rm("dist")
}

// RegenCRD regenerates CRD manifests
func RegenCRD() error {
	fmt.Println("Regenerating CRD manifests...")

	// Install controller-gen if not present
	if err := sh.Run("go", "install", "sigs.k8s.io/controller-tools/cmd/controller-gen@v0.17.2"); err != nil {
		return err
	}

	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = filepath.Join(os.Getenv("HOME"), "go")
	}
	controllerGen := filepath.Join(gopath, "bin", "controller-gen")

	// Generate CRDs
	if err := sh.Run(controllerGen, "crd:maxDescLen=0,generateEmbeddedObjectMeta=true", "webhook", "paths=./...", "output:crd:artifacts:config=resources/base/crds"); err != nil {
		return err
	}

	// Copy to Helm templates with namespace templating
	helmTemplates := "helm/operator/templates"

	crds := []string{
		"minio.min.io_tenants.yaml",
		"sts.min.io_policybindings.yaml",
	}

	for _, crd := range crds {
		srcPath := filepath.Join("resources/base/crds", crd)
		dstPath := filepath.Join(helmTemplates, crd)

		content, err := os.ReadFile(srcPath)
		if err != nil {
			return err
		}

		// Replace namespace with Helm template
		modified := strings.ReplaceAll(string(content), "namespace: minio-operator", "namespace: {{ .Release.Namespace }}")

		// Wrap with installCRDs conditional (CRD already starts with ---)
		wrapped := fmt.Sprintf("{{ if .Values.operator.installCRDs }}\n%s{{ end }}\n", modified)

		if err := os.WriteFile(dstPath, []byte(wrapped), 0644); err != nil {
			return err
		}
	}

	fmt.Println("CRD regeneration complete")
	return nil
}

// HelmPackage packages the Helm chart
func HelmPackage() error {
	fmt.Println("Packaging Helm chart...")
	return sh.Run("helm", "package", "helm/operator", "-d", "helm-releases")
}

// HelmReindex reindexes the Helm chart repository
func HelmReindex() error {
	fmt.Println("Reindexing Helm chart repository...")
	return sh.Run("helm", "repo", "index", "helm-releases", "--url", "https://operator.min.io")
}

// Install installs the operator to the current Kubernetes cluster
func Install() error {
	fmt.Println("Installing operator...")
	return sh.Run("kubectl", "apply", "-k", "resources/base")
}

// Uninstall uninstalls the operator from the current Kubernetes cluster
func Uninstall() error {
	fmt.Println("Uninstalling operator...")
	return sh.Run("kubectl", "delete", "-k", "resources/base")
}

// Dev runs Tilt for local development
func Dev() error {
	fmt.Println("Starting Tilt for local development...")
	return sh.Run("tilt", "up")
}

// Helper functions

func getVersion() string {
	version := os.Getenv("VERSION")
	if version != "" {
		return version
	}

	// Try to get from git tag
	cmd := exec.Command("git", "describe", "--tags", "--always", "--dirty")
	output, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(output))
	}

	return "dev"
}

func getCommitID() string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	output, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(output))
	}
	return "unknown"
}

func getReleaseTime() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func getOS() string {
	return runtime.GOOS
}

func getArch() string {
	return runtime.GOARCH
}

func getRegistry() string {

	return fmt.Sprintf("ghcr.io/%s", "vinaycharlie01")

}

// Made with Bob

func Rsync() error {
	return exec.Command("rsync", "-avz", "--progress", "/Users/vinaykumar/selfhosted/enlearn/operator/", "ibm-dev:/root/self-hosted/minio-operator").Run()
}
