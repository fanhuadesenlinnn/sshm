package command

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/deploy"
)

func (app *App) cmdCompletion(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("用法: sshmd completion <bash|zsh|fish>")
	}
	if containsHelpToken(args) {
		printCompletionHelp()
		return nil
	}
	switch args[0] {
	case "candidates":
		if len(args) != 1 {
			return fmt.Errorf("用法: sshmd completion candidates")
		}
		candidates, err := app.completionCandidates()
		if err != nil {
			return err
		}
		fmt.Println(strings.Join(candidates, "\n"))
	case "bash":
		if len(args) != 1 {
			return fmt.Errorf("用法: sshmd completion bash")
		}
		fmt.Print(completionScript("bash"))
	case "zsh":
		if len(args) != 1 {
			return fmt.Errorf("用法: sshmd completion zsh")
		}
		fmt.Print(completionScript("zsh"))
	case "fish":
		if len(args) != 1 {
			return fmt.Errorf("用法: sshmd completion fish")
		}
		fmt.Print(completionScript("fish"))
	default:
		return fmt.Errorf("不支持的 Shell %q，可使用 bash、zsh 或 fish", args[0])
	}
	return nil
}

func printCompletionHelp() {
	fmt.Println("Shell 自动补全")
	fmt.Println()
	fmt.Println("  sshmd completion bash      生成 bash 补全脚本")
	fmt.Println("  sshmd completion zsh       生成 zsh 补全脚本")
	fmt.Println("  sshmd completion fish      生成 fish 补全脚本")
	fmt.Println()
	fmt.Println("示例:")
	fmt.Println("  sshmd completion zsh > ~/.zsh/completions/_sshm")
	fmt.Println("  sshmd completion bash > ~/.local/share/bash-completion/completions/sshmd")
	fmt.Println()
}

func (app *App) completionCandidates() ([]string, error) {
	doc, err := app.Store.Repository().Load()
	if err != nil && !errors.Is(err, config.ErrNotInitialized) {
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
	if doc != nil {
		for _, host := range doc.Hosts {
			add(host.Alias)
		}
		for _, tag := range doc.Tags.Items {
			add(tag.Name)
		}
		if paths, err := deploy.Discover(nil); err == nil {
			if catalog, err := deploy.Load(paths); err == nil {
				for _, play := range catalog.Plays {
					add(play.Name)
				}
			}
		}
	}
	sort.Strings(candidates)
	return candidates, nil
}

func completionScript(shell string) string {
	switch shell {
	case "bash":
		return `_sshmd_complete() {
  local cur="${COMP_WORDS[COMP_CWORD]}"
  COMPREPLY=( $(compgen -W "$(sshmd completion candidates 2>/dev/null)" -- "$cur") )
}
complete -F _sshmd_complete sshmd
`
	case "zsh":
		return `#compdef sshmd
_sshm() {
  local -a candidates
  candidates=("${(@f)$(sshmd completion candidates 2>/dev/null)}")
  _describe 'sshmd command or host' candidates
}
compdef _sshm sshmd
`
	case "fish":
		return "complete -c sshmd -f -a '(sshmd completion candidates 2>/dev/null)'\n"
	default:
		return ""
	}
}
