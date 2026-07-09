package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallScriptRequiresConfirmationBeforeUserWrites(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is a POSIX shell installer")
	}
	repoRoot := findRepoRoot(t)
	archive := writeMinimalInstallArchive(t)
	root := t.TempDir()
	skillsDir := filepath.Join(root, "skills")
	binDir := filepath.Join(root, "bin")

	configHome := filepath.Join(root, "xdg")
	stdout, stderr, err := runInstallScriptWithEnv(t, repoRoot,
		[]string{"XDG_CONFIG_HOME=" + configHome},
		"--from-archive", archive,
		"--skills-dir", skillsDir,
		"--bin-dir", binDir,
	)
	if err == nil {
		t.Fatalf("install without --yes should stop before writes\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	for _, needle := range []string{
		"Burpvalve install plan",
		"Skill destination: " + filepath.Join(skillsDir, "burpvalve"),
		"Command executable: " + filepath.Join(binDir, "burpvalve"),
		"no terminal available; pass --yes to apply this install plan",
	} {
		if !strings.Contains(stderr, needle) {
			t.Fatalf("installer stderr missing %q:\n%s", needle, stderr)
		}
	}
	if stdout != "" {
		t.Fatalf("cancelled installer should not write stdout, got:\n%s", stdout)
	}
	if _, err := os.Stat(skillsDir); !os.IsNotExist(err) {
		t.Fatalf("skills dir should not be created before confirmation, stat err=%v", err)
	}
	if _, err := os.Stat(binDir); !os.IsNotExist(err) {
		t.Fatalf("bin dir should not be created before confirmation, stat err=%v", err)
	}
}

func TestInstallScriptYesAppliesPreviewedPlan(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is a POSIX shell installer")
	}
	repoRoot := findRepoRoot(t)
	archive := writeMinimalInstallArchive(t)
	root := t.TempDir()
	skillsDir := filepath.Join(root, "skills")
	binDir := filepath.Join(root, "bin")
	configHome := filepath.Join(root, "xdg")

	stdout, stderr, err := runInstallScriptWithEnv(t, repoRoot,
		[]string{"XDG_CONFIG_HOME=" + configHome},
		"--from-archive", archive,
		"--skills-dir", skillsDir,
		"--bin-dir", binDir,
		"--yes",
	)
	if err != nil {
		t.Fatalf("install --yes failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	destBinary := filepath.Join(skillsDir, "burpvalve", "scripts", "bin", "burpvalve")
	destSkill := filepath.Join(skillsDir, "burpvalve", "SKILL.md")
	command := filepath.Join(binDir, "burpvalve")
	if info, err := os.Stat(destBinary); err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("installed binary missing or not executable: info=%v err=%v", info, err)
	}
	if body, err := os.ReadFile(destSkill); err != nil || !strings.Contains(string(body), "test skill") {
		t.Fatalf("installed skill marker missing or wrong: %q err=%v", string(body), err)
	}
	if info, err := os.Lstat(command); err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode()&0o111 == 0 {
		t.Fatalf("command should be an executable file, not a symlink: info=%v err=%v", info, err)
	}
	version := exec.Command(command, "--version")
	version.Env = append(os.Environ(), "PATH=/usr/bin:/bin")
	out, err := version.CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "burpvalve-test" {
		t.Fatalf("installed command failed after install: out=%q err=%v", string(out), err)
	}
	movedSkills := filepath.Join(root, "skills-moved")
	if err := os.Rename(skillsDir, movedSkills); err != nil {
		t.Fatal(err)
	}
	version = exec.Command(command, "--version")
	version.Env = append(os.Environ(), "PATH=/usr/bin:/bin")
	out, err = version.CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "burpvalve-test" {
		t.Fatalf("installed command should survive skills dir move: out=%q err=%v", string(out), err)
	}
	for _, needle := range []string{
		"Burpvalve install plan",
		"--yes supplied; applying install plan.",
		"Command executable: " + command,
	} {
		if !strings.Contains(stderr, needle) {
			t.Fatalf("installer stderr missing %q:\n%s", needle, stderr)
		}
	}
	for _, needle := range []string{
		"Installed burpvalve skill to " + filepath.Join(movedSkills, "burpvalve"),
		"Installed command executable to " + command,
		"Verify with: " + command + " --version",
		"Current PATH does not include " + binDir,
	} {
		if !strings.Contains(stdout, strings.ReplaceAll(needle, movedSkills, skillsDir)) {
			t.Fatalf("installer stdout missing %q:\n%s", needle, stdout)
		}
	}
	config := readFileString(t, filepath.Join(configHome, "burpvalve", "config.json"))
	for _, needle := range []string{
		`"skills_dir": "` + escapeInstallJSONPath(skillsDir) + `"`,
		`"bin_dir": "` + escapeInstallJSONPath(binDir) + `"`,
	} {
		if !strings.Contains(config, needle) {
			t.Fatalf("installer config missing %q:\n%s", needle, config)
		}
	}
}

func TestInstallScriptPreviewsExistingReplacements(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is a POSIX shell installer")
	}
	repoRoot := findRepoRoot(t)
	archive := writeMinimalInstallArchive(t)
	root := t.TempDir()
	skillsDir := filepath.Join(root, "skills")
	binDir := filepath.Join(root, "bin")
	dest := filepath.Join(skillsDir, "burpvalve")
	shim := filepath.Join(binDir, "burpvalve")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "OLD"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "old-burpvalve"), shim); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := runInstallScript(t, repoRoot,
		"--from-archive", archive,
		"--skills-dir", skillsDir,
		"--bin-dir", binDir,
		"--yes",
	)
	if err != nil {
		t.Fatalf("install --yes replacement failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	for _, needle := range []string{
		"Existing skill: replace " + dest,
		"Existing command: replace " + shim,
	} {
		if !strings.Contains(stderr, needle) {
			t.Fatalf("installer stderr missing replacement preview %q:\n%s", needle, stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "OLD")); !os.IsNotExist(err) {
		t.Fatalf("old skill content should be replaced, stat err=%v", err)
	}
	if info, err := os.Lstat(shim); err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode()&0o111 == 0 {
		t.Fatalf("stale symlink should be replaced by executable file: info=%v err=%v", info, err)
	}
}

func TestInstallScriptRobotsInputCanSelectSkillsDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is a POSIX shell installer")
	}
	repoRoot := findRepoRoot(t)
	archive := writeMinimalInstallArchive(t)
	root := t.TempDir()
	skillsDir := filepath.Join(root, "robot-skills")
	binDir := filepath.Join(root, "robot-bin")
	input := fmt.Sprintf(`{"from_archive":%q,"skills_dir":%q,"bin_dir":%q,"confirm":true}`, archive, skillsDir, binDir)

	stdout, stderr, err := runInstallScriptWithInput(t, repoRoot, input, "--robots")
	if err != nil {
		t.Fatalf("robots install failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "burpvalve", "SKILL.md")); err != nil {
		t.Fatalf("robots install did not write skill package: %v", err)
	}
	if info, err := os.Lstat(filepath.Join(binDir, "burpvalve")); err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode()&0o111 == 0 {
		t.Fatalf("robots install command should be executable file: info=%v err=%v", info, err)
	}
	if !strings.Contains(stdout, `"skills_dir":"`+escapeInstallJSONPath(skillsDir)+`"`) {
		t.Fatalf("robots stdout should report selected skills_dir:\n%s", stdout)
	}
}

func TestInstallScriptYesUsesPersistedSkillsDirDefault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is a POSIX shell installer")
	}
	repoRoot := findRepoRoot(t)
	archive := writeMinimalInstallArchive(t)
	root := t.TempDir()
	skillsDir := filepath.Join(root, "persisted-skills")
	binDir := filepath.Join(root, "persisted-bin")
	configHome := filepath.Join(root, "xdg")
	configPath := filepath.Join(configHome, "burpvalve", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(`{
  "schema_version": 1,
  "defaults": {
    "skills_dir": "`+escapeInstallJSONPath(skillsDir)+`",
    "bin_dir": "`+escapeInstallJSONPath(binDir)+`"
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := runInstallScriptWithEnv(t, repoRoot,
		[]string{"XDG_CONFIG_HOME=" + configHome},
		"--from-archive", archive,
		"--yes",
	)
	if err != nil {
		t.Fatalf("install with persisted defaults failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "burpvalve", "SKILL.md")); err != nil {
		t.Fatalf("persisted skills_dir was not used: %v", err)
	}
	if _, err := os.Stat(filepath.Join(binDir, "burpvalve")); err != nil {
		t.Fatalf("persisted bin_dir was not used: %v", err)
	}
	if !strings.Contains(stderr, "Skills directory: "+skillsDir) {
		t.Fatalf("preview should show persisted skills dir:\n%s", stderr)
	}
}

func TestInstallScriptDefaultsToPublicRepoForDownloads(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is a POSIX shell installer")
	}
	repoRoot := findRepoRoot(t)
	root := t.TempDir()
	archive, checksums, asset := writeDownloadFixture(t)
	curlLog := filepath.Join(root, "curl.log")
	ghLog := filepath.Join(root, "gh.log")
	fakeBin := writeFakeCurl(t, root, archive, checksums, curlLog, true)
	writeFakeGh(t, fakeBin, ghLog, false)

	stdout, stderr, err := runInstallScriptWithEnv(t, repoRoot,
		[]string{"PATH=" + fakeBin + ":/usr/bin:/bin"},
		"--skills-dir", filepath.Join(root, "skills"),
		"--bin-dir", filepath.Join(root, "bin"),
		"--yes",
		"--no-shims",
	)
	if err != nil {
		t.Fatalf("install with default public repo failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	log := readFileString(t, curlLog)
	for _, want := range []string{
		"https://github.com/clicksopendoors/burpvalve/releases/latest/download/" + asset,
		"https://github.com/clicksopendoors/burpvalve/releases/latest/download/checksums.txt",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("curl log missing default repo URL %q:\n%s", want, log)
		}
	}
}

func TestInstallScriptRepoOverrideAndUnauthenticatedGhFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is a POSIX shell installer")
	}
	repoRoot := findRepoRoot(t)
	root := t.TempDir()
	archive, checksums, asset := writeDownloadFixture(t)
	curlLog := filepath.Join(root, "curl.log")
	ghLog := filepath.Join(root, "gh.log")
	fakeBin := writeFakeCurl(t, root, archive, checksums, curlLog, true)
	writeFakeGh(t, fakeBin, ghLog, false)

	stdout, stderr, err := runInstallScriptWithEnv(t, repoRoot,
		[]string{
			"PATH=" + fakeBin + ":/usr/bin:/bin",
			"BURPVALVE_REPO=env/default",
		},
		"--repo", "example/override",
		"--version", "v9.9.9",
		"--skills-dir", filepath.Join(root, "skills"),
		"--bin-dir", filepath.Join(root, "bin"),
		"--yes",
		"--no-shims",
	)
	if err != nil {
		t.Fatalf("install with gh fallback failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	ghLogBody := readFileString(t, ghLog)
	if !strings.Contains(ghLogBody, "release download v9.9.9 --repo example/override") {
		t.Fatalf("gh log should show explicit repo override before fallback:\n%s", ghLogBody)
	}
	curlLogBody := readFileString(t, curlLog)
	for _, want := range []string{
		"https://github.com/example/override/releases/download/v9.9.9/" + asset,
		"https://github.com/example/override/releases/download/v9.9.9/checksums.txt",
	} {
		if !strings.Contains(curlLogBody, want) {
			t.Fatalf("curl log missing override fallback URL %q:\n%s", want, curlLogBody)
		}
	}
	if strings.Contains(curlLogBody, "env/default") {
		t.Fatalf("explicit --repo should override BURPVALVE_REPO, curl log:\n%s", curlLogBody)
	}
}

func TestInstallScriptPipedStdinPathAcceptsYes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is a POSIX shell installer")
	}
	repoRoot := findRepoRoot(t)
	archive := writeMinimalInstallArchive(t)
	root := t.TempDir()
	skillsDir := filepath.Join(root, "skills")
	binDir := filepath.Join(root, "bin")

	stdout, stderr, err := runInstallScriptFromStdin(t, repoRoot,
		"--from-archive", archive,
		"--skills-dir", skillsDir,
		"--bin-dir", binDir,
		"--yes",
	)
	if err != nil {
		t.Fatalf("piped install --yes failed: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "burpvalve", "scripts", "bin", "burpvalve")); err != nil {
		t.Fatalf("piped install did not write binary: %v", err)
	}
	if !strings.Contains(stderr, "--yes supplied; applying install plan.") {
		t.Fatalf("piped install should accept --yes, stderr:\n%s", stderr)
	}
}

func TestInstallScriptDownloadFailureTextFitsPublicAndAuthenticatedRepos(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is a POSIX shell installer")
	}
	repoRoot := findRepoRoot(t)
	root := t.TempDir()
	fakeBin := writeFakeCurl(t, root, "", "", filepath.Join(root, "curl.log"), false)
	writeFakeGh(t, fakeBin, filepath.Join(root, "gh.log"), false)

	stdout, stderr, err := runInstallScriptWithEnv(t, repoRoot,
		[]string{"PATH=" + fakeBin + ":/usr/bin:/bin"},
		"--skills-dir", filepath.Join(root, "skills"),
		"--bin-dir", filepath.Join(root, "bin"),
		"--yes",
	)
	if err == nil {
		t.Fatalf("download failure test should fail\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	for _, want := range []string{
		"could not download " + installAssetName() + " from clicksopendoors/burpvalve release latest",
		"make sure the release exists, the repository is public or accessible to this shell, and network access is available",
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("failure text missing %q:\n%s", want, stderr)
		}
	}
	for _, forbidden := range []string{
		"priv" + "ate release assets",
		"missing GitHub repo",
	} {
		if strings.Contains(stderr, forbidden) {
			t.Fatalf("failure text still contains stale wording %q:\n%s", forbidden, stderr)
		}
	}
}

func runInstallScript(t *testing.T, repoRoot string, args ...string) (string, string, error) {
	t.Helper()
	return runInstallScriptWithEnv(t, repoRoot, nil, args...)
}

func runInstallScriptWithEnv(t *testing.T, repoRoot string, extraEnv []string, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{"./install.sh"}, args...)...)
	cmd.Dir = repoRoot
	home := t.TempDir()
	defaultEnv := []string{
		"PATH=/usr/bin:/bin",
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "xdg"),
		"BURPVALVE_CONFIG=",
	}
	cmd.Env = mergeEnv(os.Environ(), append(defaultEnv, extraEnv...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func runInstallScriptWithInput(t *testing.T, repoRoot string, input string, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{"./install.sh"}, args...)...)
	cmd.Dir = repoRoot
	home := t.TempDir()
	cmd.Env = mergeEnv(os.Environ(),
		"PATH=/usr/bin:/bin",
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, "xdg"),
		"BURPVALVE_CONFIG=",
	)
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func runInstallScriptFromStdin(t *testing.T, repoRoot string, args ...string) (string, string, error) {
	t.Helper()
	script, err := os.Open(filepath.Join(repoRoot, "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	defer script.Close()
	cmd := exec.Command("bash", append([]string{"-s", "--"}, args...)...)
	cmd.Dir = repoRoot
	home := t.TempDir()
	cmd.Env = mergeEnv(os.Environ(),
		"PATH=/usr/bin:/bin",
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, "xdg"),
		"BURPVALVE_CONFIG=",
	)
	cmd.Stdin = script
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	return stdout.String(), stderr.String(), err
}

func escapeInstallJSONPath(path string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return replacer.Replace(path)
}

func mergeEnv(base []string, overrides ...string) []string {
	out := append([]string{}, base...)
	for _, override := range overrides {
		key := envKey(override)
		filtered := out[:0]
		for _, existing := range out {
			if envKey(existing) != key {
				filtered = append(filtered, existing)
			}
		}
		out = append(filtered, override)
	}
	return out
}

func envKey(entry string) string {
	if idx := strings.IndexByte(entry, '='); idx >= 0 {
		return entry[:idx]
	}
	return entry
}

func writeMinimalInstallArchive(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "burpvalve_test.tar.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	gz := gzip.NewWriter(file)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	writeTarDir(t, tw, "burpvalve")
	writeTarDir(t, tw, "burpvalve/scripts")
	writeTarDir(t, tw, "burpvalve/scripts/bin")
	writeTarFile(t, tw, "burpvalve/SKILL.md", 0o644, "---\nname: burpvalve\n---\n# test skill\n")
	writeTarFile(t, tw, "burpvalve/INSTALL.md", 0o644, "install docs\n")
	writeTarFile(t, tw, "burpvalve/scripts/bin/burpvalve", 0o755, "#!/usr/bin/env sh\nif [ \"$1\" = \"--version\" ]; then echo burpvalve-test; else echo burpvalve-test; fi\n")
	return path
}

func writeTarDir(t *testing.T, tw *tar.Writer, name string) {
	t.Helper()
	if err := tw.WriteHeader(&tar.Header{Name: name + "/", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
}

func writeDownloadFixture(t *testing.T) (string, string, string) {
	t.Helper()
	archive := writeMinimalInstallArchive(t)
	asset := installAssetName()
	body, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	checksums := filepath.Join(t.TempDir(), "checksums.txt")
	if err := os.WriteFile(checksums, []byte(fmt.Sprintf("%x  %s\n", sum, asset)), 0o644); err != nil {
		t.Fatal(err)
	}
	return archive, checksums, asset
}

func installAssetName() string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	if goarch == "386" {
		goarch = "amd64"
	}
	return "burpvalve_" + goos + "_" + goarch + ".tar.gz"
}

func writeFakeCurl(t *testing.T, root, archive, checksums, logPath string, succeeds bool) string {
	t.Helper()
	binDir := filepath.Join(root, "fake-bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `#!/usr/bin/env bash
set -euo pipefail
url=""
out=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o)
      out="$2"
      shift 2
      ;;
    -*)
      shift
      ;;
    *)
      url="$1"
      shift
      ;;
  esac
done
echo "$url" >> "$FAKE_CURL_LOG"
`
	if succeeds {
		body += `case "$url" in
  */checksums.txt)
    cp "$FAKE_CHECKSUMS" "$out"
    ;;
  */burpvalve_*.tar.gz)
    cp "$FAKE_ARCHIVE" "$out"
    ;;
  *)
    exit 22
    ;;
esac
`
	} else {
		body += "exit 22\n"
	}
	if err := os.WriteFile(filepath.Join(binDir, "curl"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_ARCHIVE", archive)
	t.Setenv("FAKE_CHECKSUMS", checksums)
	t.Setenv("FAKE_CURL_LOG", logPath)
	return binDir
}

func writeFakeGh(t *testing.T, binDir, logPath string, succeeds bool) {
	t.Helper()
	exit := "1"
	if succeeds {
		exit = "0"
	}
	body := "#!/usr/bin/env bash\n" +
		"echo \"$*\" >> \"$FAKE_GH_LOG\"\n" +
		"exit " + exit + "\n"
	if err := os.WriteFile(filepath.Join(binDir, "gh"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_GH_LOG", logPath)
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func writeTarFile(t *testing.T, tw *tar.Writer, name string, mode int64, body string) {
	t.Helper()
	header := &tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: mode, Size: int64(len(body))}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
}
