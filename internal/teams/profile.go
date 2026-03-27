package teams

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	shippedteams "github.com/canhta/gistclaw/teams"
)

const DefaultProfileName = "default"

var profileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

type Profile struct {
	Name string
	Path string
}

func NormalizeProfileName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("team: profile name is required")
	}
	if !profileNamePattern.MatchString(name) {
		return "", fmt.Errorf("team: invalid profile name %q", name)
	}
	return name, nil
}

func ProfilesRoot(profilesRoot string) string {
	return profilesRoot
}

func ProfileDir(profilesRoot, profile string) string {
	return filepath.Join(ProfilesRoot(profilesRoot), profile)
}

func ListProfiles(profilesRoot string) ([]Profile, error) {
	entries, err := os.ReadDir(ProfilesRoot(profilesRoot))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("team: list profiles: %w", err)
	}

	profiles := make([]Profile, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name, err := NormalizeProfileName(entry.Name())
		if err != nil {
			continue
		}
		path := ProfileDir(profilesRoot, name)
		if _, err := os.Stat(filepath.Join(path, "team.yaml")); err != nil {
			continue
		}
		profiles = append(profiles, Profile{Name: name, Path: path})
	}

	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})
	return profiles, nil
}

func CreateProfile(profilesRoot, profile string) error {
	name, err := NormalizeProfileName(profile)
	if err != nil {
		return err
	}
	dstDir := ProfileDir(profilesRoot, name)
	if _, err := os.Stat(dstDir); err == nil {
		return fmt.Errorf("team: profile %q already exists", name)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("team: stat profile %q: %w", name, err)
	}
	if err := copyFS(shippedteams.Default(), dstDir); err != nil {
		return fmt.Errorf("team: create profile %q: %w", name, err)
	}
	return nil
}

func CloneProfile(profilesRoot, srcProfile, dstProfile string) error {
	srcName, err := NormalizeProfileName(srcProfile)
	if err != nil {
		return err
	}
	return CloneProfileFromDir(profilesRoot, ProfileDir(profilesRoot, srcName), dstProfile)
}

func CloneProfileFromDir(profilesRoot, srcDir, dstProfile string) error {
	dstName, err := NormalizeProfileName(dstProfile)
	if err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(srcDir, "team.yaml")); err != nil {
		return fmt.Errorf("team: source profile not found")
	}

	dstDir := ProfileDir(profilesRoot, dstName)
	if _, err := os.Stat(dstDir); err == nil {
		return fmt.Errorf("team: profile %q already exists", dstName)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("team: stat profile %q: %w", dstName, err)
	}

	if err := copyFS(os.DirFS(srcDir), dstDir); err != nil {
		return fmt.Errorf("team: clone profile into %q: %w", dstName, err)
	}
	return nil
}

func DeleteProfile(profilesRoot, profile string) error {
	name, err := NormalizeProfileName(profile)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(ProfileDir(profilesRoot, name)); err != nil {
		return fmt.Errorf("team: delete profile %q: %w", name, err)
	}
	return nil
}

func copyFS(src fs.FS, dstDir string) error {
	return fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		target := dstDir
		if path != "." {
			target = filepath.Join(dstDir, path)
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		body, err := fs.ReadFile(src, path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(target), err)
		}
		if err := os.WriteFile(target, body, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
		return nil
	})
}
