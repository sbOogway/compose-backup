// compose-backup — backup & restore any Docker Compose stack
//
// Build:
//   go build -o compose-backup        # current platform
//   GOOS=windows GOARCH=amd64 go build -o compose-backup.exe
//   GOOS=darwin  GOARCH=arm64 go build -o compose-backup-mac
//
// Usage:
//   compose-backup backup  <stack-dir> [output.tar.gz]
//   compose-backup restore <stack-dir> <backup.tar.gz>

package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
)

// ── Entry point ───────────────────────────────────────────────────────────────

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "❌  %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 2 {
		return usageError()
	}

	command := strings.ToLower(args[0])
	stackDir, err := filepath.Abs(args[1])
	if err != nil {
		return fmt.Errorf("invalid stack directory: %w", err)
	}
	stackName := filepath.Base(stackDir)

	switch command {
	case "backup":
		out := stackName + "_backup.tar.gz"
		if len(args) >= 3 {
			out = args[2]
		}
		return backup(stackDir, stackName, out)

	case "restore":
		if len(args) < 3 {
			return fmt.Errorf("restore requires a backup file argument")
		}
		return restore(stackDir, stackName, args[2])

	case "volumes":
		return listVolumes(stackDir, stackName)

	default:
		return usageError()
	}
}

// ── Backup ────────────────────────────────────────────────────────────────────

func backup(stackDir, stackName, outFile string) error {
	composeFile, err := findComposeFile(stackDir)
	if err != nil {
		return err
	}
	step("Found compose file: %s", composeFile)

	// Collect image names
	images, err := composeImages(composeFile)
	if err != nil {
		return fmt.Errorf("could not list images: %w", err)
	}
	if len(images) == 0 {
		return errors.New("no images found in compose config")
	}
	step("Images to save: %s", strings.Join(images, ", "))

	// Save images to a temp tar
	imagesTar := filepath.Join(os.TempDir(), stackName+"_images.tar")
	defer os.Remove(imagesTar)

	step("Saving Docker images → %s", imagesTar)
	saveArgs := append([]string{"save", "-o", imagesTar}, images...)
	if err := dockerCmd(saveArgs...); err != nil {
		return fmt.Errorf("docker save failed: %w", err)
	}

	// Write the final archive
	step("Archiving stack + images → %s", outFile)
	if err := createArchive(outFile, stackDir, imagesTar); err != nil {
		return fmt.Errorf("archive creation failed: %w", err)
	}

	ok("Backup complete → %s", outFile)
	return nil
}

// ── Restore ───────────────────────────────────────────────────────────────────

func restore(stackDir, stackName, backupFile string) error {
	if _, err := os.Stat(backupFile); err != nil {
		return fmt.Errorf("backup file not found: %s", backupFile)
	}

	// Extract to the parent of stackDir so paths are preserved
	extractTo := filepath.Dir(stackDir)
	step("Extracting %s → %s", backupFile, extractTo)
	if err := extractArchive(backupFile, extractTo); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Load images tar if present
	imagesTar := filepath.Join(extractTo, stackName+"_images.tar")
	if _, err := os.Stat(imagesTar); err == nil {
		step("Loading Docker images from %s", imagesTar)
		if err := dockerCmd("load", "-i", imagesTar); err != nil {
			return fmt.Errorf("docker load failed: %w", err)
		}
		os.Remove(imagesTar)
	} else {
		fmt.Println("  ⚠  No images tar found — skipping docker load")
	}

	composeFile, err := findComposeFile(stackDir)
	if err != nil {
		return err
	}

	step("Starting containers")
	if err := composeUp(composeFile); err != nil {
		return fmt.Errorf("docker compose up failed: %w", err)
	}

	ok("Restore complete. Stack '%s' is running.", stackName)
	return nil
}

// ── Volumes ───────────────────────────────────────────────────────────────────

func listVolumes(stackDir, stackName string) error {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer cli.Close()

	f := filters.NewArgs()
	f.Add("label", fmt.Sprintf("com.docker.compose.project=%s", stackName))

	volumes, err := cli.VolumeList(ctx, volume.ListOptions{Filters: f})
	if err != nil {
		return fmt.Errorf("failed to list volumes: %w", err)
	}

	if len(volumes.Volumes) == 0 {
		fmt.Printf("No volumes found for stack '%s'\n", stackName)
		return nil
	}

	fmt.Printf("Volumes for stack '%s':\n\n", stackName)
	for _, v := range volumes.Volumes {
		fmt.Printf("Name: %s\nMountpoint: %s\n\n", v.Name, v.Mountpoint)
	}

	return nil
}

// ── Docker helpers ────────────────────────────────────────────────────────────

// findComposeFile returns the first recognised compose file inside dir.
func findComposeFile(dir string) (string, error) {
	candidates := []string{
		"docker-compose.yml", "docker-compose.yaml",
		"compose.yml", "compose.yaml",
	}
	for _, name := range candidates {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no compose file found in %s", dir)
}

// composeImages runs `docker compose config --images` and returns the list.
func composeImages(composeFile string) ([]string, error) {
	out, err := composeOutput(composeFile, "config", "--images")
	if err != nil {
		return nil, err
	}
	var images []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			images = append(images, line)
		}
	}
	return images, nil
}

// composeOutput runs a docker compose command and captures stdout.
func composeOutput(composeFile string, subArgs ...string) (string, error) {
	baseArgs := []string{"-f", composeFile}
	args := append(baseArgs, subArgs...)

	// Try `docker compose` (V2) first, then `docker-compose` (V1)
	for _, bin := range [][]string{{"docker", "compose"}, {"docker-compose"}} {
		cmdArgs := append(bin[1:], args...)
		cmd := exec.Command(bin[0], cmdArgs...)
		out, err := cmd.Output()
		if err == nil {
			return string(out), nil
		}
		if errors.Is(err, exec.ErrNotFound) {
			continue
		}
	}
	return "", errors.New("neither 'docker compose' nor 'docker-compose' found")
}

// composeUp starts the stack detached.
func composeUp(composeFile string) error {
	for _, bin := range [][]string{{"docker", "compose"}, {"docker-compose"}} {
		args := append(bin[1:], "-f", composeFile, "up", "-d")
		cmd := exec.Command(bin[0], args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			return nil
		} else if errors.Is(err, exec.ErrNotFound) {
			continue
		} else {
			return err
		}
	}
	return errors.New("neither 'docker compose' nor 'docker-compose' found")
}

// dockerCmd runs a plain docker command, streaming output.
func dockerCmd(args ...string) error {
	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ── Archive helpers ───────────────────────────────────────────────────────────

// createArchive writes a .tar.gz containing the stack directory and images tar.
func createArchive(outFile, stackDir, imagesTar string) error {
	f, err := os.Create(outFile)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Add the stack directory tree
	if err := addDirToTar(tw, stackDir, filepath.Dir(stackDir)); err != nil {
		return err
	}
	// Add the images tar at the top level (alongside the stack dir)
	if err := addFileToTar(tw, imagesTar, filepath.Base(imagesTar)); err != nil {
		return err
	}
	return nil
}

// addDirToTar walks dir and writes every file into tw, with paths relative to base.
func addDirToTar(tw *tar.Writer, dir, base string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		// Normalise to forward slashes for cross-platform archives
		rel = filepath.ToSlash(rel)

		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = rel
		if info.IsDir() {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		fh, err := os.Open(path)
		if err != nil {
			return err
		}
		defer fh.Close()
		_, err = io.Copy(tw, fh)
		return err
	})
}

// addFileToTar adds a single file to tw under the given archive name.
func addFileToTar(tw *tar.Writer, filePath, archiveName string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	hdr.Name = archiveName
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	fh, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = io.Copy(tw, fh)
	return err
}

// extractArchive unpacks a .tar.gz into destDir.
func extractArchive(archiveFile, destDir string) error {
	f, err := os.Open(archiveFile)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		// Sanitise path to prevent traversal attacks
		target := filepath.Join(destDir, filepath.FromSlash(hdr.Name))
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal path in archive: %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}

// ── Output helpers ────────────────────────────────────────────────────────────

func step(format string, a ...any) {
	fmt.Printf("\n▶  "+format+"\n", a...)
}

func ok(format string, a ...any) {
	fmt.Printf("\n✅  "+format+"\n", a...)
}

func usageError() error {
	fmt.Fprintf(os.Stderr, `compose-backup — backup & restore any Docker Compose stack

Usage:
  compose-backup backup   <stack-dir> [output.tar.gz]
  compose-backup restore <stack-dir> <backup.tar.gz>
  compose-backup volumes <stack-dir>

Examples:
  compose-backup backup  /opt/nextcloud
  compose-backup backup  /opt/nextcloud  /backups/nextcloud_2026.tar.gz
  compose-backup restore /opt/nextcloud  /backups/nextcloud_2026.tar.gz
  compose-backup volumes /opt/nextcloud

Build for other platforms (from any machine with Go installed):
  GOOS=windows GOARCH=amd64 go build -o compose-backup.exe
  GOOS=darwin  GOARCH=arm64 go build -o compose-backup-mac
  GOOS=linux   GOARCH=amd64 go build -o compose-backup-linux
 `)
	return errors.New("invalid arguments")
}
