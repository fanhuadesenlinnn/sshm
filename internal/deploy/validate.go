package deploy

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
	"github.com/fanhuadesenlinnn/sshm/v5/internal/operation"
)

func ValidateCatalog(catalog *Catalog, hosts []config.Host) error {
	if len(catalog.Profiles) == 0 {
		return fmt.Errorf("deploy 配置不包含 profile")
	}
	for _, profile := range catalog.Profiles {
		if err := ValidateProfile(profile, hosts, false); err != nil {
			return fmt.Errorf("%s: profile %q: %w", profile.Source, profile.Name, err)
		}
	}
	return nil
}

func ValidateProfile(profile Profile, hosts []config.Host, allowEmptyTargets bool) error {
	if strings.TrimSpace(profile.Name) == "" {
		return fmt.Errorf("名称不能为空")
	}
	if !profileNamePattern.MatchString(profile.Name) {
		return fmt.Errorf("名称只能包含字母、数字、点、下划线和短横线")
	}
	for name, value := range profile.Vars {
		if !variableNamePattern.MatchString(name) {
			return fmt.Errorf("变量名称无效: %s", name)
		}
		if variablePattern.MatchString(value) {
			return fmt.Errorf("变量 %q 不能递归引用其他变量", name)
		}
	}
	if len(profile.Steps) == 0 {
		return fmt.Errorf("至少需要一个 step")
	}
	if err := validateStrategy(profile.Strategy); err != nil {
		return err
	}
	for index, step := range profile.Steps {
		if err := validateStep(step, profile.Vars); err != nil {
			return fmt.Errorf("step %d (%s): %w", index+1, step.DisplayName(index), err)
		}
	}
	if _, err := ResolveSteps(profile); err != nil {
		return err
	}
	if profile.Targets.Empty() {
		if allowEmptyTargets {
			return nil
		}
		return fmt.Errorf("targets 不能为空")
	}
	_, err := ResolveTargets(hosts, profile.Targets)
	return err
}

func validateStrategy(strategy Strategy) error {
	if strategy.Mode != "hidden" && strategy.Mode != "visible" {
		return fmt.Errorf("mode 必须是 hidden 或 visible")
	}
	if strategy.MaxParallel < 1 {
		return fmt.Errorf("max_parallel 必须大于 0")
	}
	if strategy.Mode == "visible" && strategy.MaxParallel != 1 {
		return fmt.Errorf("visible 模式要求 max_parallel=1")
	}
	if strategy.ConnectTimeout.Duration < 0 || strategy.StepTimeout.Duration < 0 || strategy.RunTimeout.Duration < 0 {
		return fmt.Errorf("timeout 不能为负数")
	}
	if strategy.RetryCount < 0 {
		return fmt.Errorf("retry_count 不能为负数")
	}
	for _, stage := range strategy.RetryOnStage {
		if stage != string(operation.StageNetwork) && stage != string(operation.StageTransfer) {
			return fmt.Errorf("retry_on_stage 仅支持 network 或 transfer")
		}
	}
	return nil
}

func validateStep(step Step, vars map[string]string) error {
	switch step.Type {
	case "exec":
		if step.Command == "" {
			return fmt.Errorf("exec step 缺少 command")
		}
		if step.Src != "" || step.Dest != "" || step.Method != "" || step.Overwrite {
			return fmt.Errorf("exec step 不能包含 copy 字段")
		}
		if _, err := Expand(step.Command, vars); err != nil {
			return err
		}
	case "copy":
		if step.Src == "" || step.Dest == "" {
			return fmt.Errorf("copy step 需要 src 和 dest")
		}
		if step.Command != "" {
			return fmt.Errorf("copy step 不能包含 command")
		}
		if step.Method != "" && step.Method != "auto" && step.Method != "sftp" && step.Method != "rsync" {
			return fmt.Errorf("method 必须是 auto、sftp 或 rsync")
		}
		if _, err := Expand(step.Src, vars); err != nil {
			return err
		}
		if _, err := Expand(step.Dest, vars); err != nil {
			return err
		}
	default:
		return fmt.Errorf("type 必须是 copy 或 exec")
	}
	if step.Timeout.Duration < 0 {
		return fmt.Errorf("timeout 不能为负数")
	}
	return nil
}

func ResolveSteps(profile Profile) ([]Step, error) {
	steps := make([]Step, len(profile.Steps))
	for index, step := range profile.Steps {
		resolved := step
		var err error
		resolved.Command, err = Expand(step.Command, profile.Vars)
		if err != nil {
			return nil, err
		}
		resolved.Src, err = Expand(step.Src, profile.Vars)
		if err != nil {
			return nil, err
		}
		resolved.Dest, err = Expand(step.Dest, profile.Vars)
		if err != nil {
			return nil, err
		}
		if resolved.Type == "exec" && strings.TrimSpace(resolved.Command) == "" {
			return nil, fmt.Errorf("step %d (%s): 变量替换后的 command 不能为空", index+1, step.DisplayName(index))
		}
		if resolved.Type == "copy" && (resolved.Src == "" || resolved.Dest == "") {
			return nil, fmt.Errorf("step %d (%s): 变量替换后的 src 和 dest 不能为空", index+1, step.DisplayName(index))
		}
		if resolved.Type == "copy" {
			if resolved.Method == "" {
				resolved.Method = "auto"
			}
			if resolved.Src == "~" || strings.HasPrefix(resolved.Src, "~/") {
				resolved.Src = config.ExpandPath(resolved.Src)
			} else if resolved.Src != "" && !filepath.IsAbs(resolved.Src) {
				resolved.Src = filepath.Join(profile.BaseDir, resolved.Src)
			}
		}
		steps[index] = resolved
	}
	return steps, nil
}
