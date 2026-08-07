package deploy

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/operation"
)

type templateArgs struct {
	Src    string `yaml:"src"`
	Dest   string `yaml:"dest"`
	Backup bool   `yaml:"backup,omitempty"`
	Mode   string `yaml:"mode,omitempty"`
}

type templateModule struct{}

func (m *templateModule) Name() string {
	return "template"
}

func (m *templateModule) DecodeArgs(node *yaml.Node) (any, error) {
	var args templateArgs
	if err := decodeStrict(node, &args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Src) == "" || strings.TrimSpace(args.Dest) == "" {
		return nil, fmt.Errorf("template 需要 src 和 dest")
	}
	if args.Mode != "" {
		if _, err := strconv.ParseUint(args.Mode, 8, 12); err != nil {
			return nil, fmt.Errorf("template mode 必须是八进制权限字符串")
		}
	}
	return &args, nil
}

func (m *templateModule) Run(tc TaskContext, raw any) ModuleResult {
	args := raw.(*templateArgs)
	localPath := resolveRelative(tc.BaseDir, args.Src)
	data, err := os.ReadFile(localPath)
	if err != nil {
		return failedModule(fmt.Errorf("读取模板 %s 失败: %w", localPath, err), operation.StageConfig)
	}
	content := string(data)
	content = strings.TrimPrefix(content, "\ufeff")
	rendered, err := RenderString(content, tc.Vars)
	if err != nil {
		return failedModule(err, operation.StageConfig)
	}
	return pushToRemote(tc, "", rendered, args.Dest, args.Backup, nil, args.Mode, tc.BaseDir)
}
