package cmd

// skills/dalang-deploy/SKILL.md is read by AI agents and followed literally. A command or flag it
// names that the CLI does not have makes an agent fail in a way nobody sees, so the skill is checked
// against the code: its frontmatter is what an agent loader needs, every `dalang app|container|volume
// <sub>` is a subcommand that exists, and every --flag it uses is defined somewhere in cmd/.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func readSkill(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "skills", "dalang-deploy", "SKILL.md"))
	if err != nil {
		t.Fatalf("the skill file is missing: %v", err)
	}
	return string(b)
}

func TestSkill_HasTheFrontmatterAnAgentLoaderNeeds(t *testing.T) {
	s := readSkill(t)
	m := regexp.MustCompile(`(?s)^---\nname: ([a-z0-9-]+)\ndescription: (.+?)\n---\n`).FindStringSubmatch(s)
	if m == nil {
		t.Fatal("the skill must start with --- name: ... description: ... ---")
	}
	if m[1] != "dalang-deploy" {
		t.Errorf("name = %q, must equal its directory name", m[1])
	}
	if len(m[2]) < 80 || !strings.Contains(strings.ToLower(m[2]), "deploy") {
		t.Errorf("the description is what makes an agent pick this skill; it says %q", m[2])
	}
}

func TestSkill_EverySubcommandItNamesExists(t *testing.T) {
	s := readSkill(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("DALANG_API_URL", "http://127.0.0.1:1") // nothing is called: there are no credentials

	run := map[string]func([]string) error{"app": cmdApp, "container": cmdContainer, "volume": cmdVolume}
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile("dalang (app|container|volume) ([a-z-]+)").FindAllStringSubmatch(s, -1) {
		key := m[1] + " " + m[2]
		if seen[key] {
			continue
		}
		seen[key] = true
		resetGlobalFlags()
		err := run[m[1]]([]string{m[2]})
		if err != nil && strings.Contains(err.Error(), "unknown "+m[1]+" command") {
			t.Errorf("the skill tells an agent to run `dalang %s`, which does not exist", key)
		}
	}
	resetGlobalFlags()
	if len(seen) < 8 {
		t.Errorf("only %d distinct subcommands found in the skill: the pattern is probably wrong", len(seen))
	}
}

func TestSkill_EveryFlagItUsesIsDefined(t *testing.T) {
	s := readSkill(t)
	var source strings.Builder
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, _ := os.ReadFile(f)
		source.Write(b)
	}
	// Only the code lines of the skill: flags in prose or in the table of causes are also commands.
	for _, m := range regexp.MustCompile(`(?:^|\s)--([a-z][a-z-]+)`).FindAllStringSubmatch(s, -1) {
		name := m[1]
		if !strings.Contains(source.String(), `"`+name+`"`) && !strings.Contains(source.String(), `"--`+name+`"`) {
			t.Errorf("the skill uses --%s, which no command defines", name)
		}
	}
}
