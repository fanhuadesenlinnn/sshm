package command

import (
	"fmt"
	"sort"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/deploy"
)

func (app *App) cmdCompletion(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("用法: sshm completion <bash|zsh|fish>")
	}
	switch args[0] {
	case "candidates":
		candidates, err := app.completionCandidates()
		if err != nil {
			return err
		}
		fmt.Println(strings.Join(candidates, "\n"))
	case "bash":
		fmt.Print(completionScript("bash"))
	case "zsh":
		fmt.Print(completionScript("zsh"))
	case "fish":
		fmt.Print(completionScript("fish"))
	default:
		return fmt.Errorf("不支持的 Shell %q，可使用 bash、zsh 或 fish", args[0])
	}
	return nil
}

func (app *App) completionCandidates() ([]string, error) {
	doc, err := app.Store.Repository().Load()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var candidates []string
	add := func(value string) {
		if !seen[value] {
			seen[value] = true
			candidates = append(candidates, value)
		}
	}
	for _, command := range commandNamesForCompletion() {
		add(command)
	}
	for _, host := range doc.Hosts {
		add(host.Alias)
	}
	for _, tag := range doc.Tags.Items {
		add(tag.Name)
	}
	if paths, err := deploy.Discover(nil, ""); err == nil {
		if catalog, err := deploy.Load(paths); err == nil {
			for _, profile := range catalog.Profiles {
				add(profile.Name)
			}
		}
	}
	sort.Strings(candidates)
	return candidates, nil
}

func completionScript(shell string) string {
	switch shell {
	case "bash":
		return `_sshm_complete() {
  local cur="${COMP_WORDS[COMP_CWORD]}"
  COMPREPLY=( $(compgen -W "$(sshm completion candidates 2>/dev/null)" -- "$cur") )
}
complete -F _sshm_complete sshm
`
	case "zsh":
		return `#compdef sshm
_sshm() {
  local -a candidates
  candidates=("${(@f)$(sshm completion candidates 2>/dev/null)}")
  _describe 'sshm command or host' candidates
}
compdef _sshm sshm
`
	case "fish":
		return "complete -c sshm -f -a '(sshm completion candidates 2>/dev/null)'\n"
	default:
		return ""
	}
}
